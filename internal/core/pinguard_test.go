// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"testing"
	"time"
)

// TestPINGuardRoundTrip : écriture puis relecture de l'état du verrou.
func TestPINGuardRoundTrip(t *testing.T) {
	newInTemp(t) // fixe config.Dir() dans un dossier temporaire isolé
	g := pinGuardState{Fails: 3, LockUntil: time.Now().Add(time.Hour).Unix()}
	if err := writePINGuard(g); err != nil {
		t.Fatalf("writePINGuard : %v", err)
	}
	back, err := readPINGuard()
	if err != nil {
		t.Fatalf("readPINGuard : %v", err)
	}
	if back.Fails != g.Fails || back.LockUntil != g.LockUntil {
		t.Fatalf("round-trip : %+v vs %+v", back, g)
	}
}

// TestLoadPINGuardRestoresLockout : un verrou persisté dans le futur est restauré par un NOUVEAU
// service (survit au redémarrage de l'app).
func TestLoadPINGuardRestoresLockout(t *testing.T) {
	newInTemp(t)
	if err := writePINGuard(pinGuardState{Fails: 0, LockUntil: time.Now().Add(time.Hour).Unix()}); err != nil {
		t.Fatal(err)
	}
	s2, err := New("test")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s2.verifyPINGated("123456"); err == nil {
		t.Error("le verrou persisté aurait dû être restauré (verifyPINGated devrait refuser)")
	}
}

// TestLoadPINGuardFailSafe : sans fichier, le service repart avec un verrou vierge (jamais de blocage).
func TestLoadPINGuardFailSafe(t *testing.T) {
	newInTemp(t) // aucun pinguard.json dans le dossier temporaire
	s, err := New("test")
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	fails, lock := s.pinFails, s.pinLockTo
	s.mu.Unlock()
	if fails != 0 || lock != 0 {
		t.Errorf("sans fichier, le verrou doit être vierge : fails=%d lock=%d", fails, lock)
	}
}

// TestSavePINGuardReflectsReset : après resetPINCounters, le disque reflète fails=0.
func TestSavePINGuardReflectsReset(t *testing.T) {
	s := newInTemp(t)
	if err := writePINGuard(pinGuardState{Fails: 4, LockUntil: 0}); err != nil {
		t.Fatal(err)
	}
	s.resetPINCounters()
	g, err := readPINGuard()
	if err != nil {
		t.Fatalf("readPINGuard : %v", err)
	}
	if g.Fails != 0 || g.LockUntil != 0 {
		t.Errorf("après reset, le disque doit refléter 0 : %+v", g)
	}
}
