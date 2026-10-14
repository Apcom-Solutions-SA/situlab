// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

// SPOKES (v1.11) : projections de mission dérivées d'un KERNEL. Un spoke est un dépôt local désigné,
// qui référence le KERNEL dont il dérive et porte une mission. Même doctrine que le KERNEL : lecture
// (liste, état, contenu) et gestes mémoire (récupérer, enregistrer) sur les trois surfaces ; la
// DÉSIGNATION (ajout/retrait) est réservée à la FENÊTRE. Réutilise les helpers de kernel.go.

package core

import (
	"errors"
	"fmt"
	"strings"

	"situlab/internal/config"
	"situlab/internal/gitx"
)

// ErrNoSpoke : aucun spoke ne correspond.
var ErrNoSpoke = errors.New("spoke introuvable")

// Spokes renvoie les spokes configurés.
func (s *Service) Spokes() []config.Spoke {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lockedReload()
	return append([]config.Spoke(nil), s.cfg.Spokes...)
}

func (s *Service) resolveSpoke(name string) (config.Spoke, error) {
	s.mu.Lock()
	s.lockedReload()
	sp := s.cfg.FindSpoke(name)
	s.mu.Unlock()
	if sp == nil {
		return config.Spoke{}, ErrNoSpoke
	}
	return *sp, nil
}

// AddSpoke désigne un dépôt local comme spoke (mission dérivée d'un KERNEL). FENÊTRE UNIQUEMENT.
func (s *Service) AddSpoke(name, localPath, kernel, mission, instance string) error {
	if err := s.assertWritable(); err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	localPath = strings.TrimSpace(localPath)
	if name == "" || localPath == "" {
		return errors.New("nom et dépôt local sont requis")
	}
	if gitx.Available() {
		if st, err := gitx.StatusOf(localPath); err != nil || !st.IsRepo {
			return fmt.Errorf("« %s » n'est pas un dépôt git", localPath)
		}
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	sp := config.Spoke{
		Name:      name,
		LocalPath: config.CleanPath(localPath),
		Kernel:    strings.TrimSpace(kernel),
		Mission:   strings.TrimSpace(mission),
		Instance:  strings.TrimSpace(instance),
	}
	if ex := cfg.FindSpoke(name); ex != nil {
		ex.LocalPath, ex.Kernel, ex.Mission, ex.Instance = sp.LocalPath, sp.Kernel, sp.Mission, sp.Instance
	} else {
		cfg.Spokes = append(cfg.Spokes, sp)
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}

// RemoveSpoke retire un spoke de la liste (ne touche pas au dépôt). FENÊTRE UNIQUEMENT.
func (s *Service) RemoveSpoke(name string) error {
	if err := s.assertWritable(); err != nil {
		return err
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if !cfg.RemoveSpoke(name) {
		return ErrNoSpoke
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}

// SetSpokeRunFile définit le fichier d'ouverture (run) d'un spoke, par son chemin local. "" = aucun.
func (s *Service) SetSpokeRunFile(localPath, runFile string) error {
	if err := s.assertWritable(); err != nil {
		return err
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cp := config.CleanPath(localPath)
	found := false
	for i := range cfg.Spokes {
		if config.CleanPath(cfg.Spokes[i].LocalPath) == cp {
			cfg.Spokes[i].RunFile = strings.TrimSpace(runFile)
			found = true
		}
	}
	if !found {
		return ErrNoSpoke
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}

// SpokeStatus lit l'état local d'un spoke (réutilise le calcul du KERNEL).
func (s *Service) SpokeStatus(name string) (*KernelStatus, error) {
	sp, err := s.resolveSpoke(name)
	if err != nil {
		return nil, err
	}
	return s.repoMemStatus(sp.Name, sp.LocalPath, sp.Instance, "Spoke à jour.")
}

// SpokeContents dresse la carte du contenu d'un spoke.
func (s *Service) SpokeContents(name string) ([]KernelEntry, error) {
	sp, err := s.resolveSpoke(name)
	if err != nil {
		return nil, err
	}
	return repoMemContents(sp.LocalPath)
}

// SpokePull récupère les dernières nouveautés d'un spoke (fast-forward ; pas de PIN).
func (s *Service) SpokePull(name string) (string, error) {
	sp, err := s.resolveSpoke(name)
	if err != nil {
		return "", err
	}
	return s.Pull(sp.Instance, sp.LocalPath)
}

// SpokeSave enregistre et envoie le travail d'un spoke (récupérer → enregistrer → envoyer, un PIN).
func (s *Service) SpokeSave(name, message, pin string) (*SyncResult, error) {
	sp, err := s.resolveSpoke(name)
	if err != nil {
		return nil, err
	}
	return s.Synchronize(sp.Instance, sp.LocalPath, message, pin)
}
