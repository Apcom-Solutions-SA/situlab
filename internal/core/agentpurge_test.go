// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"os"
	"path/filepath"
	"testing"
)

// TestPurgeDirKeeping : la remise à blanc d'un agent (retour #14) GARDE exactement les entrées préservées
// (login + fonctionnement) et efface TOUT le reste, y compris les sous-dossiers volumineux (sessions,
// caches). On exerce la mécanique sur un dossier temporaire (jamais le vrai ~/.claude).
func TestPurgeDirKeeping(t *testing.T) {
	dir := t.TempDir()
	// À GARDER
	mustWrite(t, filepath.Join(dir, ".credentials.json"), "login")
	mustWrite(t, filepath.Join(dir, "settings.json"), "cfg")
	if err := os.MkdirAll(filepath.Join(dir, "plugins", "x"), 0o700); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, "plugins", "x", "p.js"), "keep")
	// À EFFACER
	if err := os.MkdirAll(filepath.Join(dir, "projects", "deep"), 0o700); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, "projects", "deep", "session.json"), "gone")
	mustWrite(t, filepath.Join(dir, "history.jsonl"), "gone")
	if err := os.MkdirAll(filepath.Join(dir, "shell-snapshots"), 0o700); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, "shell-snapshots", "snap"), "gone")

	keep := map[string]bool{".credentials.json": true, "settings.json": true, "plugins": true}
	skipped, err := purgeDirKeeping(dir, keep, nil)
	if err != nil {
		t.Fatalf("purgeDirKeeping : %v", err)
	}
	if len(skipped) != 0 {
		t.Errorf("aucune entrée verrouillée attendue sur ce cas, obtenu skipped=%v", skipped)
	}

	for _, p := range []string{".credentials.json", "settings.json", filepath.Join("plugins", "x", "p.js")} {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			t.Errorf("devait être gardé : %s (%v)", p, err)
		}
	}
	for _, p := range []string{"projects", "history.jsonl", "shell-snapshots"} {
		if _, err := os.Stat(filepath.Join(dir, p)); !os.IsNotExist(err) {
			t.Errorf("devait être effacé : %s (obtenu %v)", p, err)
		}
	}
}

// TestPurgeAgentGuards : confirmation textuelle exigée + agent inconnu refusé (fail-safe côté cœur).
func TestPurgeAgentGuards(t *testing.T) {
	s := newInTemp(t)
	if _, err := s.PurgeAgent("claude-code", "oui"); err == nil {
		t.Error("sans la confirmation EFFACER, PurgeAgent devrait refuser")
	}
	if _, err := s.PurgeAgent("agent-fantome", "EFFACER"); err == nil {
		t.Error("un agent inconnu devrait être refusé")
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
