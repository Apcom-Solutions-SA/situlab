// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"fmt"
	"situlab/internal/config"
	"situlab/internal/gitx"
)

// AddLocalRepo enregistre un dépôt local à suivre (après vérification que c'en est un).
func (s *Service) AddLocalRepo(path string) error {
	if err := s.assertWritable(); err != nil {
		return err
	}
	if !gitx.Available() {
		return ErrNoGit
	}
	st, err := gitx.StatusOf(path)
	if err != nil {
		return err
	}
	if !st.IsRepo {
		// Les worktrees et sous-modules initialisés (dont le .git est un FICHIER de lien) sont
		// acceptés normalement. Un refus ici signifie que git ne voit aucun dépôt utilisable au
		// chemin indiqué — le plus souvent un sous-module PAS ENCORE initialisé.
		return fmt.Errorf("« %s » n'est pas (encore) un dépôt git. S'il s'agit d'un sous-module, initialisez-le d'abord (git submodule update --init) ; d'un worktree, vérifiez que son dépôt principal existe", path)
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.AddLocalRepo(path)
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}

// LocalStatus renvoie le statut git détaillé d'un dossier.
func (s *Service) LocalStatus(path string) (*gitx.Status, error) {
	if !gitx.Available() {
		return nil, ErrNoGit
	}
	return gitx.StatusOf(path)
}

// LocalDetail renvoie l'état détaillé d'un dépôt local (commits, branches, non poussés).
func (s *Service) LocalDetail(path string) (*gitx.Detail, error) {
	if !gitx.Available() {
		return nil, ErrNoGit
	}
	return gitx.DetailOf(path)
}

// BranchMatrix renvoie l'état par branche d'un dépôt local (ce poste vs en ligne,
// versions et dates). Local, sans réseau, à la demande.
func (s *Service) BranchMatrix(path string) (*gitx.BranchMatrix, error) {
	if !gitx.Available() {
		return nil, ErrNoGit
	}
	return gitx.MatrixOf(path)
}

// RemoveLocalRepo cesse de suivre un dépôt local (ne supprime rien sur le disque).
func (s *Service) RemoveLocalRepo(path string) error {
	if err := s.assertWritable(); err != nil {
		return err
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	clean := config.CleanPath(path)
	out := cfg.LocalRepos[:0]
	for _, p := range cfg.LocalRepos {
		if config.CleanPath(p) != clean {
			out = append(out, p)
		}
	}
	cfg.LocalRepos = out
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}

// --- PIN & push ---
