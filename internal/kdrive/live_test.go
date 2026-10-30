// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package kdrive

import (
	"context"
	"os"
	"testing"
	"time"
)

// Test de bout en bout contre l'API RÉELLE kDrive. Ne tourne QUE si KDRIVE_TOKEN et KDRIVE_ID sont
// fournis en environnement (jamais de secret dans le dépôt). Skip par défaut (CI incluse).
func TestLiveList(t *testing.T) {
	tok := os.Getenv("KDRIVE_TOKEN")
	id := os.Getenv("KDRIVE_ID")
	if tok == "" || id == "" {
		t.Skip("KDRIVE_TOKEN / KDRIVE_ID absents : test live ignoré")
	}
	c := New(id, tok, "test")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := c.TestConnection(ctx); err != nil {
		t.Fatalf("TestConnection: %v", err)
	}
	ents, err := c.List(ctx, "/")
	if err != nil {
		t.Fatalf("List(/): %v", err)
	}
	t.Logf("racine : %d entrées", len(ents))
	for _, e := range ents {
		t.Logf("  %v  %s  (%d o)  path=%s", e.IsDir, e.Name, e.Size, e.Path)
	}
	if _, err := c.ListShared(ctx); err != nil {
		t.Fatalf("ListShared: %v", err)
	}
	// Descente dans le premier dossier, pour valider la résolution chemin→ID.
	for _, e := range ents {
		if e.IsDir {
			sub, err := c.List(ctx, e.Path)
			if err != nil {
				t.Fatalf("List(%s): %v", e.Path, err)
			}
			t.Logf("sous-dossier %s : %d entrées", e.Path, len(sub))
			break
		}
	}
}
