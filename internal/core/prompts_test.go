// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import "testing"

// La bibliothèque de prompts : défauts livrés, surcharge d'un défaut (conservée), masquage réversible,
// et prompts ajoutés par l'utilisateur. Les personnalisations vivent en config (jamais écrasées à la MàJ).
func TestPromptsCRUD(t *testing.T) {
	s := newInTemp(t)
	base := len(defaultPrompts())

	if got := len(s.Prompts()); got != base {
		t.Fatalf("au départ, %d prompts attendus (les défauts), obtenu %d", base, got)
	}

	// Surcharge d'un prompt par défaut : même nombre, mais marqué « overridden ».
	id := defaultPrompts()[0].ID
	if err := s.SavePrompt(id, "Titre modifié", "Corps modifié"); err != nil {
		t.Fatalf("SavePrompt override : %v", err)
	}
	var found bool
	for _, p := range s.Prompts() {
		if p.ID == id {
			found = true
			if !p.Overridden || p.Title != "Titre modifié" {
				t.Errorf("surcharge non appliquée : %+v", p)
			}
		}
	}
	if !found || len(s.Prompts()) != base {
		t.Fatalf("après surcharge : %d prompts (attendu %d)", len(s.Prompts()), base)
	}

	// Réinitialisation : retour au défaut.
	if err := s.ResetPrompt(id); err != nil {
		t.Fatal(err)
	}
	for _, p := range s.Prompts() {
		if p.ID == id && p.Overridden {
			t.Error("après ResetPrompt, le prompt ne doit plus être marqué modifié")
		}
	}

	// Masquage d'un défaut : un de moins, réversible.
	if err := s.DeletePrompt(id); err != nil {
		t.Fatal(err)
	}
	if len(s.Prompts()) != base-1 {
		t.Fatalf("après masquage : %d prompts (attendu %d)", len(s.Prompts()), base-1)
	}
	if err := s.ResetPrompt(id); err != nil {
		t.Fatal(err)
	}
	if len(s.Prompts()) != base {
		t.Fatalf("après démasquage : %d prompts (attendu %d)", len(s.Prompts()), base)
	}

	// Ajout d'un prompt utilisateur (id vide → généré) puis suppression.
	if err := s.SavePrompt("", "Mon prompt", "Fais ceci"); err != nil {
		t.Fatal(err)
	}
	if len(s.Prompts()) != base+1 {
		t.Fatalf("après ajout user : %d prompts (attendu %d)", len(s.Prompts()), base+1)
	}
	var uid string
	for _, p := range s.Prompts() {
		if !p.Builtin {
			uid = p.ID
		}
	}
	if uid == "" {
		t.Fatal("prompt utilisateur introuvable")
	}
	if err := s.DeletePrompt(uid); err != nil {
		t.Fatal(err)
	}
	if len(s.Prompts()) != base {
		t.Fatalf("après suppression user : %d prompts (attendu %d)", len(s.Prompts()), base)
	}
}
