// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"reflect"
	"testing"

	"situlab/internal/config"
	"situlab/internal/keyring"
)

// TestDisconnectedInstancesAndReset : une connexion est « déconnectée » tant qu'elle n'a pas de
// jeton dans le trousseau. Après un reset du trousseau (PIN oublié), les connexions restent en
// config mais redeviennent toutes déconnectées, PIN et session repartant à blanc.
func TestDisconnectedInstancesAndReset(t *testing.T) {
	s := newInTemp(t)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Instances = []config.Instance{
		{Name: "a", URL: "https://a"},
		{Name: "b", URL: "https://b"},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	// Avant tout PIN : trousseau absent → les deux connexions sont déconnectées (ordre de config).
	if got := s.DisconnectedInstances(); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("sans trousseau, attendu [a b], obtenu %v", got)
	}

	// Définir le PIN ouvre la session ; on range un jeton pour « a » seulement.
	if err := s.SetPIN("", "123456"); err != nil {
		t.Fatalf("SetPIN : %v", err)
	}
	dek, err := s.sessionDEK()
	if err != nil {
		t.Fatalf("sessionDEK : %v", err)
	}
	kr, err := keyring.Load()
	if err != nil {
		t.Fatalf("keyring.Load : %v", err)
	}
	if err := kr.SetToken(dek, "a", "https://a", nil, "tok"); err != nil {
		t.Fatalf("SetToken : %v", err)
	}
	if got := s.DisconnectedInstances(); !reflect.DeepEqual(got, []string{"b"}) {
		t.Fatalf("« a » connectée, attendu [b], obtenu %v", got)
	}

	// Reset du trousseau : session fermée, plus de PIN, connexions conservées mais déconnectées.
	if err := s.ResetKeyring(); err != nil {
		t.Fatalf("ResetKeyring : %v", err)
	}
	if s.PINSet() {
		t.Error("plus aucun PIN ne devrait être défini après le reset")
	}
	if s.Unlocked() {
		t.Error("la session devrait être fermée après le reset")
	}
	if got := s.DisconnectedInstances(); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("après reset, attendu [a b], obtenu %v", got)
	}
	cfg2, _ := config.Load()
	if len(cfg2.Instances) != 2 {
		t.Fatalf("les connexions doivent être conservées en config, obtenu %d", len(cfg2.Instances))
	}
}
