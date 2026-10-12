// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package gitx

import (
	"os"
	"path/filepath"
	"testing"
)

// repoWithCommit prépare un dépôt avec un commit initial sur la branche « main » (nom forcé pour que
// l'intégration soit déterministe quelle que soit la config git locale).
func repoWithCommit(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitExec(t, dir, "init")
	gitExec(t, dir, "config", "user.name", "T")
	gitExec(t, dir, "config", "user.email", "t@t")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("1"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitExec(t, dir, "add", "-A")
	gitExec(t, dir, "commit", "-m", "c1")
	gitExec(t, dir, "branch", "-M", "main")
	return dir
}

// TestDeleteBranchMergeGuard : git refuse de supprimer une branche NON fusionnée sans force (garde-fou
// anti-perte), et l'accepte avec force ; une branche fusionnée se supprime sans force.
func TestDeleteBranchMergeGuard(t *testing.T) {
	if !Available() {
		t.Skip("git indisponible")
	}
	dir := repoWithCommit(t)

	// Branche fusionnée (pointe = main) : suppression simple acceptée.
	gitExec(t, dir, "branch", "done")
	if BranchMergedInto(dir, "done", "main") != true {
		t.Error("« done » (pointe = main) devrait être fusionnée dans main")
	}
	if _, err := DeleteBranch(dir, "done", false); err != nil {
		t.Fatalf("suppression d'une branche fusionnée refusée à tort : %v", err)
	}

	// Branche NON fusionnée : commit propre, puis retour sur main.
	gitExec(t, dir, "checkout", "-b", "feat")
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("2"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitExec(t, dir, "add", "-A")
	gitExec(t, dir, "commit", "-m", "travail feat")
	gitExec(t, dir, "checkout", "main")

	if BranchMergedInto(dir, "feat", "main") {
		t.Error("« feat » ne devrait PAS être fusionnée dans main")
	}
	if _, err := DeleteBranch(dir, "feat", false); err == nil {
		t.Error("git aurait dû refuser la suppression d'une branche non fusionnée sans force")
	}
	if _, err := DeleteBranch(dir, "feat", true); err != nil {
		t.Fatalf("suppression forcée échouée : %v", err)
	}
}

// TestMatrixMergedField : MatrixOf renseigne le champ Merged (fusionnée dans l'intégration main).
func TestMatrixMergedField(t *testing.T) {
	if !Available() {
		t.Skip("git indisponible")
	}
	dir := repoWithCommit(t)
	gitExec(t, dir, "branch", "done") // pointe sur main → fusionnée
	gitExec(t, dir, "checkout", "-b", "feat")
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("2"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitExec(t, dir, "add", "-A")
	gitExec(t, dir, "commit", "-m", "travail feat") // feat non fusionnée
	gitExec(t, dir, "checkout", "main")

	m, err := MatrixOf(dir)
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]BranchRow{}
	for _, r := range m.Branches {
		by[r.Name] = r
	}
	if !by["main"].Merged {
		t.Error("main devrait être marquée fusionnée (intégration elle-même)")
	}
	if !by["done"].Merged {
		t.Error("done (pointe = main) devrait être marquée fusionnée")
	}
	if by["feat"].Merged {
		t.Error("feat (travail propre) ne devrait PAS être marquée fusionnée")
	}
}

// TestWorktreeRoundtrip : ajouter un worktree (nouvelle branche), le voir dans la liste, puis le retirer.
func TestWorktreeRoundtrip(t *testing.T) {
	if !Available() {
		t.Skip("git indisponible")
	}
	dir := repoWithCommit(t)
	wt := filepath.Join(t.TempDir(), "wt-side") // parent existe, le dossier worktree non (git le crée)

	if _, err := WorktreeAdd(dir, wt, "side", true); err != nil {
		t.Fatalf("WorktreeAdd : %v", err)
	}
	list, err := WorktreeList(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("attendu 2 worktrees (principal + side), obtenu %d : %+v", len(list), list)
	}
	if !list[0].Main {
		t.Error("le premier worktree listé doit être le principal")
	}
	found := false
	for _, w := range list {
		if w.Branch == "side" {
			found = true
		}
	}
	if !found {
		t.Errorf("le worktree de la branche « side » est absent de la liste : %+v", list)
	}

	if _, err := WorktreeRemove(dir, wt, false); err != nil {
		t.Fatalf("WorktreeRemove : %v", err)
	}
	list2, err := WorktreeList(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(list2) != 1 {
		t.Fatalf("après retrait, attendu 1 worktree, obtenu %d : %+v", len(list2), list2)
	}
}
