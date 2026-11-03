// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"errors"
	"testing"
	"time"

	"situlab/internal/config"
)

// TestPolicyNoLaunderingViaSetter — P3 (revue adversariale 1.16) : une falsification de config.json ne
// peut pas être « blanchie » par un setter de politique sans rapport. Tant qu'un ré-ancrage EXPLICITE
// n'a pas eu lieu, les setters non gardés refusent (ErrPolicyTampered) : la signature n'est jamais
// re-posée par-dessus le poison.
func TestPolicyNoLaunderingViaSetter(t *testing.T) {
	s := newInTemp(t)
	if s.devKey == nil {
		t.Skip("clé d'appareil indisponible")
	}
	if err := s.SetPIN("", "123456"); err != nil {
		t.Fatal(err)
	}
	cfg, _ := config.Load()
	cfg.Instances = []config.Instance{{Name: "atl", URL: "https://a", Role: config.RoleAtelier}}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	s.retrustPolicy()
	if err := s.Unlock("123456"); err != nil {
		t.Fatal(err)
	}
	// FALSIFICATION hors SituLab : abaisser l'admin (RepoAdmin=false) → signature invalide.
	bad, _ := config.Load()
	bad.Instances[0].PINPolicy = &config.PINPolicy{RepoAdmin: boolPtr(false)}
	if err := bad.Save(); err != nil {
		t.Fatal(err)
	}
	if !s.RequiresPIN("atl", config.ActionRepoAdmin) {
		t.Fatal("falsification : admin devrait exiger le PIN (fail-closed)")
	}
	// Un setter SANS RAPPORT ne doit PAS blanchir la falsification : il refuse.
	if err := s.SetReadOnly(true); !errors.Is(err, ErrPolicyTampered) {
		t.Errorf("SetReadOnly sur policy falsifiée : attendu ErrPolicyTampered, obtenu %v", err)
	}
	if !s.RequiresPIN("atl", config.ActionRepoAdmin) {
		t.Error("après tentative de setter : la falsification ne doit pas avoir été blanchie")
	}
	// RÉ-ANCRAGE EXPLICITE (récupération délibérée) → la politique courante redevient de confiance.
	s.ReanchorPolicy()
	if s.RequiresPIN("atl", config.ActionPush) {
		t.Error("après ré-ancrage : atelier push ne devrait plus exiger le PIN")
	}
}

func boolPtr(b bool) *bool { return &b }

// TestDefaultLockTTL — P5 : la fenêtre d'exposition par défaut est réduite à 5 min (était 15 min).
func TestDefaultLockTTL(t *testing.T) {
	if defaultLockTTL != 5*time.Minute {
		t.Fatalf("defaultLockTTL = %v, attendu 5m (durcissement 1.16)", defaultLockTTL)
	}
	s := newInTemp(t)
	if got := s.lockTTL(); got != defaultLockTTL {
		t.Fatalf("lockTTL par défaut = %v, attendu %v", got, defaultLockTTL)
	}
}

