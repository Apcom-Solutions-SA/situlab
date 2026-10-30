// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"testing"

	"situlab/internal/config"
)

// TestConfirmVitrine : confirmer une vitrine suggérée crée le projet au besoin, l'ajoute sans doublon,
// et un second appel sur le même atelier enrichit le projet existant. (#16, 1.19)
func TestConfirmVitrine(t *testing.T) {
	s := newInTemp(t)
	if err := s.SetPIN("", "123456"); err != nil {
		t.Fatal(err)
	}
	if err := s.Unlock("123456"); err != nil {
		t.Fatal(err)
	}

	atl := config.RepoRef{Instance: "git apcom", Owner: "apcom", Repo: "glim"}
	v1 := config.RepoRef{Instance: "github apcom", Owner: "Apcom-Solutions-SA", Repo: "glim_v2"}
	v2 := config.RepoRef{Instance: "github apcom", Owner: "Apcom-Solutions-SA", Repo: "glim_mirror"}

	// Aucun projet : ConfirmVitrine le crée avec la vitrine.
	if err := s.ConfirmVitrine(atl, v1); err != nil {
		t.Fatalf("ConfirmVitrine (création) : %v", err)
	}
	p := findProj(s.Projects(), "glim")
	if p == nil || len(p.Productions) != 1 || p.Productions[0].Repo != "glim_v2" {
		t.Fatalf("projet attendu avec glim_v2, obtenu %+v", p)
	}

	// Rappel idempotent : pas de doublon.
	if err := s.ConfirmVitrine(atl, v1); err != nil {
		t.Fatal(err)
	}
	if p := findProj(s.Projects(), "glim"); p == nil || len(p.Productions) != 1 {
		t.Fatalf("pas de doublon attendu, obtenu %+v", p)
	}

	// Seconde vitrine : ajoutée au projet existant.
	if err := s.ConfirmVitrine(atl, v2); err != nil {
		t.Fatal(err)
	}
	if p := findProj(s.Projects(), "glim"); p == nil || len(p.Productions) != 2 {
		t.Fatalf("deux vitrines attendues, obtenu %+v", p)
	}

	// Atelier incomplet : refusé.
	if err := s.ConfirmVitrine(config.RepoRef{Owner: "x"}, v1); err == nil {
		t.Fatal("un atelier incomplet devrait être refusé")
	}
}

func findProj(ps []config.Project, name string) *config.Project {
	for i := range ps {
		if ps[i].Name == name {
			return &ps[i]
		}
	}
	return nil
}
