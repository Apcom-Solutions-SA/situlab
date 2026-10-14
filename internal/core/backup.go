// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"situlab/internal/config"
	"situlab/internal/gitx"
)

// ErrNoBackup : aucun plan de sauvegarde ne correspond.
var ErrNoBackup = errors.New("plan de sauvegarde introuvable")

const backupUploadTimeout = 5 * time.Minute // un bundle peut être volumineux

// Backups renvoie une copie des plans de sauvegarde configurés.
func (s *Service) Backups() []config.BackupPlan {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lockedReload()
	return append([]config.BackupPlan(nil), s.cfg.Backups...)
}

// AddBackupPlan enregistre (ou met à jour) un plan « dépôt git → dossier cloud ». Configuration
// (pas un geste de sécurité) : exposable sur les 3 surfaces. Valide que le dépôt est un dépôt git
// et que la connexion cloud existe.
func (s *Service) AddBackupPlan(name, repoPath, cloud, dest string) error {
	if err := s.assertWritable(); err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	repoPath = strings.TrimSpace(repoPath)
	cloud = strings.TrimSpace(cloud)
	if name == "" || repoPath == "" || cloud == "" {
		return errors.New("nom, dépôt et cloud sont requis")
	}
	if gitx.Available() {
		if st, err := gitx.StatusOf(repoPath); err != nil || !st.IsRepo {
			return fmt.Errorf("« %s » n'est pas un dépôt git", repoPath)
		}
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.FindCloud(cloud) == nil {
		return ErrNoCloud
	}
	plan := config.BackupPlan{
		Name:     name,
		RepoPath: config.CleanPath(repoPath),
		Cloud:    cloud,
		Dest:     config.CloudCleanPath(dest),
	}
	if ex := cfg.FindBackup(name); ex != nil {
		ex.RepoPath, ex.Cloud, ex.Dest = plan.RepoPath, plan.Cloud, plan.Dest // mise à jour
	} else {
		cfg.Backups = append(cfg.Backups, plan)
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}

// RemoveBackupPlan supprime un plan de sauvegarde.
func (s *Service) RemoveBackupPlan(name string) error {
	if err := s.assertWritable(); err != nil {
		return err
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if !cfg.RemoveBackup(name) {
		return ErrNoBackup
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}

// RunBackup exécute un plan : bundle git de tout l'historique → envoi dans le dossier cloud.
// Gardé par la politique PIN d'écriture de la connexion cloud de destination. Renvoie le chemin
// cloud du fichier écrit.
func (s *Service) RunBackup(name, pin string) (string, error) {
	s.mu.Lock()
	s.lockedReload()
	bp := s.cfg.FindBackup(name)
	s.mu.Unlock()
	if bp == nil {
		return "", ErrNoBackup
	}
	plan := *bp

	if err := s.authorizeCloud(plan.Cloud, config.CloudWrite, pin); err != nil {
		return "", err
	}
	if !gitx.Available() {
		return "", errors.New("git indisponible sur ce poste")
	}
	if st, err := gitx.StatusOf(plan.RepoPath); err != nil || !st.IsRepo {
		return "", fmt.Errorf("« %s » n'est pas un dépôt git", plan.RepoPath)
	}

	base := filepathBase(plan.RepoPath)
	if base == "" {
		base = "repo"
	}
	fname := base + "-" + time.Now().Format("20060102-150405") + ".bundle"
	tmp := filepath.Join(os.TempDir(), fname)
	defer os.Remove(tmp)
	if err := gitx.Bundle(plan.RepoPath, tmp); err != nil {
		return "", fmt.Errorf("création du bundle : %w", err)
	}
	content, err := os.ReadFile(tmp)
	if err != nil {
		return "", err
	}

	remote := path.Join("/"+strings.Trim(config.CloudCleanPath(plan.Dest), "/"), fname)
	cl, _, err := s.fileCloudClientFor(plan.Cloud)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), backupUploadTimeout)
	defer cancel()
	if err := cl.Put(ctx, remote, content); err != nil {
		return "", err
	}

	// Horodate le dernier backup réussi (best-effort).
	s.ioMu.Lock()
	if cfg, err := config.Load(); err == nil {
		if b := cfg.FindBackup(name); b != nil {
			b.LastRun = time.Now().Format(time.RFC3339)
			if cfg.Save() == nil {
				s.publish(cfg)
			}
		}
	}
	s.ioMu.Unlock()
	return remote, nil
}
