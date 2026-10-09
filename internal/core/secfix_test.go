// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"errors"
	"testing"

	"situlab/internal/config"
)

// TestPINLockSurvivesConfigWrites vérifie que le compteur anti-bruteforce (désormais EN
// MÉMOIRE) n'est plus écrasé par une écriture de config concurrente — la faille 1.1.0 où
// SetDefaultInstance/AddLocalRepo réécrivaient config.json et remettaient le compteur à zéro,
// neutralisant le verrou 5 essais / 60 s.
func TestPINLockSurvivesConfigWrites(t *testing.T) {
	s := newInTemp(t)
	if err := s.SetPIN("", "123456"); err != nil {
		t.Fatalf("SetPIN : %v", err)
	}
	// Une instance pour que SetDefaultInstance écrive réellement la config (le vecteur de clobber).
	cfg, _ := config.Load()
	cfg.Instances = []config.Instance{{Name: "x", URL: "https://x", Default: true}}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save config : %v", err)
	}

	for i := 0; i < pinMaxFails; i++ {
		if err := s.Unlock("000000"); err == nil {
			t.Fatalf("essai %d : un mauvais PIN aurait dû échouer", i+1)
		}
		if err := s.SetDefaultInstance("x"); err != nil { // réécrit config.json
			t.Fatalf("SetDefaultInstance : %v", err)
		}
	}
	// Le crédit d'essais est épuisé : malgré les écritures de config, le verrou doit tenir.
	if err := s.Unlock("000000"); !errors.Is(err, ErrPINLocked) {
		t.Fatalf("après %d essais + écritures config, attendu ErrPINLocked, obtenu %v", pinMaxFails, err)
	}
}
