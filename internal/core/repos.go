// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"situlab/internal/config"
	"situlab/internal/forgejo"
)

// Repos renvoie les dépôts accessibles (via le cache partagé, comme le dashboard).
func (s *Service) Repos(instance string) ([]forgejo.Repo, error) {
	cl, inst, err := s.clientFor(instance)
	if err != nil {
		return nil, err
	}
	ctx, cancel := opCtx()
	defer cancel()
	repos, _, err := s.cachedRepos(ctx, inst.Name, cl)
	return repos, err
}

// ViewRepo renvoie le détail d'un dépôt owner/repo.
func (s *Service) ViewRepo(instance, owner, repo string) (*forgejo.Repo, error) {
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return nil, err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.ViewRepo(ctx, owner, repo)
}

// SetRepoDescription met à jour la description d'un dépôt distant (métadonnée, auth par token).
func (s *Service) SetRepoDescription(instance, owner, repo, desc, pin string) error {
	if err := s.authorize(instance, config.ActionRepoAdmin, pin); err != nil {
		return err
	}
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return err
	}
	ctx, cancel := opCtx()
	defer cancel()
	_, err = cl.EditRepo(ctx, owner, repo, map[string]any{"description": desc})
	return err
}

// Orgs renvoie les organisations.
func (s *Service) Orgs(instance string) ([]forgejo.Org, error) {
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return nil, err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.ListOrgs(ctx)
}

// CreateRepo crée un dépôt distant (écriture — PIN requis). Le PIN est vérifié AVANT de
// construire le client : il ouvre la session, dont clientFor a besoin pour le token.
func (s *Service) CreateRepo(instance, owner, name, desc string, private bool, pin string) (*forgejo.Repo, error) {
	if err := s.authorize(instance, config.ActionRepoAdmin, pin); err != nil {
		return nil, err
	}
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return nil, err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.CreateRepo(ctx, owner, name, desc, private)
}

// DeleteRepo supprime définitivement un dépôt distant (écriture — PIN requis).
func (s *Service) DeleteRepo(instance, owner, repo, pin string) error {
	if err := s.authorize(instance, config.ActionRepoAdmin, pin); err != nil {
		return err
	}
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.DeleteRepo(ctx, owner, repo)
}

// RepoIssues / RepoPulls / RepoReleases : lectures (pas de PIN).
func (s *Service) RepoIssues(instance, owner, repo, state string) ([]forgejo.Issue, error) {
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return nil, err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.ListIssues(ctx, owner, repo, state)
}

func (s *Service) RepoPulls(instance, owner, repo, state string) ([]forgejo.Issue, error) {
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return nil, err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.ListPulls(ctx, owner, repo, state)
}

func (s *Service) RepoReleases(instance, owner, repo string) ([]forgejo.Release, error) {
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return nil, err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.ListReleases(ctx, owner, repo)
}

// --- Administration (nécessite un token admin) ---
