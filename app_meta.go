// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package main

import (
	"errors"
	"os"
	"path/filepath"
	"situlab/internal/config"
	"situlab/internal/core"
	"strings"
)

// AppVersion renvoie la version.
func (a *App) AppVersion() string { return version }

// Doc renvoie la fiche AGENTS.md (Markdown) — utile aux humains comme aux agents. On lit d'abord la
// copie « administrée à la source » (dossier de config, survit aux mises à jour) si elle existe ;
// sinon on retombe sur le fichier embarqué. L'utilisateur voit ainsi toujours la fiche de référence.
func (a *App) Doc() string {
	if b, err := os.ReadFile(config.AgentsDocPath()); err == nil && len(b) > 0 {
		return string(b)
	}
	return agentsDoc
}

// AgentsDocPath renvoie le chemin de la fiche AGENTS.md administrée à la source (toujours le même,
// qu'elle existe déjà ou non) — pour l'afficher et pour le communiquer à l'agent IA.
func (a *App) AgentsDocPath() string { return config.AgentsDocPath() }

// AgentsDocExists indique si la fiche de référence a déjà été enregistrée à la source.
func (a *App) AgentsDocExists() bool {
	fi, err := os.Stat(config.AgentsDocPath())
	return err == nil && !fi.IsDir()
}

// SaveAgentsDocToSource écrit (ou réécrit) la fiche de référence AGENTS.md dans le dossier de
// configuration de SituLab. Contrairement à un dépôt dans un projet, cette copie est UNIQUE, stable
// et survit aux mises à jour : c'est elle que l'agent lira au chemin indiqué (v1.17). Renvoie le chemin.
func (a *App) SaveAgentsDocToSource() (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	p := config.AgentsDocPath()
	content := agentsDoc + "\n\n## Notre collaboration (maintenue avec mon IA)\n\n" +
		"_Cette section est la nôtre : notes de travail entre moi et mon assistant. Mon IA peut la compléter ; je peux l'éditer dans SituLab (Agent IA)._\n\n" +
		strings.TrimSpace(a.svc.AgentNotes()) + "\n"
	if err := os.MkdirAll(config.Dir(), 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		return "", err
	}
	return p, nil
}

// DeleteAgentsDocFromSource supprime la fiche AGENTS.md enregistrée à la source, pour permettre de
// refaire la mise en route depuis zéro (1.18.1). Idempotent : pas d'erreur si elle n'existe pas.
func (a *App) DeleteAgentsDocFromSource() error {
	if err := a.ready(); err != nil {
		return err
	}
	if err := os.Remove(config.AgentsDocPath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Modules renvoie l'état des groupes de modules d'interface (v1.13).
func (a *App) Modules() map[string]bool { return a.svc.Modules() }

// SetModule active/désactive un groupe de modules.
func (a *App) SetModule(key string, on bool) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.SetModule(key, on)
}

// SetModules applique un lot d'états de modules (préréglage).
func (a *App) SetModules(mods map[string]bool) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.SetModules(mods)
}

// License renvoie le texte complet de la licence (Apache 2.0).
func (a *App) License() string { return licenseText }

// Notice renvoie l'avis de droits et marques (fichier NOTICE).
func (a *App) Notice() string { return noticeText }

// Purge efface toutes les données SituLab de ce poste (v1.4.1) : connexions, trousseau chiffré,
// réglages, jetons du coffre OS. IRRÉVERSIBLE — la fenêtre demande une confirmation explicite
// avant d'appeler cette méthode, puis se réinitialise (onboarding).
//
// On FERME d'abord le serveur IPC : son socket AF_UNIX vit DANS config.Dir(), or sous Windows un
// socket encore lié n'est pas supprimable → sans ça, os.RemoveAll échouerait sur daemon.sock APRÈS
// avoir déjà effacé config.json/keyring.json, et remonterait une fausse erreur. (Le socket sera
// recréé au prochain lancement ; la fenêtre, elle, continue via les liaisons in-process.)
func (a *App) Purge() error {
	if err := a.ready(); err != nil {
		return err
	}
	if a.daemonSrv != nil {
		a.daemonSrv.Close()
		a.daemonSrv = nil
	}
	return a.svc.Purge()
}

// Agents liste les agents IA détectés sur ce poste, avec leur poids (toujours affiché) et la date de
// dernière purge (retour #14). Lecture seule.
func (a *App) Agents() []core.AgentInfo {
	if err := a.ready(); err != nil {
		return nil
	}
	return a.svc.Agents()
}

// AgentPurgePreview liste les entrées de premier niveau du dossier d'un agent (gardées / effacées, avec
// poids), pour montrer l'arbre dans la confirmation de purge (retour #14). Lecture seule.
func (a *App) AgentPurgePreview(key string) ([]core.AgentEntry, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.AgentPurgePreview(key)
}

// PurgeAgent efface les données de l'agent IA `key` sauf son login et son minimum de fonctionnement
// (remise à blanc franche, retour #14). `confirm` doit valoir "EFFACER". Renvoie l'état après coup.
func (a *App) PurgeAgent(key, confirm string) (core.AgentInfo, error) {
	if err := a.ready(); err != nil {
		return core.AgentInfo{}, err
	}
	return a.svc.PurgeAgent(key, confirm)
}

// ExePath renvoie le chemin de l'exécutable SituLab (pour composer la config MCP de l'agent).
func (a *App) ExePath() (string, error) { return os.Executable() }

// InstallAgentsDoc écrit le guide des agents (AGENTS.md embarqué) dans le dossier choisi, pour
// que l'assistant IA de l'utilisateur le lise (convention AGENTS.md). Ne RÉÉCRASE jamais un
// AGENTS.md existant : dans ce cas il écrit « SituLab-AGENTS.md » à côté. Renvoie le chemin écrit.
func (a *App) InstallAgentsDoc(dir string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	if dir == "" {
		return "", errors.New("aucun dossier choisi")
	}
	p := filepath.Join(dir, "AGENTS.md")
	if _, err := os.Stat(p); err == nil {
		p = filepath.Join(dir, "SituLab-AGENTS.md") // ne pas écraser le AGENTS.md de l'utilisateur
	}
	// Le guide embarqué + la fiche de collaboration (consignes du user) : l'IA lit l'ensemble au
	// démarrage et peut enrichir la section « Notre collaboration » directement dans ce fichier.
	content := agentsDoc + "\n\n## Notre collaboration (maintenue avec mon IA)\n\n" +
		"_Cette section est la nôtre : notes de travail entre moi et mon assistant. Mon IA peut la compléter ; je peux l'éditer dans SituLab (le menu Agent IA)._\n\n" +
		strings.TrimSpace(a.svc.AgentNotes()) + "\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		return "", err
	}
	return p, nil
}

// AgentNotes / SetAgentNotes : fiche de collaboration avec l'IA, éditable dans les Réglages (v1.5.1).
func (a *App) AgentNotes() (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.svc.AgentNotes(), nil
}

func (a *App) SetAgentNotes(notes string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.SetAgentNotes(notes)
}
