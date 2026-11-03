// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package config

import (
	"path/filepath"
	"testing"
)

func sample() *Config {
	return &Config{Instances: []Instance{
		{Name: "apcom", URL: "https://git.apcom.app", Default: true},
		{Name: "lab", URL: "https://lab.example"},
	}}
}

func TestFindCaseInsensitive(t *testing.T) {
	c := sample()
	if c.Find("APCOM") == nil {
		t.Error("Find devrait être insensible à la casse")
	}
	if c.Find("inconnu") != nil {
		t.Error("Find d'un nom absent devrait renvoyer nil")
	}
}

func TestSetDefault(t *testing.T) {
	c := sample()
	if !c.SetDefault("LAB") {
		t.Fatal("SetDefault devrait trouver « LAB »")
	}
	if c.DefaultInstance().Name != "lab" {
		t.Errorf("défaut attendu lab, obtenu %s", c.DefaultInstance().Name)
	}
	if c.Instances[0].Default {
		t.Error("l'ancienne défaut devrait être démarquée")
	}
}

func TestRemovePromotesDefault(t *testing.T) {
	c := sample()
	real, ok := c.Remove("apcom") // c'était la défaut
	if !ok || real != "apcom" {
		t.Fatalf("Remove = (%q, %v)", real, ok)
	}
	if len(c.Instances) != 1 || !c.Instances[0].Default {
		t.Error("après suppression de la défaut, la restante devrait être promue défaut")
	}
	if _, ok := c.Remove("absent"); ok {
		t.Error("Remove d'un absent devrait renvoyer false")
	}
}

func TestAddLocalRepoDedup(t *testing.T) {
	c := &Config{}
	c.AddLocalRepo("a/b")
	c.AddLocalRepo(filepath.FromSlash("a/b")) // même chemin, autre écriture
	c.AddLocalRepo("a/b/")                    // slash final
	if len(c.LocalRepos) != 1 {
		t.Errorf("déduplication ratée : %v", c.LocalRepos)
	}
}

// AgentsDocPath (1.17) : la fiche de référence vit dans le dossier de config (survit aux mises à jour),
// nommée AGENTS.md, à côté de config.json.
func TestAgentsDocPath(t *testing.T) {
	p := AgentsDocPath()
	if filepath.Base(p) != "AGENTS.md" {
		t.Fatalf("nom de fichier = %q, attendu AGENTS.md", filepath.Base(p))
	}
	if filepath.Dir(p) != Dir() {
		t.Fatalf("dossier = %q, attendu %q (dossier de config)", filepath.Dir(p), Dir())
	}
	if filepath.Dir(p) != filepath.Dir(Path()) {
		t.Fatalf("la fiche doit être à côté de config.json (%q vs %q)", filepath.Dir(p), filepath.Dir(Path()))
	}
}
