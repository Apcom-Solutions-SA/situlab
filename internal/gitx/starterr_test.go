// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package gitx

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestStatusOfInvalidDirFriendlyError : quand git ne peut pas DÉMARRER parce que le dossier de
// travail n'existe pas, l'erreur remontée doit être une phrase claire — jamais l'interne
// « fork/exec … : The directory name is invalid » qui fuit le chemin du binaire git.
func TestStatusOfInvalidDirFriendlyError(t *testing.T) {
	if !Available() {
		t.Skip("git absent du PATH")
	}
	missing := filepath.Join(t.TempDir(), "n-existe-pas-situlab")
	_, err := StatusOf(missing)
	if err == nil {
		t.Fatalf("StatusOf sur un dossier inexistant devrait échouer")
	}
	msg := err.Error()
	if strings.Contains(msg, "fork/exec") || strings.Contains(strings.ToLower(msg), "git.exe") {
		t.Fatalf("message d'erreur fuit l'interne : %q", msg)
	}
	if !strings.Contains(msg, "introuvable ou inaccessible") {
		t.Fatalf("message d'erreur peu clair, attendu une phrase explicite : %q", msg)
	}
}