// TestPolicyTamperFailsClosed — P3 : une réécriture directe de config.json (hors des setters window-only)
// invalide la signature d'intégrité de la politique → le cœur retombe en POSITION FERMÉE (PIN exigé,
// lecture seule), au lieu d'appliquer la politique falsifiée. Un ré-ancrage légitime rétablit la confiance.
func TestPolicyTamperFailsClosed(t *testing.T) {
	s := newInTemp(t)
	if s.devKey == nil {
		t.Skip("clé d'appareil indisponible sur ce poste")
	}
	if err := s.SetPIN("", "123456"); err != nil {
		t.Fatalf("SetPIN : %v", err)
	}
	cfg, _ := config.Load()
	cfg.Instances = []config.Instance{{Name: "atl", URL: "https://a", Role: config.RoleAtelier}}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	s.retrustPolicy() // politique légitime signée
	if err := s.Unlock("123456"); err != nil {
		t.Fatalf("Unlock : %v", err)
	}

	// Référence : atelier push sans PIN autorisé, pas en lecture seule.
	if s.RequiresPIN("atl", config.ActionPush) {
		t.Fatal("réf : atelier push ne devrait pas exiger le PIN")
	}
	if s.ReadOnly() {
		t.Fatal("réf : pas de lecture seule")
	}

	// FALSIFICATION hors SituLab : on force push sous PIN à false explicitement ET on élargit le set,
	// sans re-signer → la signature devient incohérente.
	bad, _ := config.Load()
	bad.Instances[0].PINPolicy = &config.PINPolicy{RepoAdmin: boolPtr(false)} // tente d'abaisser l'admin
	bad.Instances = append(bad.Instances, config.Instance{Name: "evil", URL: "https://evil", Role: config.RoleAtelier})
	if err := bad.Save(); err != nil {
		t.Fatal(err)
	}

	// Fail-closed : la policy falsifiée → PIN exigé + lecture seule.
	if !s.RequiresPIN("atl", config.ActionPush) {
		t.Error("policy falsifiée : push atelier devrait EXIGER le PIN (fail-closed, fermeture P3)")
	}
	if !s.RequiresPIN("atl", config.ActionRepoAdmin) {
		t.Error("policy falsifiée : l'abaissement de l'admin ne doit pas prendre effet (fail-closed)")
	}
	if !s.ReadOnly() {
		t.Error("policy falsifiée : le cœur devrait retomber en lecture seule (fail-closed)")
	}

	// RÉPARATION légitime (ré-ancrage) → redevient de confiance.
	s.retrustPolicy()
	if s.RequiresPIN("atl", config.ActionPush) {
		t.Error("après ré-ancrage : atelier push ne devrait plus exiger le PIN")
	}
	if s.ReadOnly() {
		t.Error("après ré-ancrage : plus de lecture seule")
	}
}

// TestPolicyLegitChangeStaysTrusted — P3 : un changement de politique via un SETTER (window-only)
// re-signe automatiquement → la politique reste de confiance (pas de faux positif de falsification).
func TestPolicyLegitChangeStaysTrusted(t *testing.T) {
	s := newInTemp(t)
	if s.devKey == nil {
		t.Skip("clé d'appareil indisponible")
	}
	if err := s.SetPIN("", "123456"); err != nil {
		t.Fatal(err)
	}
	cfg, _ := config.Load()
	cfg.Instances = []config.Instance{{Name: "atl", URL: "https://a", Role: config.RoleAtelier}}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	s.retrustPolicy() // base signée
	if err := s.Unlock("123456"); err != nil {
		t.Fatal(err)
	}
	if s.ReadOnly() {
		t.Fatal("base : pas de lecture seule")
	}
	// Changement de rôle via SETTER (re-signe) → toujours de confiance, et la politique prend effet.
	if err := s.SetInstanceRole("atl", config.RoleProduction); err != nil {
		t.Fatalf("SetInstanceRole : %v", err)
	}
	if s.ReadOnly() {
		t.Error("changement de rôle via setter : la policy doit rester de confiance")
	}
	if !s.RequiresPIN("atl", config.ActionPush) {
		t.Error("production : push devrait exiger le PIN (politique appliquée, signature valide)")
	}
	// Bascule lecture seule via SETTER (re-signe) → cohérent dans les deux sens.
	if err := s.SetReadOnly(true); err != nil {
		t.Fatalf("SetReadOnly(true) : %v", err)
	}
	if !s.ReadOnly() {
		t.Error("SetReadOnly(true) : la lecture seule devrait être active (signée)")
	}
	if err := s.SetReadOnly(false); err != nil {
		t.Fatalf("SetReadOnly(false) : %v", err)
	}
	if s.ReadOnly() {
		t.Error("SetReadOnly(false) : la lecture seule devrait être désactivée (signée)")
	}
}
