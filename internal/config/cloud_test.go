// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package config

import "testing"

func TestCloudCleanPath(t *testing.T) {
	cases := map[string]string{
		"":                 "/",
		"/":                "/",
		"Documents":        "/Documents",
		"/Documents/":      "/Documents",
		"\\Documents\\a\\": "/Documents/a",
		"/a/../b":          "/b",
		"/a/./b/":          "/a/b",
	}
	for in, want := range cases {
		if got := CloudCleanPath(in); got != want {
			t.Errorf("CloudCleanPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPathAllowedFailClosed(t *testing.T) {
	// Allowlist vide : l'agent ne voit RIEN.
	empty := FileCloud{Name: "x"}
	if empty.PathAllowed("/Documents") {
		t.Error("allowlist vide devrait tout refuser")
	}

	fc := FileCloud{Name: "x", AllowedDirs: []string{"/Documents/CORPUS"}}
	ok := []string{"/Documents/CORPUS", "/Documents/CORPUS/sub", "/Documents/CORPUS/a/b.md"}
	for _, p := range ok {
		if !fc.PathAllowed(p) {
			t.Errorf("devrait autoriser %q", p)
		}
	}
	ko := []string{"/Documents", "/Documents/Autre", "/", "/Documents/CORPUS-bis"}
	for _, p := range ko {
		if fc.PathAllowed(p) {
			t.Errorf("ne devrait PAS autoriser %q", p)
		}
	}

	// Un dossier autorisé « / » ouvre tout le cloud.
	root := FileCloud{AllowedDirs: []string{"/"}}
	if !root.PathAllowed("/n/importe/quoi") {
		t.Error("allowlist « / » devrait tout autoriser")
	}
}

func TestPathAllowedAllowDeny(t *testing.T) {
	// Tout le CORPUS autorisé SAUF un sous-dossier retiré, lui-même ré-autorisé plus finement.
	fc := FileCloud{
		AllowedDirs: []string{"/", "/Privé/Partagé"},
		DeniedDirs:  []string{"/Privé"},
	}
	allow := []string{"/", "/Docs/a.md", "/Privé/Partagé", "/Privé/Partagé/x.md"}
	for _, p := range allow {
		if !fc.PathAllowed(p) {
			t.Errorf("devrait autoriser %q", p)
		}
	}
	deny := []string{"/Privé", "/Privé/Secret", "/Privé/Secret/k.md"}
	for _, p := range deny {
		if fc.PathAllowed(p) {
			t.Errorf("ne devrait PAS autoriser %q", p)
		}
	}

	// À spécificité égale, l'exclusion l'emporte.
	tie := FileCloud{AllowedDirs: []string{"/A"}, DeniedDirs: []string{"/A"}}
	if tie.PathAllowed("/A/x") {
		t.Error("à égalité, l'exclusion doit gagner")
	}
}

func TestRequiresPINCloudDefaults(t *testing.T) {
	fc := FileCloud{} // défauts : write autonome, delete sous PIN
	if fc.RequiresPINCloud(CloudWrite) {
		t.Error("write ne devrait PAS exiger le PIN par défaut")
	}
	if !fc.RequiresPINCloud(CloudDelete) {
		t.Error("delete DEVRAIT exiger le PIN par défaut")
	}

	yes, no := true, false
	fc.PINPolicy = &CloudPINPolicy{Write: &yes, Delete: &no}
	if !fc.RequiresPINCloud(CloudWrite) {
		t.Error("write explicitement sous PIN non respecté")
	}
	if fc.RequiresPINCloud(CloudDelete) {
		t.Error("delete explicitement sans PIN non respecté")
	}
}

func TestFindRemoveCloudAndBackup(t *testing.T) {
	c := &Config{
		FileClouds: []FileCloud{{Name: "Nuage", Default: true}, {Name: "Autre"}},
		Backups:    []BackupPlan{{Name: "k"}},
	}
	if c.FindCloud("nuage") == nil { // insensible à la casse
		t.Error("FindCloud insensible à la casse attendu")
	}
	if real, ok := c.RemoveCloud("NUAGE"); !ok || real != "Nuage" {
		t.Errorf("RemoveCloud = %q,%v", real, ok)
	}
	if !c.FileClouds[0].Default {
		t.Error("le défaut devrait être promu au survivant")
	}
	if !c.RemoveBackup("K") || c.FindBackup("k") != nil {
		t.Error("RemoveBackup devrait retirer le plan")
	}
}
