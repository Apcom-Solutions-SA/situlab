// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"fmt"
	"situlab/internal/config"
	"strings"
)

// SetProjectTag classe un projet dans un « tag projet » (groupe libre). tag vide = non classé. Ne
// touche à aucun dépôt — c'est un rangement de l'affichage.
func (s *Service) SetProjectTag(name, tag string) error {
	if err := s.assertWritable(); err != nil {
		return err
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	found := false
	for i := range cfg.Projects {
		if eqProjName(cfg.Projects[i].Name, name) {
			cfg.Projects[i].Tag = strings.TrimSpace(tag)
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("projet introuvable : %q", name)
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}

// normTags : trim, retrait des vides, déduplication en préservant l'ordre.
func normTags(tags []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t == "" || seen[strings.ToLower(t)] {
			continue
		}
		seen[strings.ToLower(t)] = true
		out = append(out, t)
	}
	return out
}

// SetProjectTags fixe les tags (1..N) d'un projet. Rangement d'affichage, ne touche à aucun dépôt.
// Geste FENÊTRE (assertWritable, sans PIN, jamais exposé au démon/CLI/MCP).
func (s *Service) SetProjectTags(name string, tags []string) error {
	if err := s.assertWritable(); err != nil {
		return err
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	found := false
	for i := range cfg.Projects {
		if eqProjName(cfg.Projects[i].Name, name) {
			cfg.Projects[i].Tags = normTags(tags)
			cfg.Projects[i].Tag = "" // on bascule sur le multi-tag, l'ancien mono-tag ne vit plus
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("projet introuvable : %q", name)
	}
	// Enregistre d'office les tags inconnus dans le registre (couleur par défaut).
	for _, t := range normTags(tags) {
		cfg.UpsertTagDef(t, "")
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}

// CreateProjectTag enregistre un tag au registre (avec une couleur de palette), sans l'assigner.
func (s *Service) CreateProjectTag(name, color string) error {
	return s.tagRegistryEdit(func(cfg *config.Config) error {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("nom de tag vide")
		}
		cfg.UpsertTagDef(name, color)
		return nil
	})
}

// SetProjectTagColor change la couleur (clé de palette) d'un tag du registre.
func (s *Service) SetProjectTagColor(name, color string) error {
	return s.tagRegistryEdit(func(cfg *config.Config) error {
		cfg.UpsertTagDef(name, color)
		return nil
	})
}

// RenameProjectTag renomme un tag partout (registre + tous les projets), fusionnant en cas de collision.
func (s *Service) RenameProjectTag(oldName, newName string) error {
	return s.tagRegistryEdit(func(cfg *config.Config) error {
		if strings.TrimSpace(newName) == "" {
			return fmt.Errorf("nouveau nom de tag vide")
		}
		cfg.RenameTag(oldName, newName)
		return nil
	})
}

// DeleteProjectTag retire un tag du registre et de tous les projets.
func (s *Service) DeleteProjectTag(name string) error {
	return s.tagRegistryEdit(func(cfg *config.Config) error {
		cfg.DeleteTag(name)
		return nil
	})
}

// TagDefs renvoie le registre des tags (nom + couleur). Lecture.
func (s *Service) TagDefs() []config.TagDef {
	s.mu.Lock()
	cfg := s.cfg
	s.mu.Unlock()
	return append([]config.TagDef(nil), cfg.TagDefs...)
}

func (s *Service) tagRegistryEdit(fn func(*config.Config) error) error {
	if err := s.assertWritable(); err != nil {
		return err
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := fn(cfg); err != nil {
		return err
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}

// latestTags renvoie les tags d'un dépôt, du plus récent au plus ancien (ordre de l'API), et le
// plus récent. Best-effort : une erreur remonte via err.
func (s *Service) latestTags(ref config.RepoRef) (tags []string, latest string, err error) {
	ts, e := s.RepoTags(ref.Instance, ref.Owner, ref.Repo)
	if e != nil {
		return nil, "", e
	}
	for _, t := range ts {
		tags = append(tags, t.Name)
	}
	if len(tags) > 0 {
		latest = tags[0]
	}
	return tags, latest, nil
}

// latestTag ne lit QUE le tag le plus récent d'un dépôt, sans paginer tout l'historique des versions
// (forgejo.LatestTag). À réserver aux cas où seul le dernier tag compte — vitrines, barre de version —
// par opposition à latestTags qui ramène la liste complète (nécessaire à l'atelier pour l'index de
// retard). Best-effort : une erreur remonte via err (perf 1.17, Point 2).
func (s *Service) latestTag(ref config.RepoRef) (string, error) {
	cl, _, err := s.clientFor(ref.Instance)
	if err != nil {
		return "", err
	}
	ctx, cancel := opCtx()
	defer cancel()
	t, ok, err := cl.LatestTag(ctx, ref.Owner, ref.Repo)
	if err != nil || !ok {
		return "", err
	}
	return t.Name, nil
}

// tagIndex associe chaque tag d'atelier à son rang (0 = le plus récent), pour mesurer le retard d'une
// vitrine par rapport à l'atelier.
func tagIndex(atelierTags []string) map[string]int {
	idx := make(map[string]int, len(atelierTags))
	for i, t := range atelierTags {
		idx[t] = i
	}
	return idx
}

// tagColorsFor renvoie la map tag→clé de couleur (seulement les tags qui ont une couleur).
func tagColorsFor(tags []string, byLower map[string]string) map[string]string {
	out := map[string]string{}
	for _, t := range tags {
		if c := byLower[strings.ToLower(t)]; c != "" {
			out[t] = c
		}
	}
	return out
}
