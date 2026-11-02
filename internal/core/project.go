// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"fmt"
	"strings"

	"situlab/internal/config"
)

// RepoLine décrit un dépôt d'un projet et sa version courante (dernier tag).
type RepoLine struct {
	Instance string `json:"instance"`
	Owner    string `json:"owner"`
	Repo     string `json:"repo"`
	Version  string `json:"version"` // dernier tag, ou "" si aucun
	Error    string `json:"error"`   // lecture impossible (connexion KO, droits…)
}

// ProdLine ajoute l'état d'équilibre d'une production vis-à-vis de l'atelier.
type ProdLine struct {
	RepoLine
	Balanced bool   `json:"balanced"` // même version que l'atelier
	Behind   int    `json:"behind"`   // nb de versions d'atelier plus récentes que la prod
	Promoted string `json:"promoted"` // dernière version promue par SituLab (marqueur), si connue
}

// ProjectView est l'état consolidé d'un projet pour l'affichage/agent.
type ProjectView struct {
	Name    string     `json:"name"`
	Atelier RepoLine   `json:"atelier"`
	Prods   []ProdLine `json:"productions"`
}

// Projects renvoie les projets configurés (liens atelier↔prod). Lecture, pas de PIN.
func (s *Service) Projects() []config.Project {
	s.mu.Lock()
	s.lockedReload()
	ps := append([]config.Project(nil), s.cfg.Projects...)
	s.mu.Unlock()
	return ps
}

// AddProject crée un projet (atelier obligatoire ; vitrines facultatives). Compat ancienne signature.
func (s *Service) AddProject(name string, atelier config.RepoRef, productions []config.RepoRef) error {
	return s.SaveProject("", name, "", atelier, productions)
}

