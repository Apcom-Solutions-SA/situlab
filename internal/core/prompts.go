// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"fmt"
	"strings"
	"time"

	"situlab/internal/config"
)

// Bibliothèque de prompts (v1.9.4) : une liste PAR DÉFAUT livrée avec le binaire (elle évolue aux mises
// à jour) + les personnalisations de l'utilisateur stockées en config (donc JAMAIS écrasées par une MàJ).
// L'utilisateur peut surcharger un prompt par défaut, le masquer, ou en ajouter de nouveaux.

// PromptView : un prompt tel que présenté à l'interface, avec son origine.
type PromptView struct {
	config.Prompt
	Builtin    bool `json:"builtin"`    // vient de la liste par défaut (livrée avec SituLab)
	Overridden bool `json:"overridden"` // prompt par défaut modifié par l'utilisateur
}

// defaultPrompts : les prompts suggérés livrés avec cette version. Ids stables (préfixe « b-») pour que
// les surcharges/masquages suivent le prompt à travers les mises à jour.
func defaultPrompts() []config.Prompt {
	return []config.Prompt{
		{ID: "b-link-local", Title: "Relier mes dépôts locaux", Body: "Parcours mes dossiers de code sur ce poste, identifie ceux qui sont des dépôts git reliés à mon serveur, et relie-les dans SituLab (situlab local add <dossier>). Liste ce que tu as trouvé avant d'agir."},
		{ID: "b-sync", Title: "Synchroniser un projet", Body: "Synchronise le dépôt <dossier> : récupère les nouveautés, enregistre mes modifications en cours puis envoie le tout (situlab repo sync <dossier>). Je validerai le PIN si nécessaire. Résume ce qui a changé."},
		{ID: "b-issue", Title: "Créer une issue", Body: "Crée une issue sur <owner>/<repo> à partir de cette description : « … ». Propose un titre clair et un corps structuré (contexte, attendu, étapes), puis crée-la. Je validerai le PIN."},
		{ID: "b-promote", Title: "Promouvoir une version en vitrine", Body: "Prépare la promotion de la version <vX.Y.Z> du projet <nom> vers la vitrine (situlab project promote). Rappelle-moi : pousser un tag ne crée PAS seul une Release GitHub (c'est la CI). Je validerai le PIN ; ne publie jamais en prod sans mon accord."},
		{ID: "b-review", Title: "Relire une demande de fusion", Body: "Montre-moi le diff de la demande de fusion #<n> sur <owner>/<repo>, résume les changements et les risques, et propose une revue (approuver / demander des changements) que je validerai."},
	}
}

// Prompts renvoie la vue fusionnée : prompts par défaut (hors masqués, surcharges appliquées) + prompts
// de l'utilisateur. Lecture, pas de PIN.
func (s *Service) Prompts() []PromptView {
	cfg, err := config.Load()
	if err != nil {
		cfg = &config.Config{}
	}
	hidden := map[string]bool{}
	for _, id := range cfg.HiddenPrompts {
		hidden[id] = true
	}
	out := make([]PromptView, 0, 12)
	for _, d := range defaultPrompts() {
		if hidden[d.ID] {
			continue
		}
		pv := PromptView{Prompt: d, Builtin: true}
		if ov, ok := cfg.PromptOverrides[d.ID]; ok {
			pv.Title, pv.Body, pv.Overridden = ov.Title, ov.Body, true
		}
		out = append(out, pv)
	}
	for _, u := range cfg.UserPrompts {
		out = append(out, PromptView{Prompt: u})
	}
	return out
}

// isBuiltinPrompt indique si un id correspond à un prompt livré par défaut.
func isBuiltinPrompt(id string) bool {
	for _, d := range defaultPrompts() {
		if d.ID == id {
			return true
		}
	}
	return false
}

// SavePrompt enregistre un prompt : surcharge d'un prompt par défaut (id « b-… »), modification d'un
// prompt utilisateur (id « u-… »), ou création (id vide → nouvel id « u-… »). Écriture (lecture seule refusée).
func (s *Service) SavePrompt(id, title, body string) error {
	if err := s.assertWritable(); err != nil {
		return err
	}
	title, body = strings.TrimSpace(title), strings.TrimSpace(body)
	if title == "" || body == "" {
		return fmt.Errorf("titre et corps requis")
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if isBuiltinPrompt(id) {
		if cfg.PromptOverrides == nil {
			cfg.PromptOverrides = map[string]config.Prompt{}
		}
		cfg.PromptOverrides[id] = config.Prompt{ID: id, Title: title, Body: body}
	} else {
		if strings.TrimSpace(id) == "" {
			id = fmt.Sprintf("u-%d", time.Now().UnixNano())
		}
		found := false
		for i := range cfg.UserPrompts {
			if cfg.UserPrompts[i].ID == id {
				cfg.UserPrompts[i].Title, cfg.UserPrompts[i].Body = title, body
				found = true
				break
			}
		}
		if !found {
			cfg.UserPrompts = append(cfg.UserPrompts, config.Prompt{ID: id, Title: title, Body: body})
		}
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}

// DeletePrompt masque un prompt par défaut (réversible par ResetPrompt) ou supprime un prompt utilisateur.
func (s *Service) DeletePrompt(id string) error {
	if err := s.assertWritable(); err != nil {
		return err
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if isBuiltinPrompt(id) {
		delete(cfg.PromptOverrides, id)
		has := false
		for _, h := range cfg.HiddenPrompts {
			if h == id {
				has = true
				break
			}
		}
		if !has {
			cfg.HiddenPrompts = append(cfg.HiddenPrompts, id)
		}
	} else {
		out := cfg.UserPrompts[:0]
		for _, u := range cfg.UserPrompts {
			if u.ID != id {
				out = append(out, u)
			}
		}
		cfg.UserPrompts = out
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}

// ResetPrompt rétablit un prompt par défaut : retire sa surcharge et son masquage éventuels.
func (s *Service) ResetPrompt(id string) error {
	if err := s.assertWritable(); err != nil {
		return err
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	delete(cfg.PromptOverrides, id)
	out := cfg.HiddenPrompts[:0]
	for _, h := range cfg.HiddenPrompts {
		if h != id {
			out = append(out, h)
		}
	}
	cfg.HiddenPrompts = out
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}
