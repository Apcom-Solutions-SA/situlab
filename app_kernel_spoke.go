// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package main

import (
	"situlab/internal/config"
	"situlab/internal/core"
)

func (a *App) Kernels() ([]core.KernelInfo, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.Kernels(), nil
}

func (a *App) KernelStatus(name string) (*core.KernelStatus, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.KernelStatus(name)
}

func (a *App) KernelContents(name string) ([]core.KernelEntry, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.KernelContents(name)
}

// ScanGitRepos — FENÊTRE : propose les dépôts git déjà présents sur la machine (connus + exploration)
// pour faciliter la désignation d'un KERNEL ou d'un spoke sans taper de chemin. Lecture pure.
func (a *App) ScanGitRepos() ([]core.FoundRepo, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.ScanGitRepos()
}

// ScanFolderRepos — FENÊTRE : découvre les dépôts git sous UN dossier choisi (mise en route :
// « scanner un dossier » puis suivre en lot). Lecture pure ; le suivi reste explicite (AddLocalRepo).
func (a *App) ScanFolderRepos(root string) ([]core.FoundRepo, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.ScanFolderRepos(root)
}

// RepoFiles liste les fichiers d'un dépôt local (chemins relatifs) pour choisir un « fichier
// d'ouverture » dans un arbre filtrable.
func (a *App) RepoFiles(localPath string) ([]string, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.RepoFiles(localPath)
}

// SetKernelRunFile définit le fichier d'ouverture (IA locale) d'un KERNEL, par son chemin local.
func (a *App) SetKernelRunFile(localPath, runFile string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.SetKernelRunFile(localPath, runFile)
}

// SetKernelSpokesFile définit le fichier de gestion des spokes (procédure de consolidation) d'un KERNEL.
func (a *App) SetKernelSpokesFile(localPath, spokesFile string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.SetKernelSpokesFile(localPath, spokesFile)
}

// SetSpokeRunFile définit le fichier d'ouverture (run) d'un spoke, par son chemin local.
func (a *App) SetSpokeRunFile(localPath, runFile string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.SetSpokeRunFile(localPath, runFile)
}

// AddKernel — FENÊTRE UNIQUEMENT : désigner un dépôt local comme KERNEL (mémoire).
func (a *App) AddKernel(name, localPath, instance, backup string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.AddKernel(name, localPath, instance, backup)
}

// RemoveKernel — FENÊTRE UNIQUEMENT.
func (a *App) RemoveKernel(name string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.RemoveKernel(name)
}

// SetActiveKernel — FENÊTRE UNIQUEMENT : choisir le KERNEL courant.
func (a *App) SetActiveKernel(name string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.SetActiveKernel(name)
}

// KernelPull récupère les dernières notes du KERNEL (sans PIN).
func (a *App) KernelPull(name string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.svc.KernelPull(name)
}

// KernelSave enregistre vos notes (récupérer → enregistrer → envoyer, un seul PIN).
func (a *App) KernelSave(name, message, pin string) (*core.SyncResult, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	res, err := a.svc.KernelSave(name, message, pin)
	if err == nil {
		a.afterWrite()
	}
	return res, err
}

// --- SPOKES (v1.11) : projections de mission (sous-menu KERNEL) ---

func (a *App) Spokes() ([]config.Spoke, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.Spokes(), nil
}

func (a *App) SpokeStatus(name string) (*core.KernelStatus, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.SpokeStatus(name)
}

func (a *App) SpokeContents(name string) ([]core.KernelEntry, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.SpokeContents(name)
}

// AddSpoke — FENÊTRE UNIQUEMENT : désigner un dépôt comme spoke (mission dérivée d'un KERNEL).
func (a *App) AddSpoke(name, localPath, kernel, mission, instance string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.AddSpoke(name, localPath, kernel, mission, instance)
}

// RemoveSpoke — FENÊTRE UNIQUEMENT.
func (a *App) RemoveSpoke(name string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.RemoveSpoke(name)
}

func (a *App) SpokePull(name string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.svc.SpokePull(name)
}

func (a *App) SpokeSave(name, message, pin string) (*core.SyncResult, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	res, err := a.svc.SpokeSave(name, message, pin)
	if err == nil {
		a.afterWrite()
	}
	return res, err
}
