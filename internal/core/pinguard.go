// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"situlab/internal/config"
)

// pinGuardState persiste l'état du verrou anti-bruteforce du PIN pour qu'il SURVIVE à un
// redémarrage de l'application : sans cela, fermer puis rouvrir la fenêtre (= le démon)
// réinitialisait le verrou « 5 essais / 60 s ».
//
// Fichier DÉDIÉ (jamais config.json) : un champ relu/réécrit par chaque mutation de config serait
// écrasable par une écriture concurrente — la faille corrigée en 1.1.0. On garde donc le compteur
// vivant en mémoire (seul écrivain = le démon) et on ne fait QUE le refléter sur disque ici.
//
// Fail-safe : fichier absent ou illisible → on repart neuf (jamais de verrou injustifié qui
// bloquerait l'utilisateur légitime). Ce n'est pas une frontière de sécurité dure — un process du
// même compte OS peut l'effacer, comme il peut déjà lire la mémoire du démon déverrouillé — mais un
// durcissement best-effort contre le contournement du verrou par simple redémarrage.
type pinGuardState struct {
	Fails     int   `json:"fails"`
	LockUntil int64 `json:"lock_until"` // instant de fin de verrou (Unix s) ; 0 = pas de verrou
}

func pinGuardPath() string { return filepath.Join(config.Dir(), "pinguard.json") }

// readPINGuard lit l'état persisté. Erreur si absent/illisible (traité en fail-safe par l'appelant).
func readPINGuard() (pinGuardState, error) {
	var g pinGuardState
	b, err := os.ReadFile(pinGuardPath())
	if err != nil {
		return g, err
	}
	err = json.Unmarshal(b, &g)
	return g, err
}

// writePINGuard écrit l'état de façon atomique (temp + rename, chmod 0600), même discipline que
// config/keyring. Best-effort chez l'appelant.
func writePINGuard(g pinGuardState) error {
	dir := config.Dir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(g)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "pinguard-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op après un rename réussi
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	_ = os.Chmod(tmpName, 0o600)
	return os.Rename(tmpName, pinGuardPath())
}

// loadPINGuard restaure le verrou anti-bruteforce persisté au démarrage du service. Fail-safe :
// toute erreur laisse le compteur vierge.
func (s *Service) loadPINGuard() {
	g, err := readPINGuard()
	if err != nil {
		return
	}
	now := time.Now().Unix()
	s.mu.Lock()
	if g.LockUntil > now {
		s.pinLockTo = g.LockUntil
	}
	if g.Fails > 0 && g.Fails <= pinMaxFails {
		s.pinFails = g.Fails
	}
	s.mu.Unlock()
}

// savePINGuard reflète l'état courant du verrou sur disque (best-effort, hors s.mu). Sérialisé par
// pinGuardMu pour ne jamais entrelacer deux écritures (indépendant de ioMu : fichier distinct).
func (s *Service) savePINGuard() {
	s.mu.Lock()
	g := pinGuardState{Fails: s.pinFails, LockUntil: s.pinLockTo}
	s.mu.Unlock()
	s.pinGuardMu.Lock()
	defer s.pinGuardMu.Unlock()
	_ = writePINGuard(g)
}
