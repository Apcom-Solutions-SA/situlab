// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package config

import "strings"

// RepoRef désigne un dépôt précis sur une connexion (v1.7) : instance + owner/repo.
type RepoRef struct {
	Instance string `json:"instance"`
	Owner    string `json:"owner"`
	Repo     string `json:"repo"`
}

// Key renvoie une clé canonique « instance/owner/repo » (casse d'origine conservée).
func (r RepoRef) Key() string { return r.Instance + "/" + r.Owner + "/" + r.Repo }

// Promo mémorise le dernier état promu vers une production : la version (tag) publiée et la date.
type Promo struct {
	Version string `json:"version,omitempty"` // tag/version promu en dernier
	Date    string `json:"date,omitempty"`    // date de la promotion (RFC3339)
}

// Project relie UN dépôt atelier (source de vérité du dev) à 1..N dépôts production (vitrines).
// La prod n'est jamais poussée depuis le local : on y publie par PROMOTION (v1.7.2). Promotions
// mémorise, par production (clé RepoRef.Key()), le dernier état promu — pour lire le point d'équilibre.
type Project struct {
	Name string `json:"name"`
	// Icon : pastille stylisée « color:<clé>|<initiales> » ou « upload:<dataURL> » (v1.18.2),
	// ou ancien emoji/picto (rétro-compatible, rendu tel quel).
	Icon        string           `json:"icon,omitempty"`
	Tag         string           `json:"tag,omitempty"`  // LEGACY mono-tag (lu en repli ; voir Tags)
	Tags        []string         `json:"tags,omitempty"` // tags multiples (v1.18.2)
	Atelier     RepoRef          `json:"atelier"`
	Productions []RepoRef        `json:"productions"` // vitrines (production) ; peut être vide
	Promotions  map[string]Promo `json:"promotions,omitempty"`
	// Corpus : fichiers/dossiers de clouds CORPUS reliés à ce projet (mémoire situative, v1.18.2).
	// Simple POINTEUR projet→(cloud, chemins) : ne décide d'AUCUN droit (l'accès IA reste décidé par
	// FileCloud.AllowedDirs/DeniedDirs). HORS SecurityDigest.
	Corpus []CorpusLink `json:"corpus,omitempty"`
}

// CorpusLink relie un projet à des chemins d'un cloud CORPUS (v1.18.2).
type CorpusLink struct {
	Cloud string   `json:"cloud"`
	Paths []string `json:"paths,omitempty"`
}

// TagList renvoie les tags du projet (Tags, sinon le mono-tag legacy, sinon vide).
func (p Project) TagList() []string {
	if len(p.Tags) > 0 {
		return p.Tags
	}
	if strings.TrimSpace(p.Tag) != "" {
		return []string{p.Tag}
	}
	return nil
}

// TagDef : définition d'un tag projet (nom canonique + clé de couleur d'une palette, v1.18.2).
type TagDef struct {
	Name  string `json:"name"`
	Color string `json:"color,omitempty"` // clé de palette (ex. "blue", "amber"…) ; vide = couleur par défaut
}

// UpsertTagDef crée ou met à jour la définition d'un tag (couleur). Insensible à la casse sur le nom.
func (c *Config) UpsertTagDef(name, color string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	color = strings.TrimSpace(color)
	for i := range c.TagDefs {
		if strings.EqualFold(c.TagDefs[i].Name, name) {
			if color != "" {
				c.TagDefs[i].Color = color
			}
			return
		}
	}
	c.TagDefs = append(c.TagDefs, TagDef{Name: name, Color: color})
}

// TagColorOf renvoie la clé de couleur d'un tag (vide si inconnu ou sans couleur).
func (c *Config) TagColorOf(name string) string {
	for _, t := range c.TagDefs {
		if strings.EqualFold(t.Name, name) {
			return t.Color
		}
	}
	return ""
}

// RenameTag renomme un tag partout (registre + projets), en fusionnant sur collision.
func (c *Config) RenameTag(oldName, newName string) {
	oldName = strings.TrimSpace(oldName)
	newName = strings.TrimSpace(newName)
	if oldName == "" || newName == "" || strings.EqualFold(oldName, newName) {
		return
	}
	// Couleur : si la cible n'existe pas, elle hérite de la couleur de l'ancien ; puis on retire l'ancien.
	oldColor := c.TagColorOf(oldName)
	if c.TagColorOf(newName) == "" && oldColor != "" {
		c.UpsertTagDef(newName, oldColor)
	} else {
		c.UpsertTagDef(newName, "")
	}
	defs := c.TagDefs[:0]
	for _, t := range c.TagDefs {
		if strings.EqualFold(t.Name, oldName) {
			continue
		}
		defs = append(defs, t)
	}
	c.TagDefs = defs
	for i := range c.Projects {
		tags := c.Projects[i].TagList()
		out := make([]string, 0, len(tags))
		seen := map[string]bool{}
		for _, t := range tags {
			if strings.EqualFold(t, oldName) {
				t = newName
			}
			if seen[strings.ToLower(t)] {
				continue
			}
			seen[strings.ToLower(t)] = true
			out = append(out, t)
		}
		if len(c.Projects[i].Tags) > 0 || !strings.EqualFold(c.Projects[i].Tag, "") {
			c.Projects[i].Tags = out
			c.Projects[i].Tag = ""
		}
	}
}

