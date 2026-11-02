// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package main

import "situlab/internal/config"

func (a *App) Backups() ([]config.BackupPlan, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.Backups(), nil
}

func (a *App) AddBackupPlan(name, repoPath, cloud, dest string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.AddBackupPlan(name, repoPath, cloud, dest)
}

func (a *App) RemoveBackupPlan(name string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.RemoveBackupPlan(name)
}

func (a *App) RunBackup(name string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	out, err := a.svc.RunBackup(name)
	if err == nil {
		a.afterWrite()
	}
	return out, err
}

// --- BACKUP tunnel (v1.14) ---

// SaveBackupPlan crée/met à jour un plan de sauvegarde v1.14 (générique ou ciblé). oldName = nom avant
// édition (vide à la création) : un renommage remplace l'entrée au lieu d'en créer une seconde.
func (a *App) SaveBackupPlan(oldName string, in config.BackupPlan) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.SaveBackupPlan(oldName, in)
}

// RunBackupPlan exécute un plan v1.14 (déclencheur « manual » depuis la fenêtre). Sans PIN : une
// sauvegarde est une routine additive qui doit se faire (voir authorizeBackupCloud).
func (a *App) RunBackupPlan(name string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	out, err := a.svc.RunBackupPlan(name, "manual")
	if err == nil {
		a.afterWrite()
	}
	return out, err
}

// BackupRuns renvoie le journal des sauvegardes.
func (a *App) BackupRuns() ([]config.BackupRun, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.BackupRuns(), nil
}

// --- KERNEL (v1.11) : la mémoire, lisible ---
// La fenêtre (humain) a l'accès complet, DÉSIGNATION comprise : c'est l'humain qui choisit ce qui
// fait office de mémoire. La désignation (AddKernel/RemoveKernel/SetActiveKernel) n'est PAS exposée
// au socket/CLI/MCP — un agent ne choisit jamais sa mémoire.
