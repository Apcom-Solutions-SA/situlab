// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"situlab/internal/gitx"
)

// TestPushPreview : l'aperçu de consentement distingue les fichiers NOUVEAUX (non suivis) des
// fichiers modifiés et liste les commits à envoyer — socle du consentement éclairé v1.7.
func TestPushPreview(t *testing.T) {
	if !gitx.Available() {
		t.Skip("git indisponible")
	}
	s := newInTemp(t)
	dir := t.TempDir()
	run := func(args ...string) {
		c := exec.Command("git", args...)
		c.Dir = dir
		c.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v : %v\n%s", args, err, out)
		}
	}
	run("init")
	run("config", "user.email", "t@t.t")
	run("config", "user.name", "T")
	run("config", "commit.gpgsign", "false")
	os.WriteFile(filepath.Join(dir, "suivi.txt"), []byte("v1\n"), 0o644)
	run("add", "-A")
	run("commit", "-m", "init")
	// Un commit de plus (à envoyer), un fichier suivi modifié, un fichier NOUVEAU non suivi.
	os.WriteFile(filepath.Join(dir, "suivi.txt"), []byte("v1\nv2\n"), 0o644)
	run("commit", "-am", "ajoute v2")
	os.WriteFile(filepath.Join(dir, "suivi.txt"), []byte("v1\nv2\nwip\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "nouveau.txt"), []byte("secret?\n"), 0o644)

	p := s.PushPreview(dir)
	if p == nil {
		t.Fatal("PushPreview nil")
	}
	hasUntracked := false
	for _, u := range p.Untracked {
		if strings.Contains(u, "nouveau.txt") {
			hasUntracked = true
		}
	}
	if !hasUntracked {
		t.Errorf("le fichier nouveau devrait figurer dans Untracked : %+v", p.Untracked)
	}
	for _, u := range p.Untracked {
		if strings.Contains(u, "suivi.txt") {
			t.Error("un fichier suivi ne doit pas être listé comme non suivi")
		}
	}
	// Sans amont configuré, UnpushedSubjects renvoie les derniers commits (dont « ajoute v2 »).
	foundCommit := false
	for _, c := range p.Commits {
		if strings.Contains(c, "ajoute v2") {
			foundCommit = true
		}
	}
	if !foundCommit {
		t.Errorf("le commit « ajoute v2 » devrait apparaître dans l'aperçu : %+v", p.Commits)
	}
}
