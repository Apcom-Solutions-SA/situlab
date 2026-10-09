// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package attest

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"
)

// testRootPriv reconstruit la racine de TEST à partir de la graine fixe (la même qui a produit
// apcom_root.pub embarqué), pour émettre des certificats dans les tests.
func testRootPriv() ed25519.PrivateKey {
	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(0xA0 + i)
	}
	return ed25519.NewKeyFromSeed(seed)
}

func TestAttestVerify(t *testing.T) {
	root := testRootPriv()
	now := time.Now()
	subject := "Grégory Liand <gl-lig@pm.me>"
	devPub, devPriv, err := GenerateDeviceKey()
	if err != nil {
		t.Fatalf("GenerateDeviceKey : %v", err)
	}
	cert := IssueCert(root, devPub, subject, now.Add(-time.Hour), now.Add(24*time.Hour), "SER-1")
	const sha = "0123456789abcdef0123456789abcdef01234567"
	att := Sign(devPriv, cert, sha)

	// Cohérence : la racine embarquée doit être la racine de test (embed ↔ graine).
	if emb, err := Root(); err != nil || !emb.Equal(root.Public()) {
		t.Fatalf("racine embarquée incohérente avec la racine de test (err=%v)", err)
	}

	// Positif.
	if v := Verify(att, sha, now, nil); !v.Verified || v.Subject != subject {
		t.Fatalf("attendu vérifié « %s », obtenu %+v", subject, v)
	}

	// Rejeu : l'attestation du commit `sha` ne doit pas vérifier sur un autre commit.
	if v := Verify(att, "ffffffffffffffffffffffffffffffffffffffff", now, nil); v.Verified {
		t.Error("rejeu : une attestation ne doit vérifier que sur son propre commit")
	}

	// Cert expiré.
	expired := Sign(devPriv, IssueCert(root, devPub, subject, now.Add(-48*time.Hour), now.Add(-24*time.Hour), "SER-2"), sha)
	if v := Verify(expired, sha, now, nil); v.Verified {
		t.Error("un certificat expiré ne doit pas vérifier")
	}

	// Cert pas encore valide.
	future := Sign(devPriv, IssueCert(root, devPub, subject, now.Add(24*time.Hour), now.Add(48*time.Hour), "SER-3"), sha)
	if v := Verify(future, sha, now, nil); v.Verified {
		t.Error("un certificat pas encore valide ne doit pas vérifier")
	}

	// Révoqué (serial dans la liste).
	if v := Verify(att, sha, now, map[string]bool{"SER-1": true}); v.Verified {
		t.Error("un certificat révoqué ne doit pas vérifier")
	}

	// Racine inconnue : cert signé par une autre racine.
	_, badRoot, _ := ed25519.GenerateKey(rand.Reader)
	rogue := Sign(devPriv, IssueCert(badRoot, devPub, subject, now.Add(-time.Hour), now.Add(time.Hour), "SER-4"), sha)
	if v := Verify(rogue, sha, now, nil); v.Verified {
		t.Error("un certificat non émis par la racine APCOM ne doit pas vérifier")
	}

	// Signature greffée : une autre clé d'appareil que celle du cert.
	_, otherPriv, _ := GenerateDeviceKey()
	forged := att
	forged.Sig = ed25519.Sign(otherPriv, commitSigningBytes(sha))
	if v := Verify(forged, sha, now, nil); v.Verified {
		t.Error("une signature d'une clé ≠ celle du certificat ne doit pas vérifier")
	}

	// Round-trip note (encode → décode → vérifie).
	b, err := att.Encode()
	if err != nil {
		t.Fatalf("Encode : %v", err)
	}
	back, err := Decode(b)
	if err != nil {
		t.Fatalf("Decode : %v", err)
	}
	if v := Verify(back, sha, now, nil); !v.Verified {
		t.Error("round-trip : l'attestation décodée doit vérifier")
	}
}
