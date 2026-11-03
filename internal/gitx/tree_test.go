// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package gitx

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestLocalTreeAtAndDates(t *testing.T) {
	dir := newRepo(t) // contient a.txt commité
	git := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "b.txt"), []byte("deux\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "-A")
	git("commit", "-m", "ajout sub/b.txt")

	// Racine : on voit a.txt (file) et sub (dir).
	ents, err := LocalTreeAt(dir, "")
	if err != nil {
		t.Fatalf("LocalTreeAt racine: %v", err)
	}
	var sawFile, sawDir bool
	for _, e := range ents {
		if e.Name == "a.txt" && e.Type == "file" {
			sawFile = true
		}
		if e.Name == "sub" && e.Type == "dir" {
			sawDir = true
		}
	}
	if !sawFile || !sawDir {
		t.Fatalf("racine incomplète: %+v", ents)
	}

	// Sous-dossier : on voit b.txt avec sa taille.
	sub, err := LocalTreeAt(dir, "sub")
	if err != nil {
		t.Fatalf("LocalTreeAt sub: %v", err)
	}
	if len(sub) != 1 || sub[0].Path != "sub/b.txt" || sub[0].Size == 0 {
		t.Fatalf("sous-dossier inattendu: %+v", sub)
	}

	// Dates : au moins a.txt a une date de dernier commit.
	d := LocalTreeDates(dir, []string{"a.txt", "sub/b.txt"})
	if d["a.txt"] == "" || d["sub/b.txt"] == "" {
		t.Fatalf("dates manquantes: %+v", d)
	}
}

func TestLocalTreeTotals(t *testing.T) {
	dir := newRepo(t)
	tt, err := LocalTreeTotals(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if tt.Total <= 0 {
		t.Fatalf("total attendu > 0, obtenu %d", tt.Total)
	}
}
