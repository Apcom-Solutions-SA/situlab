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
func (s *Service) lockRepo(path string) func() {
	m := s.repoMu(path)
	m.Lock()
	return m.Unlock
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
