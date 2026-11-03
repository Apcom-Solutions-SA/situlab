// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"situlab/internal/config"
	"situlab/internal/forgejo"
)

// TokenList liste les jetons du compte (lecture). otp = code 2FA si le compte en a un.
// Si password est vide, on utilise le mot de passe mémorisé (comptes sans 2FA) → PIN seul.
func (s *Service) TokenList(instance, user, password, otp string) ([]forgejo.Token, error) {
	if password == "" {
		password = s.storedPassword(instance)
	}
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return nil, err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.TokenList(ctx, user, password, otp)
}

// TokenCreate crée un jeton. Renvoie sa valeur. Si repos (liste de « owner/repo ») est non vide,
// le token est restreint à ces dépôts. otp = code 2FA. v1.5.0 : plus de PIN par action —
// l'authentification est le mot de passe Forgejo (+ 2FA) sur une session déverrouillée ; le mode
// lecture seule reste bloquant (assertWritable). Le paramètre pin n'est plus utilisé (compat).
func (s *Service) TokenCreate(instance, user, password, otp, name string, scopes, repos []string, pin string) (string, error) {
	// Création d'un jeton = mutation d'administration sensible (le jeton forgé a des droits
	// d'écriture) soumise à la politique PIN (ActionUserAdmin) en plus de l'auth mot de passe/2FA :
	// sinon un agent sur session déverrouillée utiliserait le mot de passe mémorisé et obtiendrait
	// un jeton d'écriture sans aucun PIN.
	if err := s.authorize(instance, config.ActionUserAdmin, pin); err != nil {
		return "", err
	}
	if password == "" {
		password = s.storedPassword(instance)
	}
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return "", err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.TokenCreate(ctx, user, password, otp, name, scopes, repos)
}

// TokenDelete supprime un jeton. otp = code 2FA. v1.5.0 : plus de PIN par action (voir
// TokenCreate) ; lecture seule bloquante. Le paramètre pin n'est plus utilisé (compat).
func (s *Service) TokenDelete(instance, user, password, otp string, id int64, pin string) error {
	if err := s.authorize(instance, config.ActionUserAdmin, pin); err != nil { // v1.7 : sous politique PIN (admin)
		return err
	}
	if password == "" {
		password = s.storedPassword(instance)
	}
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.TokenDelete(ctx, user, password, otp, id)
}
