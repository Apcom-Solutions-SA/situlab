// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package gitx

import "testing"

// Optimisation 1.20 : le nom du distant est déduit de l'upstream déjà connu (lu dans `status --branch`),
// SANS appel git supplémentaire. Ici le dossier n'est pas un dépôt : aucune commande git ne peut
// réussir, mais le nom doit quand même sortir de l'upstream fourni.
func TestDetectRemoteNameFromUpstream(t *testing.T) {
	name, _ := detectRemote(t.TempDir(), "origin/main")
	if name != "origin" {
		t.Fatalf("nom déduit de l'upstream attendu « origin », obtenu %q", name)
	}
	// Upstream sans « / » : pas de nom déductible, et pas de dépôt → vide.
	if n, _ := detectRemote(t.TempDir(), "bizarre"); n != "" {
		t.Fatalf("sans « / » ni dépôt, nom vide attendu, obtenu %q", n)
	}
}
