// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"os"
	"path/filepath"
	"testing"

	"situlab/internal/gitx"
)

// TestKernelDesignationAndStatus : désigner un dépôt comme KERNEL, lire son état lisible et sa carte
// de contenu. Reproduit la condition d'un runner sans identité git globale (comme la CI) en passant
// l'identité au dépôt de test.
func TestKernelDesignationAndStatus(t *testing.T) {
	if !gitx.Available() {
		t.Skip("git indisponible")
	}
	dir := t.TempDir()
	gitRun(t, dir, "init", "-b", "main")
	gitRun(t, dir, "config", "user.email", "t@x")
	gitRun(t, dir, "config", "user.name", "T")
	if err := os.MkdirAll(filepath.Join(dir, "EQUIPE"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "LISEZMOI.md"), []byte("note"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "init")

	s := newInTemp(t)
	if err := s.SetPIN("", "123456"); err != nil {
		t.Fatal(err)
	}
	if err := s.Unlock("123456"); err != nil {
		t.Fatal(err)
	}

	// Un dossier non-git est refusé.
	if err := s.AddKernel("x", t.TempDir(), "", ""); err == nil {
		t.Fatal("un dossier sans dépôt git devrait être refusé")
	}

	if err := s.AddKernel("Mémoire", dir, "", ""); err != nil {
		t.Fatalf("AddKernel : %v", err)
	}
	ks := s.Kernels()
	if len(ks) != 1 || !ks[0].Active {
		t.Fatalf("le premier KERNEL devrait être actif : %+v", ks)
	}

	// État lisible (actif par défaut via nom vide) : dépôt propre.
	st, err := s.KernelStatus("")
	if err != nil {
		t.Fatalf("KernelStatus : %v", err)
	}
	if !st.IsRepo || !st.Clean || st.Branch != "main" {
		t.Fatalf("état inattendu : %+v", st)
	}

	// Une modification en cours se traduit en « à enregistrer ».
	if err := os.WriteFile(filepath.Join(dir, "LISEZMOI.md"), []byte("note modifiée"), 0o644); err != nil {
		t.Fatal(err)
	}
	st2, err := s.KernelStatus("Mémoire")
	if err != nil {
		t.Fatal(err)
	}
	if st2.DirtyFiles == 0 {
		t.Error("une modification non enregistrée devrait être comptée")
	}

	// Carte du contenu : le dossier EQUIPE et le fichier, pas le .git.
	ents, err := s.KernelContents("")
	if err != nil {
		t.Fatalf("KernelContents : %v", err)
	}
	var sawDir, sawGit bool
	for _, e := range ents {
		if e.Name == "EQUIPE" && e.Kind == "dossier" {
			sawDir = true
		}
		if e.Name == ".git" {
			sawGit = true
		}
	}
	if !sawDir {
		t.Error("le dossier EQUIPE devrait figurer dans la carte du contenu")
	}
	if sawGit {
		t.Error(".git ne devrait jamais apparaître dans la carte humaine")
	}

	// Retrait.
	if err := s.RemoveKernel("Mémoire"); err != nil {
		t.Fatal(err)
	}
	if len(s.Kernels()) != 0 {
		t.Error("le KERNEL devrait être retiré")
	}
	if _, err := s.KernelStatus(""); err != ErrNoKernel {
		t.Errorf("sans KERNEL, KernelStatus devrait renvoyer ErrNoKernel, a renvoyé %v", err)
	}
}
