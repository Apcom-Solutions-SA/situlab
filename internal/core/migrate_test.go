// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"testing"

	gokeyring "github.com/zalando/go-keyring"

	"situlab/internal/config"
	"situlab/internal/vault"
)

// TestVaultMigration : un poste v1.0 (instances + tokens dans le coffre OS) qui définit son
// PIN en v1.1.0 voit ses tokens importés dans le trousseau chiffré et purgés du coffre OS.
func TestVaultMigration(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AppData", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	gokeyring.MockInit() // coffre OS en mémoire

	// Simuler l'existant v1.0 : une instance en config, son token au coffre OS.
	cfg := &config.Config{Instances: []config.Instance{{Name: "apcom", URL: "https://git.apcom.app", Default: true}}}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save config : %v", err)
	}
	if err := vault.SetToken("apcom", "TOKEN-OLD"); err != nil {
		t.Fatalf("SetToken coffre : %v", err)
	}

	s, err := New("test")
	if err != nil {
		t.Fatalf("New : %v", err)
	}
	// Définir le PIN ouvre la session ET déclenche la migration.
	if err := s.SetPIN("", "123456"); err != nil {
		t.Fatalf("SetPIN : %v", err)
	}

	// Le token est désormais lisible via le trousseau chiffré.
	if got, err := s.tokenFor("apcom"); err != nil || got != "TOKEN-OLD" {
		t.Fatalf("tokenFor après migration = %q, %v", got, err)
	}
	// Et retiré du coffre OS.
	if t2, _ := vault.GetToken("apcom"); t2 != "" {
		t.Fatalf("coffre OS non purgé : %q", t2)
	}
	// doctor ne signale plus de résidu.
	if r := s.Doctor().VaultResidual; r != 0 {
		t.Fatalf("VaultResidual = %d, attendu 0", r)
	}
}
