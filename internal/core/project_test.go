// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	"situlab/internal/config"
	"situlab/internal/gitx"
)

// TestSaveProject : création, gardes, renommage, collision de nom et nettoyage des marqueurs de
// promotion orphelins. Aucune de ces branches ne touche au réseau ni à git (assertWritable ne
// dépend que du mode lecture seule, faux par défaut).
func TestSaveProject(t *testing.T) {
	atelier := config.RepoRef{Instance: "forge", Owner: "acme", Repo: "site"}

	t.Run("création puis renommage", func(t *testing.T) {
		s := newInTemp(t)
		if err := s.SaveProject("", "Site", "", atelier, nil); err != nil {
			t.Fatalf("création : %v", err)
		}
		if ps := s.Projects(); len(ps) != 1 || ps[0].Name != "Site" {
			t.Fatalf("après création, attendu un seul projet « Site », obtenu %+v", ps)
		}
		// Renommer : on retrouve le projet par son ancien nom, pas de doublon.
		if err := s.SaveProject("Site", "Vitrine", "", atelier, nil); err != nil {
			t.Fatalf("renommage : %v", err)
		}
		if ps := s.Projects(); len(ps) != 1 {
			t.Fatalf("le renommage ne doit pas créer de doublon : %+v", ps)
		}
		cfg, _ := config.Load()
		if cfg.FindProject("Vitrine") == nil {
			t.Error("le nouveau nom « Vitrine » devrait exister")
		}
		if cfg.FindProject("Site") != nil {
			t.Error("l'ancien nom « Site » ne devrait plus exister")
		}
	})

	t.Run("gardes", func(t *testing.T) {
		s := newInTemp(t)
		if err := s.SaveProject("", "", "", atelier, nil); err == nil {
			t.Error("un nom vide devrait être refusé")
		}
		incomplet := config.RepoRef{Instance: "forge", Owner: "", Repo: "site"}
		if err := s.SaveProject("", "X", "", incomplet, nil); err == nil {
			t.Error("un atelier incomplet (owner vide) devrait être refusé")
		}
	})

	t.Run("collision de nom", func(t *testing.T) {
		s := newInTemp(t)
		if err := s.SaveProject("", "A", "", atelier, nil); err != nil {
			t.Fatal(err)
		}
		if err := s.SaveProject("", "B", "", atelier, nil); err != nil {
			t.Fatal(err)
		}
		// Renommer « B » en « A » entre en collision avec le projet « A » existant.
		if err := s.SaveProject("B", "A", "", atelier, nil); err == nil || !strings.Contains(err.Error(), "existe déjà") {
			t.Fatalf("collision attendue (« existe déjà »), obtenu %v", err)
		}
		if ps := s.Projects(); len(ps) != 2 {
			t.Fatalf("une collision ne doit rien changer : attendu 2 projets, obtenu %d", len(ps))
		}
	})

	t.Run("nettoyage des promotions orphelines", func(t *testing.T) {
		s := newInTemp(t)
		p1 := config.RepoRef{Instance: "forge", Owner: "acme", Repo: "p1"}
		p2 := config.RepoRef{Instance: "forge", Owner: "acme", Repo: "p2"}
		if err := s.SaveProject("", "Proj", "", atelier, []config.RepoRef{p1, p2}); err != nil {
			t.Fatal(err)
		}
		// Poser à la main un marqueur de promotion sur chaque vitrine.
		cfg, _ := config.Load()
		pr := cfg.FindProject("Proj")
		pr.Promotions = map[string]config.Promo{
			p1.Key(): {Version: "v1"},
			p2.Key(): {Version: "v2"},
		}
		if err := cfg.Save(); err != nil {
			t.Fatal(err)
		}
		// Ré-enregistrer sans p2 : son marqueur doit être nettoyé, celui de p1 préservé.
		if err := s.SaveProject("Proj", "Proj", "", atelier, []config.RepoRef{p1}); err != nil {
			t.Fatal(err)
		}
		cfg2, _ := config.Load()
		pr2 := cfg2.FindProject("Proj")
		if pr2 == nil {
			t.Fatal("projet introuvable après ré-enregistrement")
		}
		if _, ok := pr2.Promotions[p2.Key()]; ok {
			t.Error("le marqueur de la vitrine retirée (p2) aurait dû être nettoyé")
		}
		if _, ok := pr2.Promotions[p1.Key()]; !ok {
			t.Error("le marqueur de la vitrine conservée (p1) aurait dû être préservé")
		}
	})
}

