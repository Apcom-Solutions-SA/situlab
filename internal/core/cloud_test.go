// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"situlab/internal/config"
)

func fakeCloud(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); !ok || u != "alice" || p != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Method == "PROPFIND" {
			w.WriteHeader(http.StatusMultiStatus)
			w.Write([]byte(`<?xml version="1.0"?><d:multistatus xmlns:d="DAV:"></d:multistatus>`))
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))
}

func TestBuildWebDAVBase(t *testing.T) {
	cases := []struct {
		name, kind, raw, user, want string
	}{
		{"nextcloud login URL", config.CloudNextcloud, "https://cloud.apcom.app/index.php/login", "LiG", "https://cloud.apcom.app/remote.php/dav/files/LiG"},
		{"nextcloud bare host", config.CloudNextcloud, "cloud.exemple.ch", "bob", "https://cloud.exemple.ch/remote.php/dav/files/bob"},
		{"nextcloud subdir install", config.CloudNextcloud, "https://exemple.ch/nc/index.php/apps/files", "x", "https://exemple.ch/nc/remote.php/dav/files/x"},
		{"nextcloud already dav", config.CloudNextcloud, "https://cloud.x/remote.php/dav/files/alice/", "alice", "https://cloud.x/remote.php/dav/files/alice"},
		{"kdrive subdomain", config.CloudKDrive, "https://123456.connect.kdrive.infomaniak.com", "e@x", "https://123456.connect.kdrive.infomaniak.com"},
		{"kdrive subdomain with path", config.CloudKDrive, "https://123456.connect.kdrive.infomaniak.com/Documents", "e@x", "https://123456.connect.kdrive.infomaniak.com"},
		{"kdrive generic host + id", config.CloudKDrive, "https://connect.kdrive.infomaniak.com/123456/sub", "e@x", "https://connect.kdrive.infomaniak.com/123456"},
		{"kdrive ksuite web app URL", config.CloudKDrive, "https://ksuite.infomaniak.com/53843/kdrive/app/drive/3869957/", "e@x", "https://3869957.connect.kdrive.infomaniak.com"},
		{"kdrive app URL", config.CloudKDrive, "https://kdrive.infomaniak.com/app/drive/3869957/files", "e@x", "https://3869957.connect.kdrive.infomaniak.com"},
		{"kdrive just id", config.CloudKDrive, "3869957", "e@x", "https://3869957.connect.kdrive.infomaniak.com"},
	}
	for _, c := range cases {
		got, err := buildWebDAVBase(c.kind, c.raw, c.user)
		if err != nil {
			t.Fatalf("%s : erreur inattendue %v", c.name, err)
		}
		if got != c.want {
			t.Errorf("%s : buildWebDAVBase = %q, attendu %q", c.name, got, c.want)
		}
	}
	if _, err := buildWebDAVBase(config.CloudNextcloud, "", "u"); err == nil {
		t.Error("une adresse vide devrait être refusée")
	}
}

// TestResolveKDriveID : l'ID du drive REST doit être le segment après « /drive/ », jamais une
// concaténation de tous les chiffres de l'URL (régression 1.18.2 : « 53843 » + « 3869957 »).
func TestResolveKDriveID(t *testing.T) {
	cases := []struct{ name, raw, want string }{
		{"id nu", "3869957", "3869957"},
		{"id nu avec espaces", "  3869957 ", "3869957"},
		{"url app web ksuite (compte + drive)", "https://ksuite.infomaniak.com/53843/kdrive/app/drive/3869957/", "3869957"},
		{"url app kdrive", "https://kdrive.infomaniak.com/app/drive/3869957/files", "3869957"},
		{"sous-domaine connect", "https://3869957.connect.kdrive.infomaniak.com", "3869957"},
		{"sans scheme", "ksuite.infomaniak.com/53843/kdrive/app/drive/3869957/", "3869957"},
		{"vide", "", ""},
		{"ni id ni url", "mon-drive", ""},
	}
	for _, c := range cases {
		if got := resolveKDriveID(c.raw); got != c.want {
			t.Errorf("%s : resolveKDriveID(%q) = %q, attendu %q", c.name, c.raw, got, c.want)
		}
	}
}

func TestAddFileCloudAndAllowlist(t *testing.T) {
	srv := fakeCloud(t)
	defer srv.Close()

	s := newInTemp(t)
	if err := s.SetPIN("", "123456"); err != nil {
		t.Fatal(err)
	}
	if err := s.Unlock("123456"); err != nil {
		t.Fatal(err)
	}

	// Mauvais identifiant → refus AVANT toute persistance.
	if err := s.AddFileCloud("c", config.CloudNextcloud, srv.URL, "alice", "WRONG"); err == nil {
		t.Fatal("un mot de passe invalide devrait être refusé")
	}
	if len(s.FileClouds()) != 0 {
		t.Fatal("rien ne doit être persisté après un échec de connexion")
	}

	// Type non souverain → refusé.
	if err := s.AddFileCloud("c", "onedrive", srv.URL, "alice", "secret"); err == nil {
		t.Fatal("un type non souverain devrait être refusé")
	}

	if err := s.AddFileCloud("c", config.CloudNextcloud, srv.URL, "alice", "secret"); err != nil {
		t.Fatalf("AddFileCloud : %v", err)
	}
	cs := s.FileClouds()
	if len(cs) != 1 || cs[0].Name != "c" || !cs[0].Default {
		t.Fatalf("connexion mal enregistrée : %+v", cs)
	}

	// Allowlist fail-closed au départ.
	if s.CloudPathAllowed("c", "/Docs") {
		t.Error("allowlist vide : /Docs ne doit pas être autorisé")
	}
	if err := s.SetCloudAllowedDirs("c", []string{"/Docs"}); err != nil {
		t.Fatal(err)
	}
	if !s.CloudPathAllowed("c", "/Docs/a.md") {
		t.Error("/Docs/a.md devrait être autorisé après allowlist")
	}
	if s.CloudPathAllowed("c", "/Autre") {
		t.Error("/Autre ne devrait pas être autorisé")
	}

	// Politique PIN par défaut : write autonome, delete sous PIN.
	if s.RequiresPINCloud("c", config.CloudWrite) {
		t.Error("write ne devrait pas exiger le PIN par défaut")
	}
	if !s.RequiresPINCloud("c", config.CloudDelete) {
		t.Error("delete devrait exiger le PIN par défaut")
	}

	// Suppression de la connexion → secret retiré, plus de connexion.
	if err := s.RemoveFileCloud("c"); err != nil {
		t.Fatal(err)
	}
	if len(s.FileClouds()) != 0 {
		t.Error("la connexion devrait être supprimée")
	}
}
