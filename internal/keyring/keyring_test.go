// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package keyring

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
)

// isole le trousseau dans un dossier temporaire (config.Dir lit APPDATA / XDG_CONFIG_HOME).
func isolate(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
}

const pin = "secret123"

func TestInitUnlockRoundtrip(t *testing.T) {
	isolate(t)
	f, dek, err := Init(pin)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if len(dek) != keyLen {
		t.Fatalf("DEK len = %d, want %d", len(dek), keyLen)
	}
	if !Initialized() {
		t.Fatal("Initialized() = false après Init")
	}
	// Relecture depuis le disque puis déverrouillage.
	g, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	dek2, err := g.Unlock(pin)
	if err != nil {
		t.Fatalf("Unlock: %v", err)
	}
	if !bytes.Equal(dek, dek2) {
		t.Fatal("DEK déballée ≠ DEK d'origine")
	}
	_ = f
}

func TestUnlockWrongPIN(t *testing.T) {
	isolate(t)
	if _, _, err := Init(pin); err != nil {
		t.Fatalf("Init: %v", err)
	}
	f, _ := Load()
	if _, err := f.Unlock("mauvais"); !errors.Is(err, ErrBadPIN) {
		t.Fatalf("Unlock(mauvais) = %v, want ErrBadPIN", err)
	}
}

func TestSetGetToken(t *testing.T) {
	isolate(t)
	_, dek, err := Init(pin)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	f, _ := Load()
	const tok = "f00ba7-token-apcom"
	if err := f.SetToken(dek, "apcom", "https://git.apcom.app", []string{"write:repository"}, tok); err != nil {
		t.Fatalf("SetToken: %v", err)
	}
	// Relecture complète depuis le disque + nouveau déverrouillage.
	g, _ := Load()
	dek2, _ := g.Unlock(pin)
	if !g.Has("apcom") {
		t.Fatal("Has(apcom) = false")
	}
	got, err := g.GetToken(dek2, "apcom")
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}
	if got != tok {
		t.Fatalf("GetToken = %q, want %q", got, tok)
	}
	if _, err := g.GetToken(dek2, "inconnu"); !errors.Is(err, ErrNoEntry) {
		t.Fatalf("GetToken(inconnu) = %v, want ErrNoEntry", err)
	}
}

func TestScopes(t *testing.T) {
	isolate(t)
	_, dek, err := Init(pin)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	f, _ := Load()
	_ = f.SetToken(dek, "apcom", "https://git.apcom.app", []string{"write:repository", "write:admin"}, "tok")
	_ = f.SetToken(dek, "perso", "https://git.apcom.app", []string{"write:repository"}, "tok2")
	// Relecture disque : les scopes sont stockés en clair, lisibles sans la DEK.
	g, _ := Load()
	hasAdmin := func(id string) bool {
		for _, s := range g.Scopes(id) {
			if s == "write:admin" {
				return true
			}
		}
		return false
	}
	if !hasAdmin("apcom") {
		t.Error("apcom devrait avoir write:admin")
	}
	if hasAdmin("perso") {
		t.Error("perso ne devrait pas avoir write:admin")
	}
	if g.Scopes("inconnu") != nil {
		t.Error("une instance inconnue ne devrait renvoyer aucun scope")
	}
}

func TestDeleteToken(t *testing.T) {
	isolate(t)
	_, dek, _ := Init(pin)
	f, _ := Load()
	_ = f.SetToken(dek, "apcom", "https://git.apcom.app", nil, "tok")
	if err := f.DeleteToken("apcom"); err != nil {
		t.Fatalf("DeleteToken: %v", err)
	}
	g, _ := Load()
	if g.Has("apcom") {
		t.Fatal("token toujours présent après suppression")
	}
	if err := g.DeleteToken("apcom"); err != nil {
		t.Fatalf("DeleteToken idempotent attendu, got %v", err)
	}
}

// TestRewrapChangesPIN valide le vrai chemin de changement de PIN (celui du noyau) :
// Unlock(ancien) → Rewrap(nouveau). L'ancien PIN ne déverrouille plus, le nouveau oui, et les
// tokens restent lisibles (la DEK ne change pas).
func TestRewrapChangesPIN(t *testing.T) {
	isolate(t)
	_, dek, _ := Init(pin)
	f, _ := Load()
	_ = f.SetToken(dek, "apcom", "https://git.apcom.app", nil, "tok-xyz")

	g, _ := Load()
	const newPIN = "nouveau456"
	gdek, err := g.Unlock(pin)
	if err != nil {
		t.Fatalf("Unlock(ancien): %v", err)
	}
	if err := g.Rewrap(gdek, newPIN); err != nil {
		t.Fatalf("Rewrap: %v", err)
	}
	// L'ancien PIN ne déverrouille plus.
	h, _ := Load()
	if _, err := h.Unlock(pin); !errors.Is(err, ErrBadPIN) {
		t.Fatalf("ancien PIN accepté après changement: %v", err)
	}
	// Le nouveau PIN déverrouille et le token reste lisible (DEK inchangée).
	dek2, err := h.Unlock(newPIN)
	if err != nil {
		t.Fatalf("Unlock(newPIN): %v", err)
	}
	got, err := h.GetToken(dek2, "apcom")
	if err != nil || got != "tok-xyz" {
		t.Fatalf("token après ChangePIN = %q, %v", got, err)
	}
}

