// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package main

import "situlab/internal/forgejo"

// Repos liste les dépôts accessibles d'une connexion (utilisé par le sélecteur de la modale projet).
func (a *App) Repos(instance string) ([]forgejo.Repo, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.Repos(instance)
}

// Orgs liste les organisations de l'utilisateur sur une instance — pour choisir le propriétaire d'un
// nouveau dépôt, y compris une organisation où l'on n'a encore AUCUN dépôt (la liste dérivée des dépôts
// existants les manquait). Lecture, pas de PIN.
func (a *App) Orgs(instance string) ([]forgejo.Org, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.Orgs(instance)
}

// RepoBranches / RepoTags / RepoCommits / RepoContents / RepoFile : détail distant.
func (a *App) RepoBranches(instance, owner, repo string) ([]forgejo.Branch, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.RepoBranches(instance, owner, repo)
}

func (a *App) RepoTags(instance, owner, repo string) ([]forgejo.Tag, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.RepoTags(instance, owner, repo)
}

func (a *App) RepoCommits(instance, owner, repo, ref string, limit int) ([]forgejo.Commit, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.RepoCommits(instance, owner, repo, ref, limit)
}

func (a *App) RepoContents(instance, owner, repo, path, ref string) ([]forgejo.Entry, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.RepoContents(instance, owner, repo, path, ref)
}

func (a *App) RepoFile(instance, owner, repo, path, ref string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.svc.RepoFile(instance, owner, repo, path, ref)
}

// BranchTags liste les versions (tags) joignables depuis une branche d'un clone local, de la plus
// récente à la plus ancienne — pour que l'utilisateur choisisse QUELLE version promouvoir.
func (a *App) BranchTags(path, branch string) ([]string, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.BranchTags(path, branch)
}
