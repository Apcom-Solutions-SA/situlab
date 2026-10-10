// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"testing"
	"time"

	"situlab/internal/attest"
	"situlab/internal/config"
	"situlab/internal/gitx"
	"situlab/internal/keyring"
)

// testRootPriv reconstruit la racine de TEST (même graine que apcom_root.pub embarqué).
func testRootPriv() ed25519.PrivateKey {
	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(0xA0 + i)
	}
	return ed25519.NewKeyFromSeed(seed)
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = dir
	c.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_AUTHOR_NAME=T", "GIT_AUTHOR_EMAIL=t@x",
		"GIT_COMMITTER_NAME=T", "GIT_COMMITTER_EMAIL=t@x")
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("git %v : %v\n%s", args, err, out)
	}
}

// TestAttestEndToEnd : génération de clé → cert (racine de test) → installation → commit via le cœur →
// attestation lue et VÉRIFIÉE (« SituLab approved — par Untel »).
func TestAttestEndToEnd(t *testing.T) {
	if !gitx.Available() {
		t.Skip("git indisponible")
	}
	// Ce test de bout en bout signe avec la racine de TEST. Quand la VRAIE racine de gouvernance est
	// embarquée (build de release), on ne peut pas produire de cert vérifiable sans son privé → on saute
	// (la logique de la chaîne reste couverte par les tests du paquet internal/attest).
	if !attest.UsingTestRoot() {
		t.Skip("racine réelle embarquée — le bout-en-bout nécessite la racine de test")
	}
	s := newInTemp(t)
	if err := s.SetPIN("", "123456"); err != nil {
		t.Fatal(err)
	}
	cfg, _ := config.Load()
	cfg.Instances = []config.Instance{{Name: "atl", URL: "https://forge.example", Role: config.RoleAtelier}}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	if err := s.Unlock("123456"); err != nil {
		t.Fatal(err)
	}
	// La connexion a un jeton rangé (comme en pratique après onboarding) : la clé d'appareil s'y loge.
	dek, err := s.sessionDEK()
	if err != nil {
		t.Fatal(err)
	}
	kr, _ := keyring.Load()
	if err := kr.SetToken(dek, "atl", "https://forge.example", nil, "tok"); err != nil {
		t.Fatal(err)
	}
	zero(dek)

	// 1) Clé d'appareil pour l'atelier → clé publique à certifier.
	pubB64, err := s.GenerateSignKey("atl")
	if err != nil {
		t.Fatalf("GenerateSignKey : %v", err)
	}
	pub, err := base64.StdEncoding.DecodeString(pubB64)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		t.Fatalf("clé publique invalide : %v", err)
	}

	// 2) Certificat émis par la gouvernance (ici la racine de test) + installation.
	subject := "Grégory Liand <gl-lig@pm.me>"
	cert := attest.IssueCert(testRootPriv(), ed25519.PublicKey(pub), subject, time.Now().Add(-time.Hour), time.Now().Add(24*time.Hour), "SER-TEST-1")
	certJSON, _ := json.Marshal(cert)
	if err := s.InstallSignCert("atl", string(certJSON)); err != nil {
		t.Fatalf("InstallSignCert : %v", err)
	}

	// 3) Dépôt local dont l'origin pointe l'hôte de l'atelier → un commit via le cœur.
	dir := t.TempDir()
	gitRun(t, dir, "init")
	gitRun(t, dir, "remote", "add", "origin", "https://forge.example/o/r.git")
	if err := os.WriteFile(dir+"/a.txt", []byte("bonjour"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "add", "-A")
	if _, err := s.Commit(dir, "premier commit"); err != nil {
		t.Fatalf("Commit : %v", err)
	}

	// 4) L'attestation du commit doit être VÉRIFIÉE avec la bonne identité.
	sha := gitx.HeadSHA(dir)
	if sha == "" {
		t.Fatal("HEAD introuvable après commit")
	}
	v := s.CommitAttestation(dir, sha)
	if !v.Verified || v.Subject != subject {
		t.Fatalf("attendu vérifié « %s », obtenu %+v", subject, v)
	}

	// 5) Révocation du serial → l'attestation ne vérifie plus.
	cfg2, _ := config.Load()
	cfg2.RevokedSerials = []string{"SER-TEST-1"}
	if err := cfg2.Save(); err != nil {
		t.Fatal(err)
	}
	if v := s.CommitAttestation(dir, sha); v.Verified {
		t.Error("après révocation, l'attestation ne doit plus vérifier")
	}

	// 6) Un commit fait HORS SituLab (git direct) n'est pas attesté.
	gitRun(t, dir, "commit", "--allow-empty", "-m", "hors situlab")
	if v := s.CommitAttestation(dir, gitx.HeadSHA(dir)); v.Verified {
		t.Error("un commit non créé par SituLab ne doit pas être attesté")
	}
}
