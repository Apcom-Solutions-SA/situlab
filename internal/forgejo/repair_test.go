// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package forgejo

import "testing"

func TestRepairUTF8(t *testing.T) {
	// "déployée" avec les é en Latin-1 (0xE9) : doit redevenir de l'UTF-8 valide « déployée ».
	in := []byte{'d', 0xE9, 'p', 'l', 'o', 'y', 0xE9, 'e'}
	got := string(repairUTF8(in))
	if got != "déployée" {
		t.Fatalf("repairUTF8 Latin-1 = %q, attendu « déployée »", got)
	}
	// Tiret long CP1252 (0x97) → — (U+2014).
	if got := string(repairUTF8([]byte{'a', 0x97, 'b'})); got != "a—b" {
		t.Fatalf("repairUTF8 CP1252 = %q, attendu « a—b »", got)
	}
	// UTF-8 DÉJÀ valide (— encodé en UTF-8) : laissé strictement intact.
	valid := []byte("édité — ok")
	if got := repairUTF8(valid); string(got) != string(valid) {
		t.Fatalf("repairUTF8 a modifié de l'UTF-8 valide : %q", string(got))
	}
}