func TestLoadNotInitialized(t *testing.T) {
	isolate(t)
	if _, err := Load(); !errors.Is(err, ErrNotInitialized) {
		t.Fatalf("Load() = %v, want ErrNotInitialized", err)
	}
	if Initialized() {
		t.Fatal("Initialized() = true sur dossier vide")
	}
}

func TestInitAlreadyExists(t *testing.T) {
	isolate(t)
	if _, _, err := Init(pin); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, _, err := Init(pin); !errors.Is(err, ErrAlreadyInit) {
		t.Fatalf("2e Init = %v, want ErrAlreadyInit", err)
	}
}

func TestPasswordMemorized(t *testing.T) {
	isolate(t)
	_, dek, _ := Init(pin)
	f, _ := Load()
	_ = f.SetToken(dek, "apcom", "https://git.apcom.app", nil, "TOK")
	// Aucun mot de passe au départ.
	if p, err := f.GetPassword(dek, "apcom"); err != nil || p != "" {
		t.Fatalf("mot de passe initial = %q, %v (attendu vide)", p, err)
	}
	if err := f.SetPassword(dek, "apcom", "s3cr3t"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	// Relecture disque + déverrouillage.
	g, _ := Load()
	dek2, _ := g.Unlock(pin)
	if p, err := g.GetPassword(dek2, "apcom"); err != nil || p != "s3cr3t" {
		t.Fatalf("mot de passe mémorisé = %q, %v", p, err)
	}
	// Le token reste lisible et distinct.
	if tok, _ := g.GetToken(dek2, "apcom"); tok != "TOK" {
		t.Fatalf("token = %q", tok)
	}
	// Effacement.
	_ = g.SetPassword(dek2, "apcom", "")
	h, _ := Load()
	if p, _ := h.GetPassword(dek2, "apcom"); p != "" {
		t.Fatalf("mot de passe non effacé: %q", p)
	}
}

func TestLoadRejectsBadKDF(t *testing.T) {
	isolate(t)
	if _, _, err := Init(pin); err != nil {
		t.Fatalf("Init: %v", err)
	}
	// Paramètre de parallélisme à 0 : argon2.IDKey PANIQUERAIT au déverrouillage. Load doit
	// refuser proprement en ErrFormat (anti-DoS du démon).
	raw, _ := os.ReadFile(Path())
	bad := strings.Replace(string(raw), "\"p\": 4", "\"p\": 0", 1)
	if bad == string(raw) {
		t.Fatal("remplacement du paramètre p introuvable")
	}
	if err := os.WriteFile(Path(), []byte(bad), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := Load(); !errors.Is(err, ErrFormat) {
		t.Fatalf("Load avec p=0 = %v, want ErrFormat", err)
	}
}

func TestLoadRejectsFutureVersion(t *testing.T) {
	isolate(t)
	if _, _, err := Init(pin); err != nil {
		t.Fatalf("Init: %v", err)
	}
	raw, _ := os.ReadFile(Path())
	bad := strings.Replace(string(raw), "\"version\": 1", "\"version\": 2", 1)
	if bad == string(raw) {
		t.Fatal("remplacement de version introuvable")
	}
	_ = os.WriteFile(Path(), []byte(bad), 0o600)
	if _, err := Load(); !errors.Is(err, ErrFormat) {
		t.Fatalf("Load version 2 = %v, want ErrFormat (pas de réécriture silencieuse en v1)", err)
	}
}

func TestTamperedEntryDetected(t *testing.T) {
	isolate(t)
	_, dek, _ := Init(pin)
	f, _ := Load()
	_ = f.SetToken(dek, "apcom", "https://git.apcom.app", nil, "tok")
	// On altère un octet du chiffré : le tag GCM doit faire échouer le déchiffrement.
	f.Entries[0].Enc.CT[0] ^= 0xFF
	if _, err := f.GetToken(dek, "apcom"); !errors.Is(err, ErrFormat) {
		t.Fatalf("altération non détectée: %v", err)
	}
}
