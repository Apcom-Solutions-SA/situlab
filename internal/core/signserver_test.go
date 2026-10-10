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

// Le client du service de signature serveur : POST /api/sign avec le jeton Forgejo, renvoie l'attestation
// (telle quelle) ; les codes d'erreur remontent proprement.
func TestRequestServerAttestation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/sign" {
			w.WriteHeader(400)
			return
		}
		if r.Header.Get("Authorization") != "token TOK" {
			w.WriteHeader(403)
			return
		}
		var in map[string]string
		_ = json.NewDecoder(r.Body).Decode(&in)
		if len(in["commit_sha"]) == 0 || in["nonce"] == "" || in["repo_url"] == "" {
			w.WriteHeader(400)
			return
		}
		_, _ = w.Write([]byte(`{"attestation":{"commit":"` + in["commit_sha"] + `","cert":{},"sig":"zz","ts":"t"}}`))
	}))
	defer srv.Close()

	att, err := requestServerAttestation(srv.URL, "TOK", "0123456789abcdef0123456789abcdef01234567", "https://git.example/o/r")
	if err != nil {
		t.Fatalf("requestServerAttestation : %v", err)
	}
	if !strings.Contains(string(att), `"commit":"0123456789abcdef0123456789abcdef01234567"`) {
		t.Fatalf("attestation inattendue : %s", att)
	}

	// Jeton refusé → erreur (403).
	if _, err := requestServerAttestation(srv.URL, "MAUVAIS", "abc", "u"); err == nil {
		t.Error("un jeton refusé doit remonter une erreur")
	}
}

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

// Mode SERVEUR de bout en bout : un commit via SituLab déclenche un appel au service, et l'attestation
// renvoyée est écrite TELLE QUELLE dans la note git. (La validité crypto est testée côté attest ; ici on
// vérifie le branchement : appel serveur → note.)
func TestAttestCommitServerMode(t *testing.T) {
	if !gitx.Available() {
		t.Skip("git indisponible")
	}
	var gotAuth, gotSHA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		var in map[string]string
		_ = json.NewDecoder(r.Body).Decode(&in)
		gotSHA = in["commit_sha"]
		_, _ = w.Write([]byte(`{"attestation":{"commit":"` + in["commit_sha"] + `","via":"server"}}`))
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
	gitRun(t, dir, "remote", "add", "origin", "https://git.example/o/r.git")
	if err := os.WriteFile(dir+"/a.txt", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "add", "-A")
	if _, err := s.Commit(dir, "c1"); err != nil {
		t.Fatalf("Commit : %v", err)
	}
	sha := gitx.HeadSHA(dir)
	note := gitx.ReadNote(dir, attestNotesRef, sha)
	if !strings.Contains(note, `"via":"server"`) || !strings.Contains(note, sha) {
		t.Fatalf("la note doit venir du service : %q", note)
	}
	if gotAuth != "token FORGE-TOK" {
		t.Errorf("le service doit recevoir le jeton Forgejo, obtenu %q", gotAuth)
	}
	if gotSHA != sha {
		t.Errorf("le service doit recevoir le SHA du commit, obtenu %q", gotSHA)
	}
}
