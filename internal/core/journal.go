// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"time"

	"situlab/internal/config"
	"situlab/internal/journal"
)

// LogOp enregistre une opération au journal local (best-effort, jamais fatal, jamais bloquant pour
// l'appelant). actor = « fenetre » | « cli » | « mcp ». target : dépôt/chemin concerné (facultatif).
func (s *Service) LogOp(actor, op, target string, ok bool, err error, durMs int64, intent string) {
	if s.jnl == nil {
		return
	}
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	s.jnl.Log(journal.Entry{
		Time: time.Now(), Actor: actor, Op: op, Target: target,
		OK: ok, Err: msg, DurMs: durMs, Intent: intent,
	})
}

// JournalRead renvoie les dernières entrées (récentes d'abord), au plus limit ; onlyErr ne garde que
// les échecs. Lecture libre (pas de secret dans le journal).
func (s *Service) JournalRead(limit int, onlyErr bool) ([]journal.Entry, error) {
	if s.jnl == nil {
		return nil, nil
	}
	return s.jnl.Read(limit, onlyErr)
}

// JournalLevel renvoie le niveau courant (off | errors | all).
func (s *Service) JournalLevel() string {
	if s.jnl == nil {
		return string(journal.LevelErrors)
	}
	return string(s.jnl.Level())
}

// SetJournalLevel change le niveau et le persiste. Geste de réglage (fenêtre ou CLI), pas une mutation
// de contenu — pas de PIN.
func (s *Service) SetJournalLevel(level string) error {
	l := journal.NormalizeLevel(level)
	s.ioMu.Lock()
	cfg, err := config.Load()
	if err != nil {
		s.ioMu.Unlock()
		return err
	}
	cfg.JournalLevel = string(l)
	if err := cfg.Save(); err != nil {
		s.ioMu.Unlock()
		return err
	}
	s.publish(cfg)
	s.ioMu.Unlock()
	if s.jnl != nil {
		s.jnl.SetLevel(l)
	}
	return nil
}

// JournalClear efface le journal (fenêtre / CLI).
func (s *Service) JournalClear() error {
	if s.jnl == nil {
		return nil
	}
	return s.jnl.Clear()
}
