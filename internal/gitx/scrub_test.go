// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package gitx

import (
	"strings"
	"testing"
)

// TestScrub : scrub masque le couple identifiant:token d'une URL (https://u:t@host) avant que le
// message d'erreur git ne soit exposé à l'interface. Une URL sans identifiants reste intacte, et
// toutes les occurrences d'une même chaîne sont masquées, pas seulement la première.
func TestScrub(t *testing.T) {
	// Avec identifiants : le secret disparaît, le marqueur de masquage et l'hôte subsistent.
	got := scrub("https://user:token@host/r.git")
	if strings.Contains(got, "token") {
		t.Errorf("le jeton ne devrait plus apparaître : %q", got)
	}
	if !strings.Contains(got, "***@") {
		t.Errorf("le masquage « ***@ » est attendu : %q", got)
	}
	if !strings.Contains(got, "host") {
		t.Errorf("l'hôte doit rester lisible : %q", got)
	}

	// Sans identifiants : rien à masquer, chaîne inchangée.
	const plain = "https://host/r.git"
	if got := scrub(plain); got != plain {
		t.Errorf("une URL sans identifiants doit rester intacte : %q", got)
	}

	// Deux URL à identifiants dans la même chaîne : les deux secrets masqués.
	two := scrub("remote https://alice:AAA111@h1.example/a.git puis https://bob:BBB222@h2.example/b.git")
	for _, secret := range []string{"AAA111", "BBB222", "alice", "bob"} {
		if strings.Contains(two, secret) {
			t.Errorf("le secret %q ne devrait plus apparaître : %q", secret, two)
		}
	}
	if n := strings.Count(two, "***@"); n != 2 {
		t.Errorf("les deux identifiants devraient être masqués (2 × « ***@ »), obtenu %d : %q", n, two)
	}
}
