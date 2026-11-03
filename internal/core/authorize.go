// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"errors"
	"situlab/internal/config"
	"situlab/internal/gitx"
)

// gatePIN exige un PIN valide pour une action écrivant vers le distant (push, création
// de dépôt/issue…). Passe par le chemin verrouillé anti-bruteforce. Un PIN correct
// rafraîchit aussi la session de lecture (confirmer une écriture ⇒ session déverrouillée).
func (s *Service) gatePIN(pinCode string) error {
	// Mode lecture seule (v1.4) : on refuse AVANT toute vérification de PIN. gatePIN couvre les
	// mutations DISTANTES (push, création/suppression, issues, PR, admin, jetons) ; les mutations
	// LOCALES (commit, pull, stash, branche…) et de config, qui ne passent pas par ici, sont
	// gardées séparément par assertWritable().
	if s.ReadOnly() {
		return ErrReadOnly
	}
	if pinCode == "" {
		if !s.PINSet() {
			return errors.New("aucun PIN configuré : définissez-en un d'abord")
		}
		return ErrPINNeeded
	}
	dek, ok, err := s.verifyPINGated(pinCode)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("PIN incorrect")
	}
	s.openSession(dek)
	zero(dek) // openSession en a pris une copie ; on efface l'original (DEK = clé maîtresse)
	return nil
}

// RequiresPIN expose la politique PIN par connexion (v1.7) : l'action distante a exige-t-elle le
// PIN sur cette instance, ou la session déverrouillée suffit-elle ? Fail-closed : instance inconnue
// → PIN exigé.
func (s *Service) RequiresPIN(instance string, a config.PINAction) bool {
	s.mu.Lock()
	s.lockedReload()
	cfg := s.cfg
	in := cfg.Resolve(instance)
	ok := s.policyOK(cfg) // intégrité de la politique (fermeture P3)
	s.mu.Unlock()
	if in == nil || !ok {
		return true // fail-closed : connexion inconnue OU politique falsifiée → PIN exigé
	}
	return in.RequiresPIN(a)
}

// authorize autorise une mutation DISTANTE selon la politique de la connexion (v1.7). Lecture seule
// → refus. Si la politique exige le PIN pour cette action → chemin gatePIN classique (qui ouvre la
// session). Sinon (ex. atelier autonome) → la session déjà déverrouillée suffit, sans PIN.
// Fail-closed : session fermée sur un chemin sans PIN → ErrLocked (le démon aurait dû déverrouiller).
func (s *Service) authorize(instance string, a config.PINAction, pin string) error {
	if s.ReadOnly() {
		return ErrReadOnly
	}
	if !s.RequiresPIN(instance, a) {
		if s.Unlocked() {
			return nil
		}
		return ErrLocked
	}
	return s.gatePIN(pin)
}

// RequiresPINForPush décide de l'exigence de PIN pour un ENVOI de code en liant la politique au
// DÉPÔT RÉELLEMENT poussé (son remote origin), pas seulement à la connexion nommée par l'appelant.
// Un agent ne peut donc pas pousser vers une production via une connexion atelier : on prend la
// politique la plus stricte entre la connexion nommée et celle déduite de l'origin du dépôt, et on
// exige le PIN (fail-closed) si l'origin ne correspond à aucune connexion unique (inconnu/ambigu).
func (s *Service) RequiresPINForPush(named, path string) bool {
	if s.RequiresPIN(named, config.ActionPush) {
		return true
	}
	m := s.instanceForPath(path)
	if m == "" {
		return true // remote inconnu ou hôte partagé par plusieurs connexions → prudence
	}
	return s.RequiresPIN(m, config.ActionPush)
}

// authorizePush autorise un envoi de code selon RequiresPINForPush (politique liée au dépôt réel).
func (s *Service) authorizePush(named, path, pin string) error {
	if s.ReadOnly() {
		return ErrReadOnly
	}
	if !s.RequiresPINForPush(named, path) {
		if s.Unlocked() {
			return nil
		}
		return ErrLocked
	}
	return s.gatePIN(pin)
}

// instanceForPath déduit la connexion propriétaire d'un dépôt local à partir de l'hôte de son
// remote origin. Renvoie "" si inconnu ou si l'hôte est partagé par plusieurs connexions.
func (s *Service) instanceForPath(path string) string {
	st, err := gitx.StatusOf(path)
	if err != nil || st.Remote == "" {
		return ""
	}
	return s.instanceForHost(hostOf(st.Remote))
}

// --- Écritures et lectures API étendues (v1) ---
