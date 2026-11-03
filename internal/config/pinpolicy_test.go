// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package config

import "testing"

func ptr(b bool) *bool { return &b }

// TestRequiresPINDefaults : les défauts par rôle. Atelier = autonome sur le contenu (push/issues
// sans PIN), PIN sur l'administration. Production = tout sous PIN.
func TestRequiresPINDefaults(t *testing.T) {
	atelier := Instance{Name: "a", Role: RoleAtelier}
	prod := Instance{Name: "p", Role: RoleProduction}
	unset := Instance{Name: "u"} // rôle vide → atelier par défaut

	cases := []struct {
		in   Instance
		act  PINAction
		want bool
	}{
		{atelier, ActionPush, false},
		{atelier, ActionIssues, false},
		{atelier, ActionRepoAdmin, true},
		{atelier, ActionUserAdmin, true},
		{prod, ActionPush, true},
		{prod, ActionIssues, true},
		{prod, ActionRepoAdmin, true},
		{prod, ActionUserAdmin, true},
		{unset, ActionPush, false}, // défaut = atelier
		{unset, ActionRepoAdmin, true},
	}
	for _, c := range cases {
		if got := c.in.RequiresPIN(c.act); got != c.want {
			t.Errorf("%s/%s RequiresPIN = %v, want %v", c.in.NormalizedRole(), c.act, got, c.want)
		}
	}
}

// TestRequiresPINOverride : une politique explicite prime sur le défaut du rôle, champ par champ ;
// un champ nil hérite du défaut.
func TestRequiresPINOverride(t *testing.T) {
	// Atelier mais on exige le PIN pour push (prudent) ; issues restent au défaut (false).
	in := Instance{Role: RoleAtelier, PINPolicy: &PINPolicy{Push: ptr(true)}}
	if !in.RequiresPIN(ActionPush) {
		t.Error("override Push=true doit exiger le PIN")
	}
	if in.RequiresPIN(ActionIssues) {
		t.Error("Issues non défini doit hériter du défaut atelier (false)")
	}
	// Production mais on autorise les issues sans PIN (choix explicite).
	p := Instance{Role: RoleProduction, PINPolicy: &PINPolicy{Issues: ptr(false)}}
	if p.RequiresPIN(ActionIssues) {
		t.Error("override Issues=false doit autoriser sans PIN")
	}
	if !p.RequiresPIN(ActionPush) {
		t.Error("Push non défini doit hériter du défaut production (true)")
	}
}
