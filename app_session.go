// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package main

// PinSet indique si un PIN est configuré.
func (a *App) PinSet() (bool, error) {
	if err := a.ready(); err != nil {
		return false, err
	}
	return a.svc.PINSet(), nil
}

// SetPin définit ou change le PIN (ancien requis s'il en existe un).
func (a *App) SetPin(oldCode, newCode string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.SetPIN(oldCode, newCode)
}

// Unlocked indique si la session de lecture est ouverte.
func (a *App) Unlocked() (bool, error) {
	if err := a.ready(); err != nil {
		return false, err
	}
	return a.svc.Unlocked(), nil
}

// Unlock ouvre la session de lecture avec le PIN (saisi dans la fenêtre). Indispensable au
// retour dans l'app : sans session, toutes les lectures échouent (ErrLocked).
func (a *App) Unlock(pin string) error {
	if err := a.ready(); err != nil {
		return err
	}
	if err := a.svc.Unlock(pin); err != nil {
		return err
	}
	// Plus de sauvegarde « à l'ouverture » (1.18.1) : sauvegarder en ouvrant n'a pas de sens — la
	// sauvegarde est le geste de FERMETURE (voir beforeClose). On ne lance donc rien ici.
	return nil
}

// Lock referme la session (oublie la clé en mémoire).
func (a *App) Lock() error {
	if err := a.ready(); err != nil {
		return err
	}
	a.svc.Lock()
	return nil
}

// ResetKeyring efface le trousseau (PIN oublié, irrécupérable) et ramène à l'onboarding.
func (a *App) ResetKeyring() error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.ResetKeyring()
}

// LockDelay / SetLockDelay : délai d'auto-verrouillage en minutes (-1 = jamais, 0 = défaut 15).
func (a *App) LockDelay() (int, error) {
	if err := a.ready(); err != nil {
		return 0, err
	}
	return a.svc.LockDelay(), nil
}

func (a *App) SetLockDelay(min int) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.SetLockDelay(min)
}

func (a *App) LockAfterWrite() (bool, error) {
	if err := a.ready(); err != nil {
		return false, err
	}
	return a.svc.LockAfterWrite(), nil
}

func (a *App) SetLockAfterWrite(on bool) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.SetLockAfterWrite(on)
}

// ReadOnly / SetReadOnly : mode lecture seule (v1.4) — bloque toute mutation.
func (a *App) ReadOnly() (bool, error) {
	if err := a.ready(); err != nil {
		return false, err
	}
	return a.svc.ReadOnly(), nil
}

func (a *App) SetReadOnly(on bool) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.SetReadOnly(on)
}

// RevalidateSecurityPolicy ré-ancre (re-signe) la politique de sécurité LOCALE sur ce poste. FENÊTRE
// UNIQUEMENT (jamais exposé au socket/CLI/MCP, pour qu'un appelant ne puisse pas « bénir » une
// falsification). Récupération après une clé d'appareil renouvelée (restauration d'une sauvegarde /
// migration de poste) ou une édition hors SituLab : la politique courante redevient de confiance. À
// n'utiliser qu'après avoir VÉRIFIÉ les réglages de sécurité (rôles, périmètres, PIN, lecture seule).
func (a *App) RevalidateSecurityPolicy() error {
	if err := a.ready(); err != nil {
		return err
	}
	a.svc.ReanchorPolicy()
	return nil
}

// SecurityPolicyNeedsRevalidation : la politique de ce poste est-elle signée mais NON reconnue (clé
// d'appareil renouvelée, mise à niveau, édition hors SituLab) ? Si oui, SituLab reste en position fermée
// (lecture seule) et l'UI propose « re-valider la politique » (seul geste qui débloque).
func (a *App) SecurityPolicyNeedsRevalidation() (bool, error) {
	if err := a.ready(); err != nil {
		return false, err
	}
	return a.svc.SecurityPolicyTampered(), nil
}
