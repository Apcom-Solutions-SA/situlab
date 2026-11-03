// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"situlab/internal/config"
	"situlab/internal/forgejo"
)

// CreateIssue crée une issue (écriture — PIN requis).
func (s *Service) CreateIssue(instance, owner, repo, title, body, pin string) (*forgejo.Issue, error) {
	if err := s.authorize(instance, config.ActionIssues, pin); err != nil {
		return nil, err
	}
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return nil, err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.CreateIssue(ctx, owner, repo, title, body)
}

// CommentIssue ajoute un commentaire à une issue/PR (écriture — PIN requis).
func (s *Service) CommentIssue(instance, owner, repo string, index int, body, pin string) error {
	if err := s.authorize(instance, config.ActionIssues, pin); err != nil {
		return err
	}
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.CommentIssue(ctx, owner, repo, index, body)
}

// SetIssueState ferme/rouvre une issue (écriture — PIN requis).
func (s *Service) SetIssueState(instance, owner, repo string, index int, state, pin string) error {
	if err := s.authorize(instance, config.ActionIssues, pin); err != nil {
		return err
	}
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.SetIssueState(ctx, owner, repo, index, state)
}

// EditIssue modifie titre/corps d'une issue (écriture — PIN requis).
func (s *Service) EditIssue(instance, owner, repo string, index int, title, body, pin string) error {
	if err := s.authorize(instance, config.ActionIssues, pin); err != nil {
		return err
	}
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.EditIssue(ctx, owner, repo, index, title, body)
}

// CreatePull ouvre une demande de fusion (écriture — PIN requis).
func (s *Service) CreatePull(instance, owner, repo, title, head, base, body, pin string) (*forgejo.Issue, error) {
	if err := s.authorize(instance, config.ActionIssues, pin); err != nil {
		return nil, err
	}
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return nil, err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.CreatePull(ctx, owner, repo, title, head, base, body)
}

// MergePull fusionne une demande de fusion (écriture — PIN requis).
func (s *Service) MergePull(instance, owner, repo string, index int, method, pin string) error {
	if err := s.authorize(instance, config.ActionIssues, pin); err != nil {
		return err
	}
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.MergePull(ctx, owner, repo, index, method)
}