// DeleteTag retire un tag du registre et de tous les projets.
func (c *Config) DeleteTag(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	defs := c.TagDefs[:0]
	for _, t := range c.TagDefs {
		if strings.EqualFold(t.Name, name) {
			continue
		}
		defs = append(defs, t)
	}
	c.TagDefs = defs
	for i := range c.Projects {
		tags := c.Projects[i].TagList()
		if len(tags) == 0 {
			continue
		}
		out := make([]string, 0, len(tags))
		for _, t := range tags {
			if strings.EqualFold(t, name) {
				continue
			}
			out = append(out, t)
		}
		c.Projects[i].Tags = out
		c.Projects[i].Tag = ""
	}
}

// ArchivePairing range une clé d'appariement (idempotent). Renvoie true si elle a été ajoutée.
func (c *Config) ArchivePairing(key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	for _, k := range c.ArchivedPairings {
		if k == key {
			return false
		}
	}
	c.ArchivedPairings = append(c.ArchivedPairings, key)
	return true
}

// UnarchivePairing retire une clé de l'archive. Renvoie true si elle y était.
func (c *Config) UnarchivePairing(key string) bool {
	out := c.ArchivedPairings[:0]
	found := false
	for _, k := range c.ArchivedPairings {
		if k == key {
			found = true
			continue
		}
		out = append(out, k)
	}
	c.ArchivedPairings = out
	return found
}

// VitrineHint : suggestion manuelle « cette vitrine va avec cet atelier » (noms différents). Elle
// rattache la vitrine à la ligne de l'atelier dans la matrice SANS créer de projet — l'utilisateur
// confirme ensuite par « Appairer ». (1.18.1)
type VitrineHint struct {
	Atelier RepoRef `json:"atelier"`
	Vitrine RepoRef `json:"vitrine"`
}

func sameRepoRef(a, b RepoRef) bool {
	return strings.EqualFold(a.Instance, b.Instance) && strings.EqualFold(a.Owner, b.Owner) && strings.EqualFold(a.Repo, b.Repo)
}

// AddVitrineHint enregistre une suggestion atelier→vitrine (idempotent). true si ajoutée.
func (c *Config) AddVitrineHint(atelier, vitrine RepoRef) bool {
	if atelier.Repo == "" || vitrine.Repo == "" {
		return false
	}
	for _, h := range c.VitrineHints {
		if sameRepoRef(h.Atelier, atelier) && sameRepoRef(h.Vitrine, vitrine) {
			return false
		}
	}
	c.VitrineHints = append(c.VitrineHints, VitrineHint{Atelier: atelier, Vitrine: vitrine})
	return true
}

// ClearVitrineHintsForAtelier retire toutes les suggestions portant sur cet atelier (appelé quand la
// paire est confirmée en projet, pour ne pas laisser de suggestion fantôme).
func (c *Config) ClearVitrineHintsForAtelier(atelier RepoRef) {
	out := c.VitrineHints[:0]
	for _, h := range c.VitrineHints {
		if sameRepoRef(h.Atelier, atelier) {
			continue
		}
		out = append(out, h)
	}
	c.VitrineHints = out
}

// IsPairingArchived indique si une clé d'appariement est archivée.
func (c *Config) IsPairingArchived(key string) bool {
	for _, k := range c.ArchivedPairings {
		if k == key {
			return true
		}
	}
	return false
}

// PruneDanglingPromotions retire les marqueurs de promotion dont la vitrine n'existe plus dans le
// projet (après un changement de vitrines), pour ne pas garder d'état orphelin.
func (p *Project) PruneDanglingPromotions() {
	if len(p.Promotions) == 0 {
		return
	}
	keep := map[string]bool{}
	for _, pr := range p.Productions {
		keep[pr.Key()] = true
	}
	for k := range p.Promotions {
		if !keep[k] {
			delete(p.Promotions, k)
		}
	}
}

// FindProject renvoie le projet par nom (insensible à la casse), ou nil.
func (c *Config) FindProject(name string) *Project {
	for i := range c.Projects {
		if eqName(c.Projects[i].Name, name) {
			return &c.Projects[i]
		}
	}
	return nil
}
