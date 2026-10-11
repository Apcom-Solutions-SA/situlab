// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package daemon

import (
	"errors"
	"testing"

	"situlab/internal/config"
	"situlab/internal/core"
)

// prompteur factice : scénarise la saisie (ou le refus) du PIN et compte les appels.
type fakePrompter struct {
	pin     string
	ok      bool
	timeout bool // simule un délai écoulé (PINTimedOut) plutôt qu'un refus
	calls   int
	reason  string
	action  string
}

func (f *fakePrompter) PromptPIN(reason string, _ any, action string) (string, PINOutcome) {
	f.calls++
	f.reason = reason
	f.action = action
	switch {
	case f.ok:
		return f.pin, PINConfirmed
	case f.timeout:
		return "", PINTimedOut
	default:
		return "", PINRefused
	}
}

func newDaemon(t *testing.T, p PINPrompter) *Daemon {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("AppData", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	svc, err := core.New("test")
	if err != nil {
		t.Fatalf("core.New : %v", err)
	}
	return New(svc, p)
}

// TestEnsureUnlockedCtxCarriesAction : quand la session est verrouillée, l'invite UNIQUE de
// déverrouillage porte le CONTEXTE de l'action (reason + clé d'action), pas un générique (Point 5).
func TestEnsureUnlockedCtxCarriesAction(t *testing.T) {
	fp := &fakePrompter{ok: true, pin: "123456"}
	d := newDaemon(t, fp)
	if err := d.svc.SetPIN("", "123456"); err != nil {
		t.Fatalf("SetPIN : %v", err)
	}
	d.svc.Lock()
	if err := d.ensureUnlockedCtx("Autoriser l'envoi de X", nil, "push"); err != nil {
		t.Fatalf("ensureUnlockedCtx : %v", err)
	}
	if fp.reason != "Autoriser l'envoi de X" || fp.action != "push" {
		t.Errorf("l'invite doit porter le contexte : reason=%q action=%q", fp.reason, fp.action)
	}
	if !d.svc.Unlocked() {
		t.Error("le même PIN doit avoir déverrouillé la session")
	}
}

func TestStatusAndPINSetFlow(t *testing.T) {
	p := &fakePrompter{}
	d := newDaemon(t, p)

	// Avant tout PIN : verrouillé, pas de PIN.
	st, err := d.dispatch(OpStatus, Args{})
	if err != nil {
		t.Fatalf("status : %v", err)
	}
	s := st.(Status)
	if s.PINSet || s.Unlocked {
		t.Fatalf("état initial inattendu : %+v", s)
	}

	// Définir le PIN ouvre la session (chemin in-process ; le socket ne pose jamais de PIN).
	if err := d.svc.SetPIN("", "123456"); err != nil {
		t.Fatalf("SetPIN : %v", err)
	}
	st, _ = d.dispatch(OpStatus, Args{})
	s = st.(Status)
	if !s.PINSet || !s.Unlocked {
		t.Fatalf("après pin.set, attendu PINSet+Unlocked : %+v", s)
	}

	// Lock referme.
	if _, err := d.dispatch(OpLock, Args{}); err != nil {
		t.Fatalf("lock : %v", err)
	}
	st, _ = d.dispatch(OpStatus, Args{})
	if st.(Status).Unlocked {
		t.Fatal("encore déverrouillé après lock")
	}
}

func TestUnlockViaModal(t *testing.T) {
	p := &fakePrompter{pin: "123456", ok: true}
	d := newDaemon(t, p)
	if err := d.svc.SetPIN("", "123456"); err != nil {
		t.Fatalf("SetPIN : %v", err)
	}
	d.dispatch(OpLock, Args{})

	// OpUnlock sans PIN explicite : le démon doit solliciter la modale.
	if _, err := d.dispatch(OpUnlock, Args{}); err != nil {
		t.Fatalf("unlock via modale : %v", err)
	}
	if p.calls != 1 {
		t.Fatalf("modale appelée %d fois, attendu 1", p.calls)
	}
	if !d.svc.Unlocked() {
		t.Fatal("session non ouverte après unlock")
	}
}

func TestEnsureUnlockedCancelled(t *testing.T) {
	p := &fakePrompter{ok: false} // l'humain annule
	d := newDaemon(t, p)
	_ = d.svc.SetPIN("", "123456")
	d.dispatch(OpLock, Args{})
	if err := d.ensureUnlocked(); !errors.Is(err, core.ErrLocked) {
		t.Fatalf("annulation : attendu ErrLocked, obtenu %v", err)
	}
}

func TestWritePIN(t *testing.T) {
	// Le PIN est TOUJOURS obtenu via la modale (jamais par le socket).
	p := &fakePrompter{pin: "999999", ok: true}
	d := newDaemon(t, p)
	_ = d.svc.SetPIN("", "123456")

	got, err := d.writePIN("Autoriser X", nil, "admin")
	if err != nil || got != "999999" {
		t.Fatalf("writePIN via modale = %q, %v", got, err)
	}
	if p.reason != "Autoriser X" {
		t.Fatalf("raison transmise = %q", p.reason)
	}
	if p.calls == 0 {
		t.Fatal("la modale aurait dû être sollicitée")
	}

	// Modale refusée : ErrPINNeeded (→ code 3).
	p.ok = false
	if _, err := d.writePIN("Autoriser Y", nil, "admin"); !errors.Is(err, core.ErrPINNeeded) {
		t.Fatalf("refus modale : attendu ErrPINNeeded, obtenu %v", err)
	}

	// Délai écoulé : ErrPINTimeout DISTINCT du refus — l'agent doit savoir « relance et valide »
	// plutôt que « refusé ».
	p.ok, p.timeout = false, true
	if _, err := d.writePIN("Autoriser Z", nil, "admin"); !errors.Is(err, core.ErrPINTimeout) {
		t.Fatalf("délai modale : attendu ErrPINTimeout, obtenu %v", err)
	}
	if errors.Is(core.ErrPINTimeout, core.ErrPINNeeded) {
		t.Fatal("ErrPINTimeout et ErrPINNeeded doivent rester distincts")
	}
}

// TestEnsureUnlockedTimeout : un déverrouillage non validé dans le délai renvoie ErrPINTimeout
// (distinct de l'annulation ErrLocked).
func TestEnsureUnlockedTimeout(t *testing.T) {
	p := &fakePrompter{timeout: true}
	d := newDaemon(t, p)
	_ = d.svc.SetPIN("", "123456")
	d.dispatch(OpLock, Args{})
	if err := d.ensureUnlocked(); !errors.Is(err, core.ErrPINTimeout) {
		t.Fatalf("délai déverrouillage : attendu ErrPINTimeout, obtenu %v", err)
	}
}

// TestGatePolicy : la politique PIN par connexion décide, au niveau du démon, s'il faut solliciter
// la modale. Atelier = session suffit (pas de modale) ; production = modale ; jetons = modale (admin).
func TestGatePolicy(t *testing.T) {
	p := &fakePrompter{pin: "123456", ok: true}
	d := newDaemon(t, p)
	if err := d.svc.SetPIN("", "123456"); err != nil { // ouvre la session
		t.Fatalf("SetPIN : %v", err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Instances = []config.Instance{
		{Name: "atl", URL: "https://a.example", Role: config.RoleAtelier},
		{Name: "prod", URL: "https://b.example", Role: config.RoleProduction},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	// Atelier + issues : session déverrouillée suffit — aucune modale.
	p.calls = 0
	if pin, err := d.gate(OpIssueCreate, Args{Instance: "atl"}, "x"); err != nil || pin != "" || p.calls != 0 {
		t.Fatalf("atelier/issues : attendu (\"\", nil, 0 appel) ; obtenu (%q, %v, %d)", pin, err, p.calls)
	}
	// Production + issues : PIN exigé — modale sollicitée.
	p.calls = 0
	if pin, err := d.gate(OpIssueCreate, Args{Instance: "prod"}, "x"); err != nil || pin != "123456" || p.calls != 1 {
		t.Fatalf("prod/issues : attendu (pin, nil, 1 appel) ; obtenu (%q, %v, %d)", pin, err, p.calls)
	}
	// Jetons : sous PIN (admin) même sur un atelier.
	p.calls = 0
	if _, err := d.gate(OpTokenMgrNew, Args{Instance: "atl"}, "x"); err != nil || p.calls != 1 {
		t.Fatalf("jeton/atelier : attendu modale sollicitée ; obtenu (%v, %d appels)", err, p.calls)
	}
}

func TestUnknownOp(t *testing.T) {
	d := newDaemon(t, &fakePrompter{})
	if _, err := d.dispatch("n.existe.pas", Args{}); err == nil {
		t.Fatal("opération inconnue aurait dû échouer")
	}
}
