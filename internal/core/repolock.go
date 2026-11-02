// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"path/filepath"
	"strings"
	"sync"
)

// Verrou PAR DÉPÔT (v1.9.2) : sérialise les opérations git touchant un même .git, pour qu'une action
// d'agent (push/pull/sync/commit) et la récupération de fond (FetchAllLocal) ne lancent JAMAIS deux
// processus git concurrents sur le même dépôt. Deux `git fetch`/`push` simultanés se disputent les
// verrous de refs (« cannot lock ref », « another git process is running ») → échecs transitoires et,
// surtout, un état lu pendant une écriture de refs concurrente peut induire l'agent en erreur. Le
// verrou est fin (par chemin) : des dépôts différents restent parallèles.
//
// Clé normalisée pour que deux graphies du même chemin (séparateurs, casse Windows) partagent le verrou.

func repoKey(path string) string {
	p := filepath.Clean(path)
	if filepath.Separator == '\\' { // Windows : système de fichiers insensible à la casse
		p = strings.ToLower(p)
	}
	return p
}

func (s *Service) repoMu(path string) *sync.Mutex {
	m, _ := s.repoLocks.LoadOrStore(repoKey(path), &sync.Mutex{})
	return m.(*sync.Mutex)
}

// lockRepo prend le verrou du dépôt et renvoie la fonction de libération (usage : defer s.lockRepo(p)()).
// À la libération, on SIGNALE que l'état git a pu changer : toute mutation locale ou distante d'un dépôt
// passe par ce verrou (commit, merge, rebase, reset, push, fetch, pull, sync, tag local…), donc c'est le
// point unique où relancer le rafraîchissement de l'UI (optimisation 1.20). Un sur-déclenchement (geste
// sans effet, échec) est inoffensif : l'UI compare les signatures et ne repeint que si besoin.
func (s *Service) lockRepo(path string) func() {
	m := s.repoMu(path)
	m.Lock()
	return func() {
		m.Unlock()
		s.SignalGitChange()
	}
}

// SetGitChanged enregistre le relais « l'état git a changé » (l'App le câble sur un événement Wails
// « git:changed »). Posé une fois au démarrage, avant tout geste.
func (s *Service) SetGitChanged(fn func()) { s.gitChg = fn }

// SignalGitChange notifie l'UI qu'une mutation git a eu lieu (best-effort, non bloquant, nil-safe).
// Déclenché à la libération du verrou de dépôt (gestes locaux/distants portant sur un dépôt local) et
// appelé par les frontières (démon, fenêtre) pour les gestes d'API distante qui ne verrouillent aucun
// dépôt local (création de dépôt/version/tag côté serveur, suppression de branche distante…).
func (s *Service) SignalGitChange() {
	if cb := s.gitChg; cb != nil {
		cb()
	}
}

// tryLockRepo tente de prendre le verrou sans bloquer. Renvoie (release, true) si obtenu, (nil, false)
// sinon — utilisé par la récupération de fond pour SAUTER un dépôt occupé par une action au premier plan.
func (s *Service) tryLockRepo(path string) (func(), bool) {
	m := s.repoMu(path)
	if !m.TryLock() {
		return nil, false
	}
	return m.Unlock, true
}
