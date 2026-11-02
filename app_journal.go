// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package main

import "situlab/internal/journal"

// Journal local des opérations (1.20) — exposé à la fenêtre (Réglages → Journaux). Lecture libre
// (aucun secret) ; le niveau est un réglage, pas une mutation de contenu (pas de PIN).

func (a *App) JournalRead(limit int, onlyErr bool) ([]journal.Entry, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.JournalRead(limit, onlyErr)
}

func (a *App) JournalLevel() string {
	if a.svc == nil {
		return string(journal.LevelErrors)
	}
	return a.svc.JournalLevel()
}

func (a *App) SetJournalLevel(level string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.SetJournalLevel(level)
}

func (a *App) JournalClear() error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.JournalClear()
}
