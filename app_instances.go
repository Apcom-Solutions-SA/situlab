// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package main

import (
	"situlab/internal/config"
	"situlab/internal/core"
	"situlab/internal/forgejo"
)

// Doctor renvoie l'état de l'environnement.
func (a *App) Doctor() (core.Doctor, error) {
	if err := a.ready(); err != nil {
		return core.Doctor{}, err
	}
	return a.svc.Doctor(), nil
}

// GetInstances renvoie les instances configurées.
func (a *App) GetInstances() ([]config.Instance, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.Instances(), nil
}

// AddInstance ajoute une instance avec un token déjà en main (teste avant de persister).
func (a *App) AddInstance(name, url, token string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.AddInstance(name, url, token)
}

// AddInstanceToken ajoute une instance par JETON déjà en main. provider vaut "forgejo"
// (URL requise) ou "github" (l'utilisateur colle un Personal Access Token ; l'URL est
// fixée à github.com). Teste la connexion avant de persister.
func (a *App) AddInstanceToken(name, provider, url, token string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.AddInstanceToken(name, provider, url, token)
}

// EditInstance : modifier une connexion git existante (URL + jeton optionnel). Nom stable.
func (a *App) EditInstance(name, newURL, newToken string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.EditInstance(name, newURL, newToken)
}

// AddInstanceLogin : onboarding « PIN-only ». Identifiant + mot de passe (+ code 2FA si le
// compte en a un) ; SituLab crée le token via l'API, le chiffre par le PIN, et ne stocke ni
// le mot de passe ni l'OTP. Si otp est vide et que le serveur exige la 2FA, l'appel échoue
// (le frontend révèle alors le champ code 2FA et réessaie).
func (a *App) AddInstanceLogin(name, url, user, password, otp string, adminScope, remember bool) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.AddInstanceLogin(name, url, user, password, otp, adminScope, remember)
}

// RemoveInstance supprime une instance.
func (a *App) RemoveInstance(name string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.RemoveInstance(name)
}

// SetDefaultInstance définit l'instance par défaut.
func (a *App) SetDefaultInstance(name string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.lockedErr(a.svc.SetDefaultInstance(name))
}

// TestInstance teste une connexion.
func (a *App) TestInstance(name string) (*forgejo.User, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.TestInstance(name)
}

// Whoami renvoie l'utilisateur courant.
func (a *App) Whoami(instance string) (*forgejo.User, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.Whoami(instance)
}

// WhoamiProbe : variante de Whoami pour la sonde « en ligne » PÉRIODIQUE de la fenêtre. Identique, mais
// ne prolonge pas la session (le jeton est lu sans glissement d'inactivité) — ainsi l'auto-verrouillage
// tombe même si l'on reste sur une vue connectée sans interagir.
func (a *App) WhoamiProbe(instance string) (*forgejo.User, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.WhoamiProbe(instance)
}

// Dashboard renvoie la vue unifiée distant+local (objet central de SituLab).
func (a *App) Dashboard(instance string) (*core.DashboardData, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.Dashboard(instance)
}

// ActiveTokenName : nom exact du token utilisé par SituLab pour l'instance (à protéger).
func (a *App) ActiveTokenName(instance string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.svc.ActiveTokenName(instance), nil
}

// DisconnectedInstances : instances sans token valide (à reconnecter).
func (a *App) DisconnectedInstances() ([]string, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.DisconnectedInstances(), nil
}

// SetInstanceRole fixe le rôle d'une connexion (atelier|production). Fenêtre uniquement (v1.7).
func (a *App) SetInstanceRole(name, role string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.SetInstanceRole(name, role)
}

// SetInstancePINPolicy remplace la politique PIN d'une connexion (policy nil = défauts du rôle).
func (a *App) SetInstancePINPolicy(name string, policy *config.PINPolicy) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.SetInstancePINPolicy(name, policy)
}

// --- Projets atelier↔production (v1.7.1) ---

// AdminScopeFor indique si le jeton de l'instance porte déjà le droit d'administration (v1.4.2) :
// si non (et que le compte est admin), la fenêtre propose « Activer l'administration ».
func (a *App) AdminScopeFor(instance string) (bool, error) {
	if err := a.ready(); err != nil {
		return false, err
	}
	return a.svc.AdminScopeFor(instance), nil
}

// --- Gestionnaire de tokens ---
// ReconnectInstance recrée le token d'une instance (portée admin en option) — sans supprimer/
// ré-ajouter. Le mot de passe n'est mémorisé que si remember est vrai.
func (a *App) ReconnectInstance(name, user, password, otp string, adminScope, remember bool) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.ReconnectInstance(name, user, password, otp, adminScope, remember)
}

// ReconnectToken remet un jeton pour une instance existante (GitHub : nouveau PAT collé).
func (a *App) ReconnectToken(name, token string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.ReconnectToken(name, token)
}

// RememberPassword mémorise le mot de passe (chiffré par le PIN) après un 1er déverrouillage
// réussi du gestionnaire de jetons — pour ne plus le redemander ensuite.
func (a *App) RememberPassword(instance, password string, twoFA bool) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.RememberPassword(instance, password, twoFA)
}
