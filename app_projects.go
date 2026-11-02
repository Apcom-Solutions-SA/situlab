// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package main

import (
	"situlab/internal/config"
	"situlab/internal/core"
)

func (a *App) Projects() ([]config.Project, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.Projects(), nil
}

func (a *App) ProjectStatus(name string) (*core.ProjectView, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.ProjectStatus(name)
}

// ProjectBoards : état « 3 colonnes » (local · atelier · production) de tous les projets (v1.8).
// ProjectsLight : état d'accueil léger (zéro réseau) de tous les projets — pour la page « classeurs ».
func (a *App) ProjectsLight() ([]core.ProjectLightView, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.ProjectsLight(), nil
}

// ProjectBoardOne : état 3 colonnes d'UN projet (réseau pour ce projet seul), au clic sur un classeur.
func (a *App) ProjectBoardOne(name string) (*core.ProjectBoard, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.ProjectBoardOne(name)
}

func (a *App) ProjectBoards() ([]core.ProjectBoard, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.ProjectBoards()
}

// RepoMatrix assemble la matrice des dépôts (ateliers, vitrines, clones locaux) et propose les
// appariements local↔atelier↔vitrine à transformer en projets. Action de service « Organiser mes
// dépôts » — lecture réseau, l'humain confirme chaque liaison.
func (a *App) RepoMatrix() (*core.RepoMatrix, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.RepoMatrix()
}

// ArchivePairing range une suggestion d'appariement (réversible, sans suppression).
func (a *App) ArchivePairing(key string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.ArchivePairing(key)
}

// UnarchivePairing repasse une suggestion archivée en « en attente » d'appairage.
func (a *App) UnarchivePairing(key string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.UnarchivePairing(key)
}

// PairingDelete supprime les dépôts d'une ligne (distants : définitif, sous PIN). wipeLocal=false :
// le clone local est retiré du suivi (fichiers conservés) ; wipeLocal=true : le dossier local est
// EFFACÉ du disque (irréversible) puis retiré du suivi.
func (a *App) PairingDelete(localPath string, wipeLocal bool, atelier config.RepoRef, productions []config.RepoRef, pin string) error {
	if err := a.ready(); err != nil {
		return err
	}
	err := a.svc.PairingDelete(localPath, wipeLocal, atelier, productions, pin)
	if err == nil {
		a.afterWrite()
	}
	return err
}

// SetProjectTag classe un projet dans un « tag projet » (groupe libre) ; tag vide = non classé.
func (a *App) SetProjectTag(name, tag string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.SetProjectTag(name, tag)
}

// --- Tags projet puissants (v1.18.2) — FENÊTRE uniquement (jamais daemon/CLI/MCP) ---

// SetProjectTags fixe les tags (1..N) d'un projet.
func (a *App) SetProjectTags(name string, tags []string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.SetProjectTags(name, tags)
}

// CreateProjectTag enregistre un tag au registre avec une couleur de palette.
func (a *App) CreateProjectTag(name, color string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.CreateProjectTag(name, color)
}

// RenameProjectTag renomme un tag partout (registre + projets).
func (a *App) RenameProjectTag(oldName, newName string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.RenameProjectTag(oldName, newName)
}

// DeleteProjectTag retire un tag du registre et de tous les projets.
func (a *App) DeleteProjectTag(name string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.DeleteProjectTag(name)
}

// SetProjectTagColor change la couleur (clé de palette) d'un tag.
func (a *App) SetProjectTagColor(name, color string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.SetProjectTagColor(name, color)
}

// TagDefs renvoie le registre des tags (nom + couleur).
func (a *App) TagDefs() []config.TagDef {
	if err := a.ready(); err != nil {
		return nil
	}
	return a.svc.TagDefs()
}

// SetProjectCorpus définit le mapping CORPUS↔dépôt d'un projet (v1.18.2) — FENÊTRE uniquement.
func (a *App) SetProjectCorpus(name string, links []config.CorpusLink) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.SetProjectCorpus(name, links)
}

// AddProject crée un projet (atelier obligatoire, vitrines facultatives). icon = emoji/picto choisi.
func (a *App) AddProject(name, icon string, atelier config.RepoRef, productions []config.RepoRef) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.SaveProject("", name, icon, atelier, productions)
}

// AddVitrineHint assemble une paire atelier→vitrine SANS la confirmer (noms différents). La vitrine
// se rattache à la ligne de l'atelier ; l'utilisateur confirme ensuite par « Appairer ». (1.18.1)
func (a *App) AddVitrineHint(atelier, vitrine config.RepoRef) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.AddVitrineHint(atelier, vitrine)
}

// UpdateProject modifie un projet existant (retrouvé par oldName) : renommage, icône, atelier et
// vitrines. Permet notamment de passer d'un projet « local + atelier » à « local + atelier + vitrine ».
func (a *App) UpdateProject(oldName, name, icon string, atelier config.RepoRef, productions []config.RepoRef) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.SaveProject(oldName, name, icon, atelier, productions)
}

func (a *App) RemoveProject(name string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.RemoveProject(name)
}

// ConfirmVitrine confirme une vitrine suggérée dans le projet de son atelier (crée le projet au besoin),
// et efface la suggestion (#16). Geste fenêtre, local, sans PIN.
func (a *App) ConfirmVitrine(atelier, vitrine config.RepoRef) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.ConfirmVitrine(atelier, vitrine)
}

// ProjectPromote publie la version de l'atelier vers une production du projet (PIN saisi dans la
// fenêtre). Geste de production : jamais déclenché par une IA sans confirmation humaine.
func (a *App) ProjectPromote(name string, prodIndex int, sourceBranch, version, targetBranch, mode, notes, pin string) (*core.PromoteResult, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	res, err := a.svc.ProjectPromote(name, prodIndex, core.PromoteOptions{SourceBranch: sourceBranch, Version: version, TargetBranch: targetBranch, Mode: mode, Notes: notes}, pin)
	if err == nil {
		a.afterWrite()
	}
	return res, err
}