// SaveProject crée OU modifie un projet. oldName vide = création ; sinon on retrouve le projet par
// son ancien nom et on peut le renommer, changer son icône, son atelier et ses vitrines. Un atelier
// reste obligatoire ; les vitrines (production) sont facultatives. Les marqueurs de promotion des
// vitrines conservées sont préservés ; ceux des vitrines retirées sont nettoyés.
func (s *Service) SaveProject(oldName, name, icon string, atelier config.RepoRef, productions []config.RepoRef) error {
	if err := s.assertWritable(); err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	oldName = strings.TrimSpace(oldName)
	if name == "" {
		return fmt.Errorf("nom de projet vide")
	}
	if atelier.Instance == "" || atelier.Owner == "" || atelier.Repo == "" {
		return fmt.Errorf("dépôt atelier incomplet (instance/owner/repo)")
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	key := oldName
	if key == "" {
		key = name
	}
	// Collision : le nom cible ne doit pas appartenir à un AUTRE projet que celui édité.
	for i := range cfg.Projects {
		if eqProjName(cfg.Projects[i].Name, name) && !eqProjName(cfg.Projects[i].Name, key) {
			return fmt.Errorf("un projet « %s » existe déjà", name)
		}
	}
	existing := cfg.FindProject(key)
	if existing != nil {
		existing.Name = name
		existing.Icon = strings.TrimSpace(icon)
		existing.Atelier = atelier
		existing.Productions = productions
		existing.PruneDanglingPromotions()
	} else {
		cfg.Projects = append(cfg.Projects, config.Project{Name: name, Icon: strings.TrimSpace(icon), Atelier: atelier, Productions: productions})
	}
	cfg.ClearVitrineHintsForAtelier(atelier) // la paire est confirmée : plus de suggestion fantôme
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}

func eqProjName(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

// ConfirmVitrine confirme une vitrine SUGGÉRÉE (match par nom ou VitrineHint) dans le projet de son
// atelier (#16, 1.19). Si un projet existe déjà pour cet atelier, on y AJOUTE la vitrine sans toucher
// au reste (nom, icône, tags, autres vitrines) ; sinon on crée le projet. Dans les deux cas la
// suggestion fantôme (hint) est effacée. Geste FENÊTRE, local, sans PIN.
func (s *Service) ConfirmVitrine(atelier, vitrine config.RepoRef) error {
	if err := s.assertWritable(); err != nil {
		return err
	}
	if atelier.Instance == "" || atelier.Owner == "" || atelier.Repo == "" {
		return fmt.Errorf("atelier incomplet (instance/owner/repo)")
	}
	if vitrine.Instance == "" || vitrine.Owner == "" || vitrine.Repo == "" {
		return fmt.Errorf("vitrine incomplète (instance/owner/repo)")
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ak, vk := refKey(atelier), refKey(vitrine)
	var proj *config.Project
	for i := range cfg.Projects {
		if refKey(cfg.Projects[i].Atelier) == ak {
			proj = &cfg.Projects[i]
			break
		}
	}
	if proj != nil {
		already := false
		for _, pr := range proj.Productions {
			if refKey(pr) == vk {
				already = true
				break
			}
		}
		if !already {
			proj.Productions = append(proj.Productions, vitrine)
		}
	} else {
		cfg.Projects = append(cfg.Projects, config.Project{Name: atelier.Repo, Atelier: atelier, Productions: []config.RepoRef{vitrine}})
	}
	cfg.ClearVitrineHintsForAtelier(atelier) // la paire est confirmée : plus de suggestion fantôme
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}

// SetProjectCorpus définit les fichiers/dossiers CORPUS reliés à un projet (mapping, v1.18.2). Geste
// de désignation : FENÊTRE uniquement (assertWritable, sans PIN, jamais exposé au démon/CLI/MCP). Ne
// crée AUCUN droit (le mapping est un pointeur ; l'accès IA reste décidé par les droits du cloud).
func (s *Service) SetProjectCorpus(name string, links []config.CorpusLink) error {
	if err := s.assertWritable(); err != nil {
		return err
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	clean := make([]config.CorpusLink, 0, len(links))
	for _, l := range links {
		cloud := strings.TrimSpace(l.Cloud)
		if cloud == "" || cfg.ResolveCloud(cloud) == nil {
			continue // ignore les clouds inexistants (best-effort)
		}
		seen := map[string]bool{}
		paths := make([]string, 0, len(l.Paths))
		for _, p := range l.Paths {
			cp := config.CloudCleanPath(p)
			if seen[cp] {
				continue
			}
			seen[cp] = true
			paths = append(paths, cp)
		}
		if len(paths) == 0 {
			continue
		}
		clean = append(clean, config.CorpusLink{Cloud: cloud, Paths: paths})
	}
	found := false
	for i := range cfg.Projects {
		if eqProjName(cfg.Projects[i].Name, name) {
			cfg.Projects[i].Corpus = clean
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

// RemoveProject retire un projet par nom (ne touche à aucun dépôt).
func (s *Service) RemoveProject(name string) error {
	if err := s.assertWritable(); err != nil {
		return err
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	out := cfg.Projects[:0]
	for _, p := range cfg.Projects {
		if !strings.EqualFold(strings.TrimSpace(p.Name), strings.TrimSpace(name)) {
			out = append(out, p)
		}
	}
	cfg.Projects = out
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}

// Modes de promotion (v1.9.2) — l'utilisateur choisit AU CAS PAR CAS la méthode. Clarifie la confusion
// « promouvoir ≠ publier une Release » : pousser du code (± un tag) sur la vitrine ne crée PAS, à lui
// seul, une Release GitHub ni ses installeurs. La Release et les binaires sont produits par la CI,
// déclenchée par le PUSH DU TAG (`release.yml`). Trois méthodes :
const (
	// PromoteModeCode : met à jour la branche de la vitrine SANS tag — aucune nouvelle version publiée
	// (utile pour aligner du contenu sans déclencher de release).
	PromoteModeCode = "code"
	// PromoteModeTag (défaut) : code + tag de version → sur GitHub, le tag DÉCLENCHE la CI qui bâtit les
	// installeurs et publie la Release. C'est le cas normal d'une mise en vitrine versionnée.
	PromoteModeTag = "tag"
	// PromoteModeRelease : code + tag + création EXPLICITE de l'objet Release via l'API (avec notes de
	// version). Garantit que la Release existe même sans attendre la CI — mais les INSTALLEURS restent
	// produits par la CI (l'app ne compile pas de binaires). À utiliser quand on veut la page Release
	// tout de suite, les assets suivant via la CI.
	PromoteModeRelease = "release"
)
