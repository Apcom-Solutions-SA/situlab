// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"testing"

	"situlab/internal/config"
	"situlab/internal/gitx"
)

// TestLocalRemoteIndex : l'index remote->chemin recense chaque clone local suivi en UN seul balayage
// (remplace la boucle O(projets × depots) de localCloneFor dans ProjectBoards).
func TestLocalRemoteIndex(t *testing.T) {
	if !gitx.Available() {
		t.Skip("git indisponible")
	}
	s := newInTemp(t)
	a := t.TempDir()
	gitRun(t, a, "init")
	gitRun(t, a, "remote", "add", "origin", "https://forge.example/o/a.git")
	b := t.TempDir()
	gitRun(t, b, "init")
	gitRun(t, b, "remote", "add", "origin", "https://forge.example/o/b.git")

	cfg, _ := config.Load()
	cfg.LocalRepos = []string{a, b}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	idx := s.localRemoteIndex()
	if got := idx[normURL("https://forge.example/o/a.git")]; got != a {
		t.Errorf("clone a non indexe : got=%q idx=%v", got, idx)
	}
	if got := idx[normURL("https://forge.example/o/b.git")]; got != b {
		t.Errorf("clone b non indexe : got=%q idx=%v", got, idx)
	}
	// Un remote inconnu n'a pas d'entree.
	if got := idx[normURL("https://forge.example/o/zzz.git")]; got != "" {
		t.Errorf("remote inconnu ne devrait pas etre indexe : %q", got)
	}
}

// TestLocalCloneForRefOffline : la resolution du clone local se fait SANS reseau (repli par suffixe
// owner/repo quand l'instance n'est pas configuree) — socle de la page d'accueil instantanee.
func TestLocalCloneForRefOffline(t *testing.T) {
	if !gitx.Available() {
		t.Skip("git indisponible")
	}
	s := newInTemp(t)
	a := t.TempDir()
	gitRun(t, a, "init")
	gitRun(t, a, "remote", "add", "origin", "https://forge.example/acme/widget.git")
	cfg, _ := config.Load()
	cfg.LocalRepos = []string{a}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	idx := s.localRemoteIndex()
	if got := s.localCloneForRefOffline(config.RepoRef{Instance: "forge", Owner: "acme", Repo: "widget"}, idx); got != a {
		t.Errorf("resolution hors-ligne : got %q, want %q", got, a)
	}
	if got := s.localCloneForRefOffline(config.RepoRef{Owner: "acme", Repo: "autre"}, idx); got != "" {
		t.Errorf("depot inconnu ne devrait pas resoudre : %q", got)
	}
}

// TestProjectsLight : l'accueil leger renvoie identite + nombre de vitrines, sans reseau.
func TestProjectsLight(t *testing.T) {
	s := newInTemp(t)
	cfg, _ := config.Load()
	cfg.Projects = []config.Project{{
		Name:        "P1",
		Atelier:     config.RepoRef{Instance: "x", Owner: "o", Repo: "r"},
		Productions: []config.RepoRef{{Instance: "x", Owner: "o", Repo: "prod"}},
	}}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	light := s.ProjectsLight()
	if len(light) != 1 || light[0].Name != "P1" || light[0].Vitrines != 1 || light[0].Atelier.Repo != "r" {
		t.Errorf("ProjectsLight inattendu : %+v", light)
	}
}
