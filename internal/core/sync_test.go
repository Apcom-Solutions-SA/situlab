// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"situlab/internal/gitx"
)

// gitRepoNoRemote crée un dépôt git jetable avec un 1er commit et SANS distant.
func gitRepoNoRemote(t *testing.T) string {
	t.Helper()
	if !gitx.Available() {
		t.Skip("git indisponible")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s : %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	run("init")
	run("config", "user.email", "t@t.t")
	run("config", "user.name", "T")
	run("config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("un\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-m", "init")
	return dir
}

// TestSynchronizeGuards vérifie les branches de garde de Synchronize (avant tout accès réseau) :
// PIN requis, dossier non suivi, tête détachée, dépôt sans distant — chacune doit renvoyer son
// erreur guidée stable plutôt qu'une sortie git brute.
func TestSynchronizeGuards(t *testing.T) {
	if !gitx.Available() {
		t.Skip("git indisponible")
	}
	s := newInTemp(t)
	if err := s.SetPIN("", "123456"); err != nil {
		t.Fatalf("SetPIN : %v", err)
	}

	// PIN vide → ErrPINNeeded, avant toute opération git.
	if _, err := s.Synchronize("x", t.TempDir(), "", ""); !errors.Is(err, ErrPINNeeded) {
		t.Errorf("PIN vide : attendu ErrPINNeeded, obtenu %v", err)
	}

	// Dossier qui n'est pas un dépôt → ErrNotARepo.
	if _, err := s.Synchronize("x", t.TempDir(), "", "123456"); !errors.Is(err, ErrNotARepo) {
		t.Errorf("non-dépôt : attendu ErrNotARepo, obtenu %v", err)
	}

	// Dépôt sans distant → ErrNoRemote.
	repo := gitRepoNoRemote(t)
	if _, err := s.Synchronize("x", repo, "", "123456"); !errors.Is(err, ErrNoRemote) {
		t.Errorf("sans distant : attendu ErrNoRemote, obtenu %v", err)
	}

	// Tête détachée → ErrDetached (prime sur l'absence de distant).
	detach := exec.Command("git", "checkout", "--detach", "HEAD")
	detach.Dir = repo
	if out, err := detach.CombinedOutput(); err != nil {
		t.Fatalf("git checkout --detach : %v\n%s", err, out)
	}
	if _, err := s.Synchronize("x", repo, "", "123456"); !errors.Is(err, ErrDetached) {
		t.Errorf("tête détachée : attendu ErrDetached, obtenu %v", err)
	}
}
