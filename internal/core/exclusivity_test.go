// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"strings"
	"testing"

	"situlab/internal/gitx"
)

// TestKernelSpokeRoleExclusivity : un dépôt est KERNEL ou spoke, jamais les deux (1.19). Tant qu'un
// rôle est posé, l'autre est refusé ; il faut d'abord retirer le rôle en cours pour basculer.
func TestKernelSpokeRoleExclusivity(t *testing.T) {
	if !gitx.Available() {
		t.Skip("git indisponible")
	}
	dir := t.TempDir()
	gitRun(t, dir, "init", "-b", "main")
	gitRun(t, dir, "config", "user.email", "t@x")
	gitRun(t, dir, "config", "user.name", "T")
	gitRun(t, dir, "commit", "--allow-empty", "-m", "init")

	s := newInTemp(t)
	if err := s.SetPIN("", "123456"); err != nil {
		t.Fatal(err)
	}
	if err := s.Unlock("123456"); err != nil {
		t.Fatal(err)
	}

	// Désigné KERNEL → refus de le désigner spoke.
	if err := s.AddKernel("mem", dir, "", ""); err != nil {
		t.Fatalf("AddKernel : %v", err)
	}
	if err := s.AddSpoke("mission", dir, "mem", "", ""); err == nil {
		t.Fatal("un dépôt déjà KERNEL ne devrait pas pouvoir devenir spoke")
	} else if !strings.Contains(err.Error(), "déjà désigné comme KERNEL") {
		t.Fatalf("message d'exclusivité attendu, obtenu : %v", err)
	}

	// On retire le KERNEL → le spoke devient possible.
	if err := s.RemoveKernel("mem"); err != nil {
		t.Fatalf("RemoveKernel : %v", err)
	}
	if err := s.AddSpoke("mission", dir, "", "", ""); err != nil {
		t.Fatalf("AddSpoke après retrait du KERNEL : %v", err)
	}

	// Désormais spoke → refus de le désigner KERNEL.
	if err := s.AddKernel("mem2", dir, "", ""); err == nil {
		t.Fatal("un dépôt déjà spoke ne devrait pas pouvoir devenir KERNEL")
	} else if !strings.Contains(err.Error(), "déjà désigné comme spoke") {
		t.Fatalf("message d'exclusivité attendu, obtenu : %v", err)
	}
}
