// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package main

import (
	"situlab/internal/attest"
	"situlab/internal/core"
	"situlab/internal/gitx"
)

// AttestHistory : derniers commits locaux + état d'attestation (lecture).
func (a *App) AttestHistory(path string, limit int) ([]core.AttestedCommit, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.AttestHistory(path, limit)
}

// Prompts : bibliothèque de prompts (défauts livrés + personnalisations utilisateur).
func (a *App) Prompts() ([]core.PromptView, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.Prompts(), nil
}

// SavePrompt : enregistre/surcharge un prompt (id vide = nouveau prompt utilisateur).
func (a *App) SavePrompt(id, title, body string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.SavePrompt(id, title, body)
}

// DeletePrompt : masque un prompt par défaut ou supprime un prompt utilisateur.
func (a *App) DeletePrompt(id string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.DeletePrompt(id)
}

// ResetPrompt : rétablit un prompt par défaut (retire surcharge/masquage).
func (a *App) ResetPrompt(id string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.ResetPrompt(id)
}

// FileHistory : commits qui ont modifié un fichier (clone local) — pour l'onglet Historique d'un fichier.
func (a *App) FileHistory(path, file string, limit int) ([]gitx.LocalCommit, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.FileHistory(path, file, limit)
}

// FileDiffAt : diff d'un commit restreint à un fichier (clone local).
func (a *App) FileDiffAt(path, sha, file string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.svc.FileDiffAt(path, sha, file)
}

// AttestUsingTestRoot indique si la racine de signature embarquée est encore la racine de TEST
// (signature « de test », forgeable). L'interface affiche un bandeau d'avertissement tant que c'est
// le cas — à remplacer par la vraie racine de gouvernance avant mise en prod de la signature.
func (a *App) AttestUsingTestRoot() bool { return attest.UsingTestRoot() }

// AttestSoftwareActive indique si la SIGNATURE LOGICIELLE « SituLab » est active (identité unique
// embarquée dans le binaire) — le logiciel signe, indépendamment de l'utilisateur.
func (a *App) AttestSoftwareActive() bool { return attest.HasSoftwareSigner() }

// GenerateSignKey : génère la clé d'appareil d'une instance, renvoie sa clé publique à faire certifier.
func (a *App) GenerateSignKey(instance string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.svc.GenerateSignKey(instance)
}

// InstallSignCert : installe le certificat d'appareil émis par la gouvernance.
func (a *App) InstallSignCert(instance, certJSON string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.InstallSignCert(instance, certJSON)
}
