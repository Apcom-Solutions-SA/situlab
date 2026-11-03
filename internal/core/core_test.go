// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import "testing"

func TestNormURL(t *testing.T) {
	cases := map[string]string{
		"https://git.apcom.app/apcom/SituLab.git":        "git.apcom.app/apcom/situlab",
		"https://git.apcom.app/apcom/SituLab":            "git.apcom.app/apcom/situlab",
		"ssh://git@git.apcom.app:2222/apcom/SituLab.git": "git.apcom.app/apcom/situlab",
		"git@git.apcom.app:apcom/SituLab.git":            "git.apcom.app/apcom/situlab",
		"HTTPS://Git.Apcom.App/Apcom/SituLab.git":        "git.apcom.app/apcom/situlab",
		"":              "",
		"pas-une-url":   "",
		"https://host/": "",
	}
	for in, want := range cases {
		if got := normURL(in); got != want {
			t.Errorf("normURL(%q) = %q, attendu %q", in, got, want)
		}
	}
}

func TestNormalizeInstanceURL(t *testing.T) {
	ok := map[string]string{
		"https://git.apcom.app":    "https://git.apcom.app",
		"https://git.apcom.app/":   "https://git.apcom.app",
		"git.apcom.app":            "https://git.apcom.app", // schéma ajouté
		"HTTPS://Git.APCOM.App":    "https://git.apcom.app", // hôte en minuscules
		"http://localhost:3000/x":  "http://localhost:3000/x",
		"http://127.0.0.1/":        "http://127.0.0.1",         // loopback : http toléré
		"http://192.168.1.10:3000": "http://192.168.1.10:3000", // IP privée : http toléré
	}
	for in, want := range ok {
		got, err := normalizeInstanceURL(in)
		if err != nil {
			t.Errorf("normalizeInstanceURL(%q) erreur inattendue : %v", in, err)
		}
		if got != want {
			t.Errorf("normalizeInstanceURL(%q) = %q, attendu %q", in, got, want)
		}
	}
	// S4 : http:// refusé pour un hôte public (jeton en clair) ; un suffixe .localhost ne suffit
	// pas à faire passer un hôte pour local (#4).
	for _, bad := range []string{"", "ftp://x", "file:///etc", "   ",
		"http://git.apcom.app", "http://8.8.8.8", "http://evil.localhost"} {
		if _, err := normalizeInstanceURL(bad); err == nil {
			t.Errorf("normalizeInstanceURL(%q) aurait dû échouer", bad)
		}
	}
}

func TestComputeState(t *testing.T) {
	cases := []struct {
		v    RepoView
		want string
	}{
		{RepoView{HasRemote: true}, "nolocal"},
		{RepoView{LocalPath: "x", LocalError: "boom"}, "unreadable"},
		{RepoView{LocalPath: "x", Detached: true}, "detached"},
		{RepoView{LocalPath: "x", Dirty: true}, "dirty"},
		{RepoView{LocalPath: "x", Ahead: 2, Behind: 1}, "diverged"},
		{RepoView{LocalPath: "x", Ahead: 2}, "ahead"},
		{RepoView{LocalPath: "x", Behind: 3}, "behind"},
		{RepoView{LocalPath: "x"}, "uptodate"},
	}
	for _, c := range cases {
		if got := computeState(&c.v); got != c.want {
			t.Errorf("computeState(%+v) = %q, attendu %q", c.v, got, c.want)
		}
	}
}

func TestSyncSummary(t *testing.T) {
	cases := []struct {
		r    SyncResult
		want string
	}{
		{SyncResult{Nothing: true}, "Déjà à jour, rien à synchroniser."},
		{SyncResult{Committed: true}, "Synchronisé : version enregistrée."},
		{SyncResult{Pulled: true, Pushed: true}, "Synchronisé : nouveautés récupérées, envoyé en ligne."},
		{SyncResult{Committed: true, Pulled: true, Pushed: true}, "Synchronisé : version enregistrée, nouveautés récupérées, envoyé en ligne."},
	}
	for _, c := range cases {
		if got := syncSummary(&c.r); got != c.want {
			t.Errorf("syncSummary(%+v) = %q, attendu %q", c.r, got, c.want)
		}
	}
}
