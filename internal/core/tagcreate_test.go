// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"strings"
	"testing"
)

// TestCreateProjectTag : créer un tag autonome depuis « Gérer les tags » (#20, 1.19) — nom requis,
// pas de doublon (insensible à la casse).
func TestCreateProjectTag(t *testing.T) {
	s := newInTemp(t)
	if err := s.SetPIN("", "123456"); err != nil {
		t.Fatal(err)
	}
	if err := s.Unlock("123456"); err != nil {
		t.Fatal(err)
	}

	if err := s.CreateProjectTag("Clients", "indigo"); err != nil {
		t.Fatalf("CreateProjectTag : %v", err)
	}
	defs := s.TagDefs()
	found := false
	for _, d := range defs {
		if d.Name == "Clients" {
			found = true
		}
	}
	if !found {
		t.Fatalf("le tag créé devrait figurer dans TagDefs : %+v", defs)
	}

	// L'existant fait un UPSERT : recréer le même tag (casse différente) ne doit pas échouer ni
	// dupliquer l'entrée du registre.
	if err := s.CreateProjectTag("clients", "teal"); err != nil {
		t.Fatalf("recréer un tag existant (upsert) ne devrait pas échouer : %v", err)
	}
	n := 0
	for _, d := range s.TagDefs() {
		if strings.EqualFold(d.Name, "clients") {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("le registre ne doit pas dupliquer le tag, trouvé %d entrées", n)
	}

	if err := s.CreateProjectTag("   ", ""); err == nil {
		t.Fatal("un nom vide devrait être refusé")
	}
}
