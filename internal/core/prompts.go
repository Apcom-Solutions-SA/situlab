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
		// Ne décrète jamais le PIN : SituLab arbitre selon la politique de la connexion et confirme
		// (fait / refusé / délai). Les prompts restent courts et directifs.
		{ID: "b-map", Title: "Faire le point sur mon poste", Body: "Fais le point avec situlab map --output json : dis-moi mes KERNEL, mes spokes, les tags projet, et l'état local↔atelier↔vitrine de chaque dépôt. Résume sans jargon."},
		{ID: "b-kernel-open", Title: "Ouvrir mon KERNEL", Body: "Fais l'ouverture du KERNEL <nom> : situlab kernel open <nom> te donne le fichier à lire (chemin absolu). Récupère d'abord les dernières notes si besoin (situlab kernel pull), lis ce fichier, puis suis ce qu'il indique."},
		{ID: "b-link-local", Title: "Relier mes dépôts locaux", Body: "Parcours mes dossiers de code sur ce poste, identifie ceux qui sont des dépôts git, et relie-les dans SituLab (situlab local add <dossier>). Liste ce que tu as trouvé avant d'agir."},
		{ID: "b-sync", Title: "Synchroniser un projet", Body: "Synchronise le dépôt <dossier> : récupère les nouveautés, enregistre mes modifications puis envoie le tout (situlab repo sync <dossier>). SituLab me demandera de valider si besoin. Résume ce qui a changé."},
		{ID: "b-issue", Title: "Créer une issue", Body: "Crée une issue sur <owner>/<repo> à partir de cette description : « … ». Propose un titre clair et un corps structuré (contexte, attendu, étapes), puis crée-la."},
		{ID: "b-promote", Title: "Promouvoir une version en vitrine", Body: "Prépare la promotion de la version <vX.Y.Z> du projet <nom> vers la vitrine (situlab project promote). Rappel : pousser un tag ne crée pas seul une Release GitHub (c'est la CI). Ne publie jamais en prod sans mon accord."},
		{ID: "b-review", Title: "Relire une demande de fusion", Body: "Montre-moi le diff de la demande de fusion #<n> sur <owner>/<repo>, résume les changements et les risques, et propose une revue (approuver / demander des changements) que je validerai."},
		{ID: "b-cloud-summary", Title: "Déposer un résumé dans le CORPUS", Body: "Dépose une note dans mon CORPUS : lis d'abord tes dossiers autorisés (situlab cloud allowed <cloud>), puis écris un .md de synthèse dans un dossier autorisé (situlab cloud write <fichier> --content \"…\"). Reste dans le périmètre autorisé."},
		{ID: "b-backup-run", Title: "Lancer une sauvegarde", Body: "Liste mes plans de sauvegarde (situlab backup list), puis lance le plan <nom> (situlab backup run <nom>) et confirme-moi ce qui a été sauvegardé."},
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
