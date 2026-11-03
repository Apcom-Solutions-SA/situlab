// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package gitx

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// newRepo crée un dépôt git jetable avec un 1er commit, pour tester les opérations locales
// sur un vrai dépôt (sans réseau). Skippe si git n'est pas installé.
func newRepo(t *testing.T) string {
	t.Helper()
	if !Available() {
		t.Skip("git indisponible")
	}
	dir := t.TempDir()
	runGit := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s : %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	runGit("init")
	runGit("config", "user.email", "t@t.t")
	runGit("config", "user.name", "T")
	runGit("config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("un\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit("add", "-A")
	runGit("commit", "-m", "init")
	return dir
}

func TestRepoStatusCommitDiff(t *testing.T) {
	dir := newRepo(t)

	st, err := StatusOf(dir)
	if err != nil || !st.IsRepo {
		t.Fatalf("StatusOf : %v (isRepo=%v)", err, st.IsRepo)
	}
	if !st.Clean {
		t.Fatalf("un dépôt fraîchement commité devrait être propre : %+v", st)
	}
	if st.Branch == "" {
		t.Error("branche vide")
	}

	// Modification non enregistrée → dirty + diff non vide.
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("un\ndeux\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, _ = StatusOf(dir)
	if st.Clean {
		t.Fatal("devrait être sale après modification")
	}
	d, err := Diff(dir)
	if err != nil || !strings.Contains(d, "deux") {
		t.Fatalf("Diff devrait montrer l'ajout : %v / %q", err, d)
	}

	// Commit via gitx (add -A + commit) → de nouveau propre.
	if _, err := Commit(dir, "ajoute deux"); err != nil {
		t.Fatalf("Commit : %v", err)
	}
	if st, _ := StatusOf(dir); !st.Clean {
		t.Fatal("devrait être propre après Commit")
	}
	if cs := Log(dir, 10); len(cs) < 2 {
		t.Fatalf("Log devrait avoir ≥2 commits, obtenu %d", len(cs))
	}
}

func TestRepoBranchCreateSwitch(t *testing.T) {
	dir := newRepo(t)
	if _, err := CreateBranch(dir, "feature-x"); err != nil {
		t.Fatalf("CreateBranch : %v", err)
	}
	if st, _ := StatusOf(dir); st.Branch != "feature-x" {
		t.Fatalf("devrait être sur feature-x, obtenu %q", st.Branch)
	}
	// Nom dangereux refusé (anti-injection d'options).
	if _, err := CreateBranch(dir, "--force"); err == nil {
		t.Error("CreateBranch('--force') devrait être refusé")
	}
}

// TestConflictDetection : un conflit de fusion réel doit être vu par StatusOf (Conflicted non vide,
// Clean=false). C'est le socle du garde-fou : Synchroniser refuse d'enregistrer des fichiers
// en conflit.
func TestConflictDetection(t *testing.T) {
	dir := newRepo(t)
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s : %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	// Branche A : modifie a.txt.
	run("switch", "-c", "a")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("version A\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("commit", "-am", "A")
	// Branche B depuis le 1er commit : modifie la MÊME ligne autrement.
	run("switch", "-c", "b", "HEAD~1")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("version B\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("commit", "-am", "B")
	// Fusion de A dans B → conflit sur a.txt (on ignore le code de sortie non nul).
	mg := exec.Command("git", "merge", "a")
	mg.Dir = dir
	mg.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	_ = mg.Run()

	st, err := StatusOf(dir)
	if err != nil {
		t.Fatalf("StatusOf : %v", err)
	}
	if len(st.Conflicted) == 0 {
		t.Fatalf("un conflit de fusion devrait être détecté (Conflicted) : %+v", st)
	}
	if st.Clean {
		t.Error("un arbre en conflit n'est pas propre")
	}
}

func TestRepoStashRoundtrip(t *testing.T) {
	dir := newRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("un\nmodif\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := StashSave(dir, "wip"); err != nil {
		t.Fatalf("StashSave : %v", err)
	}
	if st, _ := StatusOf(dir); !st.Clean {
		t.Fatal("après stash, l'arbre devrait être propre")
	}
	if _, err := StashPop(dir); err != nil {
		t.Fatalf("StashPop : %v", err)
	}
	if st, _ := StatusOf(dir); st.Clean {
		t.Fatal("après pop, la modification devrait être revenue")
	}
}
