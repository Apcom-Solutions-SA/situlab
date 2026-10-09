// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package config

import "testing"

// TestProjectRoundTrip : un projet (atelier + productions + marqueurs) se retrouve par nom, insensible
// à la casse, et RepoRef.Key est canonique.
func TestProjectRoundTrip(t *testing.T) {
	c := &Config{Projects: []Project{{
		Name:        "Site",
		Atelier:     RepoRef{Instance: "forge", Owner: "acme", Repo: "site"},
		Productions: []RepoRef{{Instance: "gh", Owner: "acme", Repo: "site-prod"}},
		Promotions:  map[string]Promo{"gh/acme/site-prod": {Version: "v1.2", Date: "2026-10-09T00:00:00Z"}},
	}}}

	if c.FindProject("site") == nil {
		t.Error("FindProject devrait être insensible à la casse")
	}
	if c.FindProject("inconnu") != nil {
		t.Error("FindProject d'un nom absent devrait être nil")
	}
	p := c.FindProject("Site")
	if got := p.Productions[0].Key(); got != "gh/acme/site-prod" {
		t.Errorf("RepoRef.Key = %q, attendu gh/acme/site-prod", got)
	}
	if p.Promotions[p.Productions[0].Key()].Version != "v1.2" {
		t.Error("le marqueur de promotion devrait être retrouvé par la clé de la production")
	}
}
