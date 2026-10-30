// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package attest

import (
	"testing"
	"time"
)

// Chaîne à deux niveaux (v1.9.2) : racine (épinglée) → CA intermédiaire → certificat d'appareil.
func TestChainTwoLevels(t *testing.T) {
	root := testRootPriv()
	now := time.Now()
	const sha = "0123456789abcdef0123456789abcdef01234567"

	// Racine → CA intermédiaire (1 an), puis CA → appareil (90 j), signé et vérifié par la chaîne.
	caPub, caPriv, _ := GenerateDeviceKey()
	ca := IssueCA(root, caPub, "CA d'émission", now.Add(-time.Hour), now.AddDate(1, 0, 0), "CA-1")

	devPub, devPriv, _ := GenerateDeviceKey()
	cert := IssueCertFrom(caPriv, ca, devPub, "Grégory Liand", now.Add(-time.Hour), now.AddDate(0, 0, 90), "DEV-1")
	att := Sign(devPriv, cert, sha)

	if v := Verify(att, sha, now, nil); !v.Verified || v.Subject != "Grégory Liand" {
		t.Fatalf("chaîne 2 niveaux : attendu vérifié, obtenu %+v", v)
	}

	// Révocation de la CA intermédiaire → toute la branche tombe.
	if v := Verify(att, sha, now, map[string]bool{"CA-1": true}); v.Verified {
		t.Error("révoquer la CA doit invalider les certificats qu'elle a émis")
	}

	// Révocation du cert d'appareil seul.
	if v := Verify(att, sha, now, map[string]bool{"DEV-1": true}); v.Verified {
		t.Error("un certificat d'appareil révoqué ne doit pas vérifier")
	}
}

// Un émetteur qui n'est pas une autorité (IsCA=false) ne peut pas certifier un appareil.
func TestChainIssuerNotCA(t *testing.T) {
	root := testRootPriv()
	now := time.Now()
	const sha = "0123456789abcdef0123456789abcdef01234567"

	// « Faux intermédiaire » : un cert d'APPAREIL (IsCA=false) utilisé comme émetteur.
	notCAPub, notCAPriv, _ := GenerateDeviceKey()
	notCA := IssueCert(root, notCAPub, "pas une CA", now.Add(-time.Hour), now.AddDate(0, 0, 90), "X-1")

	devPub, devPriv, _ := GenerateDeviceKey()
	cert := IssueCertFrom(notCAPriv, notCA, devPub, "appareil", now.Add(-time.Hour), now.AddDate(0, 0, 90), "DEV-2")
	att := Sign(devPriv, cert, sha)

	if v := Verify(att, sha, now, nil); v.Verified {
		t.Error("un certificat émis par un non-CA ne doit pas vérifier")
	}
}

// Falsifier IsCA (promotion appareil→autorité) casse la signature, car IsCA est couvert.
func TestChainIsCATamper(t *testing.T) {
	root := testRootPriv()
	now := time.Now()

	pub, _, _ := GenerateDeviceKey()
	leaf := IssueCert(root, pub, "appareil", now.Add(-time.Hour), now.AddDate(0, 0, 90), "DEV-3")
	if err := verifyCert(leaf, now, nil); err != nil {
		t.Fatalf("le cert d'appareil légitime doit vérifier : %v", err)
	}
	leaf.IsCA = true // promotion frauduleuse sans re-signature
	if err := verifyCert(leaf, now, nil); err == nil {
		t.Error("falsifier IsCA doit invalider la signature du certificat")
	}
}

// Chaîne trop profonde : racine → CA → CA → appareil dépasse maxChainDepth.
func TestChainTooDeep(t *testing.T) {
	root := testRootPriv()
	now := time.Now()
	const sha = "0123456789abcdef0123456789abcdef01234567"

	ca1Pub, ca1Priv, _ := GenerateDeviceKey()
	ca1 := IssueCA(root, ca1Pub, "CA niveau 1", now.Add(-time.Hour), now.AddDate(1, 0, 0), "CA-A")

	// CA niveau 2 émise par la CA niveau 1 (chaîne imbriquée).
	ca2Pub, ca2Priv, _ := GenerateDeviceKey()
	ca2 := signCert(ca1Priv, &ca1, ca2Pub, "CA niveau 2", now.Add(-time.Hour), now.AddDate(1, 0, 0), "CA-B", true)

	devPub, devPriv, _ := GenerateDeviceKey()
	cert := IssueCertFrom(ca2Priv, ca2, devPub, "appareil profond", now.Add(-time.Hour), now.AddDate(0, 0, 90), "DEV-4")
	att := Sign(devPriv, cert, sha)

	if v := Verify(att, sha, now, nil); v.Verified {
		t.Error("une chaîne de plus d'une autorité intermédiaire doit être refusée")
	}
}

// CA expirée → les certificats qu'elle a émis ne vérifient plus.
func TestChainExpiredCA(t *testing.T) {
	root := testRootPriv()
	now := time.Now()
	const sha = "0123456789abcdef0123456789abcdef01234567"

	caPub, caPriv, _ := GenerateDeviceKey()
	caExpired := IssueCA(root, caPub, "CA expirée", now.AddDate(0, 0, -200), now.AddDate(0, 0, -1), "CA-OLD")

	devPub, devPriv, _ := GenerateDeviceKey()
	cert := IssueCertFrom(caPriv, caExpired, devPub, "appareil", now.Add(-time.Hour), now.AddDate(0, 0, 90), "DEV-5")
	att := Sign(devPriv, cert, sha)

	if v := Verify(att, sha, now, nil); v.Verified {
		t.Error("un certificat émis par une CA expirée ne doit pas vérifier")
	}
}

// Tant que l'identité logicielle n'est pas mintée (placeholder embarqué), SoftwareSigner est inactif
// (le cœur retombe alors sur le cert d'instance). Une fois mintée par la gouvernance, elle s'active.
func TestSoftwareSignerPlaceholder(t *testing.T) {
	if HasSoftwareSigner() {
		t.Error("avec les fichiers placeholder embarqués, la signature logicielle doit être INACTIVE")
	}
	if _, _, ok := SoftwareSigner(); ok {
		t.Error("SoftwareSigner devrait renvoyer ok=false sur le placeholder")
	}
}
