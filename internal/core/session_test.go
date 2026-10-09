// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"errors"
	"testing"

	"situlab/internal/keyring"
)

func newInTemp(t *testing.T) *Service {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("AppData", dir)         // Windows (os.UserConfigDir)
	t.Setenv("XDG_CONFIG_HOME", dir) // Linux/macOS
	s, err := New("test")
	if err != nil {
		t.Fatalf("New : %v", err)
	}
	return s
}

// TestSessionLifecycle : définir le PIN ouvre la session ; Lock la ferme ; un token rangé
// n'est lisible que session déverrouillée, et jamais après Lock.
func TestSessionLifecycle(t *testing.T) {
	s := newInTemp(t)

	if s.Unlocked() {
		t.Fatal("session déverrouillée avant tout PIN")
	}
	if err := s.SetPIN("", "123456"); err != nil {
		t.Fatalf("SetPIN initial : %v", err)
	}
	if !s.Unlocked() {
		t.Fatal("définir le PIN aurait dû ouvrir la session")
	}

	// On range un token via la DEK de session (simule ce que fait AddInstance).
	dek, err := s.sessionDEK()
	if err != nil {
		t.Fatalf("sessionDEK : %v", err)
	}
	kr, err := keyring.Load()
	if err != nil {
		t.Fatalf("keyring.Load : %v", err)
	}
	if err := kr.SetToken(dek, "apcom", "https://git.apcom.app", nil, "TOK-123"); err != nil {
		t.Fatalf("SetToken : %v", err)
	}

	// Lecture via le service, session ouverte.
	if got, err := s.tokenFor("apcom"); err != nil || got != "TOK-123" {
		t.Fatalf("tokenFor (déverrouillé) = %q, %v", got, err)
	}

	// Après Lock : plus aucun accès au token.
	s.Lock()
	if s.Unlocked() {
		t.Fatal("session encore ouverte après Lock")
	}
	if _, err := s.tokenFor("apcom"); !errors.Is(err, ErrLocked) {
		t.Fatalf("tokenFor (verrouillé) = %v, want ErrLocked", err)
	}

	// Re-déverrouillage par Unlock : le token redevient lisible (DEK inchangée).
	if err := s.Unlock("123456"); err != nil {
		t.Fatalf("Unlock : %v", err)
	}
	if got, err := s.tokenFor("apcom"); err != nil || got != "TOK-123" {
		t.Fatalf("tokenFor après re-Unlock = %q, %v", got, err)
	}
	// Mauvais PIN au déverrouillage.
	s.Lock()
	if err := s.Unlock("000000"); err == nil {
		t.Fatal("Unlock avec mauvais PIN aurait dû échouer")
	}
}