// TestProjectPromoteGuards : les gardes de ProjectPromote qui s'appliquent AVANT tout accès réseau.
// Point clef : promouvoir vers une production exige TOUJOURS le PIN (fail-closed), même quand la
// connexion cible est de rôle atelier — un rôle mal classé ne doit pas ouvrir une publication.
func TestProjectPromoteGuards(t *testing.T) {
	if !gitx.Available() {
		t.Skip("git indisponible")
	}
	s := newInTemp(t)
	if err := s.SetPIN("", "123456"); err != nil {
		t.Fatalf("SetPIN : %v", err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	// La cible « prodatl » est délibérément de rôle atelier : la garde ne doit pas s'y fier.
	cfg.Instances = []config.Instance{
		{Name: "atl", URL: "https://atelier.example", Role: config.RoleAtelier},
		{Name: "prodatl", URL: "https://prod.example", Role: config.RoleAtelier},
	}
	cfg.Projects = []config.Project{{
		Name:        "P",
		Atelier:     config.RepoRef{Instance: "atl", Owner: "o", Repo: "r"},
		Productions: []config.RepoRef{{Instance: "prodatl", Owner: "o", Repo: "r"}},
	}}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	if err := s.Unlock("123456"); err != nil {
		t.Fatalf("Unlock : %v", err)
	}

	// Projet inconnu → refus (introuvable), avant toute opération.
	if _, err := s.ProjectPromote("inconnu", 0, PromoteOptions{}, "123456"); err == nil {
		t.Error("projet inconnu : une erreur était attendue")
	}
	// Production hors bornes → refus (inexistante).
	if _, err := s.ProjectPromote("P", 99, PromoteOptions{}, "123456"); err == nil {
		t.Error("production inexistante : une erreur était attendue")
	}
	// PIN vide → ErrPINNeeded malgré une cible de rôle atelier (promotion toujours sous PIN).
	if _, err := s.ProjectPromote("P", 0, PromoteOptions{}, ""); !errors.Is(err, ErrPINNeeded) {
		t.Errorf("PIN vide : attendu ErrPINNeeded, obtenu %v", err)
	}
	// Lecture seule → ErrReadOnly, avant toute vérification de PIN.
	if err := s.SetReadOnly(true); err != nil {
		t.Fatalf("SetReadOnly : %v", err)
	}
	if _, err := s.ProjectPromote("P", 0, PromoteOptions{}, "123456"); !errors.Is(err, ErrReadOnly) {
		t.Errorf("lecture seule : attendu ErrReadOnly, obtenu %v", err)
	}
}

// TestBranchTags : les versions joignables depuis une branche d'un clone local. Sans origin, on
// retombe sur la branche locale. Une branche vide est refusée sans toucher au dépôt.
func TestBranchTags(t *testing.T) {
	if !gitx.Available() {
		t.Skip("git indisponible")
	}
	s := newInTemp(t)

	// Branche vide : refus net.
	if _, err := s.BranchTags(t.TempDir(), ""); err == nil || !strings.Contains(err.Error(), "branche manquante") {
		t.Fatalf("branche vide : attendu « branche manquante », obtenu %v", err)
	}

	// Dépôt local avec un tag v1.0 sur main ; pas d'origin → repli sur la branche locale.
	repo := gitRepoNoRemote(t)
	run := func(args ...string) {
		c := exec.Command("git", args...)
		c.Dir = repo
		c.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %s : %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	run("branch", "-M", "main")
	run("tag", "v1.0")

	tags, err := s.BranchTags(repo, "main")
	if err != nil {
		t.Fatalf("BranchTags : %v", err)
	}
	if len(tags) != 1 || tags[0] != "v1.0" {
		t.Fatalf("tags de main = %v, attendu [v1.0]", tags)
	}
}
