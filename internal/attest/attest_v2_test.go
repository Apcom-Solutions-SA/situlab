// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package attest

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"
)

// TestCommitPreimageStable : le pré-image est déterministe et change dès qu'une composante du commit
// change (arbre, parent, corps) — fondement de l'immutabilité de l'attestation v2.
func TestCommitPreimageStable(t *testing.T) {
	base := CommitPreimage("tree1", []string{"p1"}, "A <a> 1 +0000", "C <c> 2 +0000", "msg")
	if string(CommitPreimage("tree1", []string{"p1"}, "A <a> 1 +0000", "C <c> 2 +0000", "msg")) != string(base) {
		t.Fatal("pré-image non déterministe")
	}
	for name, got := range map[string][]byte{
		"arbre":  CommitPreimage("tree2", []string{"p1"}, "A <a> 1 +0000", "C <c> 2 +0000", "msg"),
		"parent": CommitPreimage("tree1", []string{"pX"}, "A <a> 1 +0000", "C <c> 2 +0000", "msg"),
		"corps":  CommitPreimage("tree1", []string{"p1"}, "A <a> 1 +0000", "C <c> 2 +0000", "msg2"),
		"auteur": CommitPreimage("tree1", []string{"p1"}, "Z <z> 1 +0000", "C <c> 2 +0000", "msg"),
	} {
		if string(got) == string(base) {
			t.Errorf("un changement de %s doit changer le pré-image", name)
		}
	}
}

// TestSignVerifyPreimage : signature v2 d'un pré-image et vérification fail-closed (altération, révocation,
// fenêtre, clé greffée, racine inconnue, round-trip).
func TestSignVerifyPreimage(t *testing.T) {
	root := testRootPriv()
	now := time.Now()
	subject := "Grégory Liand <gl-lig@pm.me>"
	devPub, devPriv, err := GenerateDeviceKey()
	if err != nil {
		t.Fatal(err)
	}
	cert := IssueCert(root, devPub, subject, now.Add(-time.Hour), now.Add(24*time.Hour), "SER-V2-1")
	pre := CommitPreimage("t", []string{"p"}, "A <a> 1 +0200", "A <a> 1 +0200", "sujet\n\ncorps")
	att := SignPreimage(devPriv, cert, pre)
	if att.V != 2 {
		t.Fatalf("attendu V=2, obtenu %d", att.V)
	}

	if v := VerifyPreimage(att, pre, now, nil); !v.Verified || v.Subject != subject {
		t.Fatalf("attendu vérifié « %s », obtenu %+v", subject, v)
	}
	// Immutabilité : un pré-image altéré (commit modifié) ne doit jamais vérifier.
	bad := CommitPreimage("t", []string{"p"}, "A <a> 1 +0200", "A <a> 1 +0200", "sujet\n\nCORPS ALTÉRÉ")
	if v := VerifyPreimage(att, bad, now, nil); v.Verified {
		t.Error("un pré-image altéré ne doit pas vérifier")
	}
	// Révoqué.
	if v := VerifyPreimage(att, pre, now, map[string]bool{"SER-V2-1": true}); v.Verified {
		t.Error("un cert révoqué ne doit pas vérifier")
	}
	// Hors fenêtre de validité.
	expired := SignPreimage(devPriv, IssueCert(root, devPub, subject, now.Add(-48*time.Hour), now.Add(-24*time.Hour), "SER-V2-2"), pre)
	if v := VerifyPreimage(expired, pre, now, nil); v.Verified {
		t.Error("un cert expiré ne doit pas vérifier")
	}
	// Signature greffée d'une autre clé d'appareil.
	_, otherPriv, _ := GenerateDeviceKey()
	forged := att
	forged.Sig = ed25519.Sign(otherPriv, pre)
	if v := VerifyPreimage(forged, pre, now, nil); v.Verified {
		t.Error("une signature d'une clé ≠ celle du cert ne doit pas vérifier")
	}
	// Racine inconnue.
	_, badRoot, _ := ed25519.GenerateKey(rand.Reader)
	rogue := SignPreimage(devPriv, IssueCert(badRoot, devPub, subject, now.Add(-time.Hour), now.Add(time.Hour), "SER-V2-3"), pre)
	if v := VerifyPreimage(rogue, pre, now, nil); v.Verified {
		t.Error("un cert non émis par la racine APCOM ne doit pas vérifier")
	}
	// Round-trip encode/décode.
	raw, err := att.Encode()
	if err != nil {
		t.Fatal(err)
	}
	back, err := Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	if v := VerifyPreimage(back, pre, now, nil); !v.Verified {
		t.Error("round-trip v2 : l'attestation décodée doit vérifier")
	}
}
