// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"situlab/internal/config"
	"situlab/internal/gitx"
	"situlab/internal/keyring"
)

// La liste de révocation accepte un tableau JSON ou un objet { "revoked": [...] }.
func TestFetchRevokedSerials(t *testing.T) {
	for _, body := range []string{`["A","B"]`, `{"revoked":["A","B"]}`} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/.well-known/situlab-revoked.json" {
				w.WriteHeader(404)
				return
			}
			_, _ = w.Write([]byte(body))
		}))
		ser, err := fetchRevokedSerials(srv.URL)
		srv.Close()
		if err != nil || len(ser) != 2 || ser[0] != "A" {
			t.Fatalf("fetchRevokedSerials(%s) = %v, %v", body, ser, err)
		}
	}
}

// Mode SERVEUR de bout en bout (v2) : un commit via SituLab appelle /api/sign-preimage avec le jeton
// Forgejo et le PRÉ-IMAGE du commit, et l'attestation renvoyée est posée en TRAILER immuable dans le
// commit (plus de note). Ici on vérifie le branchement : appel serveur (pré-image) → trailer.
func TestAttestCommitServerMode(t *testing.T) {
	if !gitx.Available() {
		t.Skip("git indisponible")
	}
	var gotAuth, gotPre, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		var in map[string]string
		_ = json.NewDecoder(r.Body).Decode(&in)
		gotPre = in["preimage_b64"]
		_, _ = w.Write([]byte(`{"attestation":{"v":2,"cert":{},"sig":"zz","ts":"t"}}`))
	}))
	defer srv.Close()

	s := newInTemp(t)
	if err := s.SetPIN("", "123456"); err != nil {
		t.Fatal(err)
	}
	cfg, _ := config.Load()
	cfg.Instances = []config.Instance{{Name: "atl", URL: "https://git.example", Role: config.RoleAtelier, SignMode: "server", SignServerURL: srv.URL}}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	s.retrustPolicy() // policy posée hors setters (test) : ré-ancrer la signature
	if err := s.Unlock("123456"); err != nil {
		t.Fatal(err)
	}
	dek, err := s.sessionDEK()
	if err != nil {
		t.Fatal(err)
	}
	kr, _ := keyring.Load()
	if err := kr.SetToken(dek, "atl", "https://git.example", nil, "FORGE-TOK"); err != nil {
		t.Fatal(err)
	}
	zero(dek)

	dir := t.TempDir()
	gitRun(t, dir, "init")
	gitRun(t, dir, "config", "user.email", "t@t.t")
	gitRun(t, dir, "config", "user.name", "T")
	gitRun(t, dir, "remote", "add", "origin", "https://git.example/o/r.git")
	if err := os.WriteFile(dir+"/a.txt", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "add", "-A")
	if _, err := s.Commit(dir, "c1"); err != nil {
		t.Fatalf("Commit : %v", err)
	}
	sha := gitx.HeadSHA(dir)
	obj, err := gitx.CommitObject(dir, sha)
	if err != nil {
		t.Fatalf("CommitObject : %v", err)
	}
	if !strings.Contains(obj.Message, attestTrailerKey+": ") {
		t.Fatalf("le commit doit porter le trailer d'attestation, message = %q", obj.Message)
	}
	if gotPath != "/api/sign-preimage" {
		t.Errorf("le service doit être appelé sur /api/sign-preimage, obtenu %q", gotPath)
	}
	if gotAuth != "token FORGE-TOK" {
		t.Errorf("le service doit recevoir le jeton Forgejo, obtenu %q", gotAuth)
	}
	if gotPre == "" {
		t.Errorf("le service doit recevoir le pré-image du commit (preimage_b64 vide)")
	}
}
