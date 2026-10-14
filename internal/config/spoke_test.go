// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package config

import "testing"

func TestSpokeHelpers(t *testing.T) {
	c := &Config{Spokes: []Spoke{
		{Name: "Mission A", LocalPath: "/a", Kernel: "Mémoire", Mission: "projet A"},
		{Name: "Mission B", LocalPath: "/b"},
	}}
	if c.FindSpoke("mission a") == nil {
		t.Fatal("FindSpoke devrait trouver « mission a » (insensible à la casse)")
	}
	if c.FindSpoke("zzz") != nil {
		t.Fatal("FindSpoke ne devrait rien renvoyer pour un nom inconnu")
	}
	if !c.RemoveSpoke("Mission A") || len(c.Spokes) != 1 {
		t.Fatalf("RemoveSpoke devrait retirer un spoke (reste %d)", len(c.Spokes))
	}
	if c.RemoveSpoke("Mission A") {
		t.Fatal("RemoveSpoke d'un nom déjà retiré devrait échouer")
	}
}
