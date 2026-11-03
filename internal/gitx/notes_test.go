// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package gitx

import "testing"

// La note d'attestation « SituLab approved » se pose sur un commit et se relit à l'identique ; une
// seconde écriture REMPLACE (add -f). C'est le support de la signature de garantie ; sa PUBLICATION
// en ligne réutilise le chemin Push durci (runC/authCfg), non testable ici sur remote local
// (protocol.file.allow=never).
func TestNotesRoundtrip(t *testing.T) {
	dir := newRepo(t)
	sha := HeadSHA(dir)
	if sha == "" {
		t.Fatal("HEAD introuvable")
	}

	// Pas de note au départ.
	if got := ReadNote(dir, "situlab", sha); got != "" {
		t.Fatalf("aucune note attendue, obtenu %q", got)
	}

	// Écriture puis relecture à l'identique.
	payload := `{"commit":"` + sha + `","sig":"test"}`
	if err := WriteNote(dir, "situlab", sha, payload); err != nil {
		t.Fatalf("WriteNote : %v", err)
	}
	if got := ReadNote(dir, "situlab", sha); got != payload {
		t.Fatalf("note attendue %q, obtenu %q", payload, got)
	}

	// Remplacement (add -f) : la nouvelle valeur écrase l'ancienne, pas d'empilement.
	payload2 := `{"commit":"` + sha + `","sig":"test2"}`
	if err := WriteNote(dir, "situlab", sha, payload2); err != nil {
		t.Fatalf("WriteNote (remplacement) : %v", err)
	}
	if got := ReadNote(dir, "situlab", sha); got != payload2 {
		t.Fatalf("note remplacée attendue %q, obtenu %q", payload2, got)
	}

	// SHA invalide rejeté (garde-fou safeRevision).
	if err := WriteNote(dir, "situlab", "bad;rev", "x"); err == nil {
		t.Error("un SHA invalide doit être refusé")
	}
}
