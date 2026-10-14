// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"os"
	"path/filepath"
	"testing"
)

// TestReadGitRemote : l'URL « origin » prime ; à défaut le premier distant ; "" si aucun.
func TestReadGitRemote(t *testing.T) {
	cases := []struct {
		name, cfg, want string
	}{
		{"origin", "[remote \"origin\"]\n\turl = https://git.apcom.app/LiG/KERNEL\n", "https://git.apcom.app/LiG/KERNEL"},
		{"origin-prime-sur-autre", "[remote \"upstream\"]\n\turl = https://x/y\n[remote \"origin\"]\n\turl = https://git/ok\n", "https://git/ok"},
		{"premier-si-pas-origin", "[remote \"upstream\"]\n\turl = https://x/y\n", "https://x/y"},
		{"aucun", "[core]\n\tbare = false\n", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, ".git", "config"), []byte(c.cfg), 0o644); err != nil {
				t.Fatal(err)
			}
			if got := readGitRemote(dir); got != c.want {
				t.Fatalf("readGitRemote = %q, veut %q", got, c.want)
			}
		})
	}
	// Dossier sans .git : pas de distant, pas de panique.
	if got := readGitRemote(t.TempDir()); got != "" {
		t.Fatalf("sans .git, attendu \"\", obtenu %q", got)
	}
}
