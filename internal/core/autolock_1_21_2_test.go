// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"errors"
	"testing"
	"time"
)

// TestBackgroundProbeDoesNotSlideSession — 1.21.2 : une SONDE d'arrière-plan (auto-fetch des refs,
// sonde « en ligne » périodique) lit un jeton via le chemin PEEK, qui ne doit JAMAIS repousser
// l'échéance d'inactivité. Sinon, avec un intervalle de sonde < délai de verrouillage (p. ex. auto-fetch
// 7 min contre verrouillage 10 min), la session se réarme toute seule et l'auto-verrouillage ne tombe
// jamais. Le chemin USAGE normal (sessionDEK), lui, continue de glisser — c'est voulu : il garde la
// session vivante pendant un vrai travail humain OU agent (CLI/MCP).
func TestBackgroundProbeDoesNotSlideSession(t *testing.T) {
	s := newInTemp(t)
	if err := s.SetPIN("", "123456"); err != nil {
		t.Fatalf("SetPIN : %v", err)
	}

	// Échéance connue, encore valide, et TTL connu.
	s.mu.Lock()
	s.sessTTL = time.Hour
	fixed := time.Now().Add(10 * time.Minute)
	s.sessUntil = fixed
	s.mu.Unlock()

	// PEEK (sonde de fond) : ne doit PAS repousser l'échéance.
	dek, err := s.sessionDEKPeek()
	if err != nil {
		t.Fatalf("sessionDEKPeek : %v", err)
	}
	zero(dek)
	s.mu.Lock()
	after := s.sessUntil
	s.mu.Unlock()
	if !after.Equal(fixed) {
		t.Fatalf("sessionDEKPeek a glissé la session : %v -> %v (ne doit pas bouger)", fixed, after)
	}

	// USAGE normal : DOIT repousser l'échéance (now+ttl, bien au-delà de fixed).
	dek2, err := s.sessionDEK()
	if err != nil {
		t.Fatalf("sessionDEK : %v", err)
	}
	zero(dek2)
	s.mu.Lock()
	slid := s.sessUntil
	s.mu.Unlock()
	if !slid.After(fixed) {
		t.Fatalf("sessionDEK n'a pas glissé la session : %v (attendu > %v)", slid, fixed)
	}

	// Session EXPIRÉE : peek renvoie ErrLocked ET zéroise la DEK (comme sessionDEK).
	s.mu.Lock()
	s.sessUntil = time.Now().Add(-time.Second)
	s.mu.Unlock()
	if _, err := s.sessionDEKPeek(); !errors.Is(err, ErrLocked) {
		t.Fatalf("peek sur session expirée : attendu ErrLocked, obtenu %v", err)
	}
	if s.Unlocked() {
		t.Fatal("peek sur session expirée doit zéroiser la DEK (session verrouillée)")
	}

	// Session verrouillée : peek renvoie ErrLocked sans paniquer.
	if _, err := s.sessionDEKPeek(); !errors.Is(err, ErrLocked) {
		t.Fatalf("peek sur session verrouillée : attendu ErrLocked, obtenu %v", err)
	}
}
