// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"os"
	"path/filepath"
	"testing"

	"situlab/internal/gitx"
)

// TestScanFolderRepos : scanner UN dossier choisi découvre ses dépôts git (y compris en sous-dossier),
// ignore un dossier sans dépôt, marque « déjà suivi », et classe les non suivis en tête. (#12, 1.19)
func TestScanFolderRepos(t *testing.T) {
	if !gitx.Available() {
		t.Skip("git indisponible")
	}
	root := t.TempDir()
	alpha := filepath.Join(root, "alpha")
	beta := filepath.Join(root, "sub", "beta")
	plain := filepath.Join(root, "plain")
	for _, d := range []string{alpha, beta, plain} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, d := range []string{alpha, beta} {
		gitRun(t, d, "init", "-b", "main")
		gitRun(t, d, "config", "user.email", "t@x")
		gitRun(t, d, "config", "user.name", "T")
		gitRun(t, d, "commit", "--allow-empty", "-m", "init")
	}

	s := newInTemp(t)
	if err := s.SetPIN("", "123456"); err != nil {
		t.Fatal(err)
	}
	if err := s.Unlock("123456"); err != nil {
		t.Fatal(err)
	}

	hits, err := s.ScanFolderRepos(root)
	if err != nil {
		t.Fatalf("ScanFolderRepos : %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("attendu 2 dépôts, obtenu %d : %+v", len(hits), hits)
	}
	for _, h := range hits {
		if h.Known {
			t.Fatalf("aucun dépôt suivi avant AddLocalRepo : %+v", h)
		}
	}

	if err := s.AddLocalRepo(alpha); err != nil {
		t.Fatalf("AddLocalRepo : %v", err)
	}
	hits2, err := s.ScanFolderRepos(root)
	if err != nil {
		t.Fatal(err)
	}
	// Non suivis en tête : beta (non suivi) avant alpha (désormais suivi).
	if len(hits2) != 2 || hits2[0].Known || !hits2[1].Known {
		t.Fatalf("classement/suivi incorrects : %+v", hits2)
	}

	if _, err := s.ScanFolderRepos(filepath.Join(root, "n-existe-pas")); err == nil {
		t.Fatal("un dossier inexistant devrait être refusé")
	}
}
