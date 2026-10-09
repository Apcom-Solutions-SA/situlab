// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"errors"
	"testing"
)

// TestSetPINRateLimited vérifie que SetPIN (changement de PIN) passe par le même
// verrou anti-bruteforce que Push : 5 anciens PIN faux verrouillent le 6e essai.
// (Régression de la revue v0.3 : SetPIN était un oracle de bruteforce non limité.)
func TestSetPINRateLimited(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AppData", dir)         // Windows (os.UserConfigDir)
	t.Setenv("XDG_CONFIG_HOME", dir) // Linux/macOS

	s, err := New("test")
	if err != nil {
		t.Fatalf("New : %v", err)
	}
	if err := s.SetPIN("", "123456"); err != nil {
		t.Fatalf("définition initiale du PIN : %v", err)
	}

	for i := 0; i < pinMaxFails; i++ {
		if err := s.SetPIN("000000", "654321"); err == nil {
			t.Fatalf("l'essai %d avec un mauvais ancien PIN aurait dû échouer", i+1)
		}
	}
	// Le crédit d'essais est épuisé : l'appel suivant doit être verrouillé.
	if err := s.SetPIN("000000", "654321"); !errors.Is(err, ErrPINLocked) {
		t.Fatalf("après %d échecs, attendu ErrPINLocked, obtenu : %v", pinMaxFails, err)
	}
	// Le verrou partagé bloque aussi Push (même si le PIN fourni est correct).
	if _, err := s.Push("", dir, "123456"); !errors.Is(err, ErrPINLocked) && !errors.Is(err, ErrNoGit) {
		t.Fatalf("Push pendant le verrou : attendu ErrPINLocked, obtenu : %v", err)
	}
}
