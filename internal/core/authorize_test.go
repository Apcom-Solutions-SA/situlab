// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"errors"
	"os"
	"os/exec"
	"testing"

	"situlab/internal/config"
	"situlab/internal/gitx"
)

// TestAuthorizePerConnection : la politique PIN par connexion (v1.7). Atelier déverrouillé =
// push/issues autorisés SANS PIN, administration SOUS PIN ; production = tout sous PIN.
func TestAuthorizePerConnection(t *testing.T) {
	s := newInTemp(t)
	if err := s.SetPIN("", "123456"); err != nil {
		t.Fatalf("SetPIN : %v", err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Instances = []config.Instance{
		{Name: "atl", URL: "https://a", Role: config.RoleAtelier},
		{Name: "prod", URL: "https://p", Role: config.RoleProduction},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	if err := s.Unlock("123456"); err != nil {
		t.Fatalf("Unlock : %v", err)
	}

	// Atelier, session ouverte : push & issues sans PIN -> autorisé.
	if err := s.authorize("atl", config.ActionPush, ""); err != nil {
		t.Errorf("atelier push sans PIN : attendu autorisé, obtenu %v", err)
	}
	if err := s.authorize("atl", config.ActionIssues, ""); err != nil {
		t.Errorf("atelier issues sans PIN : attendu autorisé, obtenu %v", err)
	}
	// Atelier : administration sans PIN -> refusé.
	if err := s.authorize("atl", config.ActionRepoAdmin, ""); !errors.Is(err, ErrPINNeeded) {
		t.Errorf("atelier admin sans PIN : attendu ErrPINNeeded, obtenu %v", err)
	}
	// Production : push sans PIN -> refusé, avec bon PIN -> autorisé.
	if err := s.authorize("prod", config.ActionPush, ""); !errors.Is(err, ErrPINNeeded) {
		t.Errorf("prod push sans PIN : attendu ErrPINNeeded, obtenu %v", err)
	}
	if err := s.authorize("prod", config.ActionPush, "123456"); err != nil {
		t.Errorf("prod push avec PIN correct : attendu autorisé, obtenu %v", err)
	}

	// Politique explicite : atelier mais push sous PIN.
	cfg2, _ := config.Load()
	cfg2.Find("atl").PINPolicy = &config.PINPolicy{Push: func() *bool { b := true; return &b }()}
	if err := cfg2.Save(); err != nil {
		t.Fatal(err)
	}
	if err := s.authorize("atl", config.ActionPush, ""); !errors.Is(err, ErrPINNeeded) {
		t.Errorf("atelier push forcé sous PIN : attendu ErrPINNeeded, obtenu %v", err)
	}

	// Jetons : création sous politique PIN (admin) — refusée sans PIN.
	if _, err := s.TokenCreate("atl", "u", "p", "", "n", nil, nil, ""); !errors.Is(err, ErrPINNeeded) {
		t.Errorf("TokenCreate sans PIN : attendu ErrPINNeeded, obtenu %v", err)
	}
}

// TestRequiresPINForPush : la politique d'un ENVOI suit le DÉPÔT réellement poussé (son remote),
// pas la connexion nommée. Un clone dont l'origin est un serveur de PRODUCTION exige le PIN même si
// l'appelant nomme une connexion atelier (pas de push prod via une connexion atelier).
func TestRequiresPINForPush(t *testing.T) {
	if !gitx.Available() {
		t.Skip("git indisponible")
	}
	s := newInTemp(t)
	if err := s.SetPIN("", "123456"); err != nil {
		t.Fatal(err)
	}
	cfg, _ := config.Load()
	cfg.Instances = []config.Instance{
		{Name: "atl", URL: "https://atelier.example", Role: config.RoleAtelier},
		{Name: "prod", URL: "https://prod.example", Role: config.RoleProduction},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	// Dépôt local dont l'origin pointe vers l'hôte de PRODUCTION.
	dir := t.TempDir()
	for _, args := range [][]string{{"init"}, {"remote", "add", "origin", "https://prod.example/o/r.git"}} {
		c := exec.Command("git", args...)
		c.Dir = dir
		c.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v : %v\n%s", args, err, out)
		}
	}
	// Même en nommant la connexion atelier, le remote prod impose le PIN.
	if !s.RequiresPINForPush("atl", dir) {
		t.Error("push vers un clone dont l'origin est en PRODUCTION devrait exiger le PIN, même via la connexion atelier")
	}
	// Un dépôt sans remote connu → fail-closed (PIN requis).
	if !s.RequiresPINForPush("atl", t.TempDir()) {
		t.Error("remote inconnu devrait être fail-closed (PIN requis)")
	}
}

// TestAuthorizeFailClosedWhenLocked : sur un chemin SANS PIN (atelier autonome), une session fermée
// n'autorise pas l'action pour autant — elle renvoie ErrLocked (et non ErrPINNeeded). Le démon
// aurait dû déverrouiller ; sans session, on refuse.
func TestAuthorizeFailClosedWhenLocked(t *testing.T) {
	s := newInTemp(t)
	if err := s.SetPIN("", "123456"); err != nil {
		t.Fatalf("SetPIN : %v", err)
	}
	cfg, _ := config.Load()
	cfg.Instances = []config.Instance{{Name: "atl", URL: "https://a", Role: config.RoleAtelier}}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	// SetPIN a ouvert la session : on la referme pour exercer le chemin verrouillé.
	s.Lock()

	// Push atelier sans PIN, session fermée → ErrLocked (pas ErrPINNeeded).
	if err := s.authorize("atl", config.ActionPush, ""); !errors.Is(err, ErrLocked) {
		t.Errorf("session fermée : attendu ErrLocked, obtenu %v", err)
	}
	// Session rouverte → autorisé sans PIN.
	if err := s.Unlock("123456"); err != nil {
		t.Fatalf("Unlock : %v", err)
	}
	if err := s.authorize("atl", config.ActionPush, ""); err != nil {
		t.Errorf("session ouverte : push atelier sans PIN devrait être autorisé, obtenu %v", err)
	}
}

// TestInstanceForHostShared : quand l'origin d'un dépôt correspond à un hôte servi par PLUSIEURS
// connexions, on ne peut pas trancher laquelle s'applique → fail-closed (PIN requis). Un hôte servi
// par une seule connexion lève l'ambiguïté et suit sa politique.
func TestInstanceForHostShared(t *testing.T) {
	if !gitx.Available() {
		t.Skip("git indisponible")
	}
	s := newInTemp(t)

	cfg, _ := config.Load()
	cfg.Instances = []config.Instance{
		{Name: "atl1", URL: "https://forge.example", Role: config.RoleAtelier},
		{Name: "atl2", URL: "https://forge.example", Role: config.RoleAtelier},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	// Dépôt local dont l'origin pointe vers l'hôte partagé.
	dir := t.TempDir()
	for _, args := range [][]string{{"init"}, {"remote", "add", "origin", "https://forge.example/o/r.git"}} {
		c := exec.Command("git", args...)
		c.Dir = dir
		c.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v : %v\n%s", args, err, out)
		}
	}

	// Hôte ambigu (deux connexions) → PIN requis.
	if !s.RequiresPINForPush("atl1", dir) {
		t.Error("hôte partagé par plusieurs connexions : le PIN devrait être exigé (fail-closed)")
	}

	// Une seule connexion sur cet hôte → ambiguïté levée, politique atelier (push sans PIN).
	cfg2, _ := config.Load()
	cfg2.Instances = []config.Instance{{Name: "atl1", URL: "https://forge.example", Role: config.RoleAtelier}}
	if err := cfg2.Save(); err != nil {
		t.Fatal(err)
	}
	if s.RequiresPINForPush("atl1", dir) {
		t.Error("hôte servi par une seule connexion atelier : le PIN ne devrait pas être exigé")
	}
}
