// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"os"
	"testing"

	"situlab/internal/gitx"
)

// TestSplitAttestTrailer : extraction du trailer d'attestation et reconstruction du corps signé.
func TestSplitAttestTrailer(t *testing.T) {
	body, val, ok := splitAttestTrailer("sujet\n\ncorps\n\nSituLab-Approved: ABC123\n")
	if !ok || val != "ABC123" || body != "sujet\n\ncorps" {
		t.Fatalf("extraction : ok=%v val=%q body=%q", ok, val, body)
	}
	if b, v, ok := splitAttestTrailer("juste un message\n"); ok || v != "" || b != "juste un message" {
		t.Errorf("sans trailer : ok=%v v=%q b=%q", ok, v, b)
	}
	// Reconstruction = corps signé (symétrie avec gitx.appendTrailer).
	orig := "sujet\n\ncorps multi\nligne"
	b, v, ok := splitAttestTrailer(orig + "\n\nSituLab-Approved: XYZ\n")
	if !ok || v != "XYZ" || b != orig {
		t.Errorf("round-trip : ok=%v v=%q b=%q (corps attendu %q)", ok, v, b, orig)
	}
}

// TestCommitAttestationUnattested : un commit sans trailer ni note n'est pas vérifié (fail-closed),
// quelle que soit la racine embarquée.
func TestCommitAttestationUnattested(t *testing.T) {
	if !gitx.Available() {
		t.Skip("git indisponible")
	}
	s := newInTemp(t)
	dir := t.TempDir()
	gitRun(t, dir, "init")
	gitRun(t, dir, "config", "user.name", "T")
	gitRun(t, dir, "config", "user.email", "t@x")
	if err := os.WriteFile(dir+"/a.txt", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "hors situlab")
	if v := s.CommitAttestation(dir, gitx.HeadSHA(dir)); v.Verified {
		t.Error("un commit sans attestation ne doit pas être vérifié")
	}
}
