// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package config

import "testing"

func TestKernelHelpers(t *testing.T) {
	c := &Config{}
	c.Kernels = []Kernel{{Name: "Mémoire", LocalPath: "/a"}, {Name: "Autre", LocalPath: "/b"}}
	c.ActiveKernel = "Mémoire"

	// FindKernel insensible à la casse.
	if c.FindKernel("mémoire") == nil {
		t.Fatal("FindKernel devrait trouver « mémoire »")
	}
	if c.FindKernel("inconnu") != nil {
		t.Fatal("FindKernel ne devrait rien renvoyer pour un nom inconnu")
	}

	// ResolveKernel : vide => actif.
	if k := c.ResolveKernel(""); k == nil || k.Name != "Mémoire" {
		t.Fatalf("ResolveKernel(\"\") devrait renvoyer l'actif, a renvoyé %+v", k)
	}

	// SetActiveKernel bascule l'actif ; nom inconnu échoue.
	if !c.SetActiveKernel("Autre") || c.ActiveKernel != "Autre" {
		t.Fatal("SetActiveKernel(Autre) devrait réussir")
	}
	if c.SetActiveKernel("rien") {
		t.Fatal("SetActiveKernel sur un nom inconnu devrait échouer")
	}

	// RemoveKernel de l'actif → promeut le premier restant.
	if !c.RemoveKernel("Autre") {
		t.Fatal("RemoveKernel(Autre) devrait réussir")
	}
	if len(c.Kernels) != 1 || c.ActiveKernel != "Mémoire" {
		t.Fatalf("après retrait de l'actif, l'actif devrait être promu : %q (%d restants)", c.ActiveKernel, len(c.Kernels))
	}

	// Retrait du dernier → plus d'actif.
	if !c.RemoveKernel("Mémoire") || c.ActiveKernel != "" || len(c.Kernels) != 0 {
		t.Fatalf("après retrait du dernier KERNEL, aucun actif attendu : %q (%d)", c.ActiveKernel, len(c.Kernels))
	}
}
