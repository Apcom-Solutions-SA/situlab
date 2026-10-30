// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package gitx

import "testing"

func TestParseStatus(t *testing.T) {
	// Sortie type `git status --porcelain=v2 --branch` : branche, suivi, avance/retard,
	// un fichier modifié, un untracked avec espace, un renommage (séparé par TAB).
	out := "# branch.head work/situlab\n" +
		"# branch.upstream origin/work/situlab\n" +
		"# branch.ab +2 -1\n" +
		"1 .M N... 100644 100644 100644 aaa bbb internal/core/core.go\n" +
		"? mon fichier.txt\n" +
		"2 R. N... 100644 100644 100644 ccc ddd R100 nouveau.md\tancien.md\n"
	var s Status
	parseStatus(out, &s)

	if s.Branch != "work/situlab" {
		t.Errorf("branche = %q", s.Branch)
	}
	if s.Upstream != "origin/work/situlab" {
		t.Errorf("upstream = %q", s.Upstream)
	}
	if s.Ahead != 2 || s.Behind != 1 {
		t.Errorf("ahead/behind = %d/%d", s.Ahead, s.Behind)
	}
	if s.Clean {
		t.Error("devrait être marqué modifié")
	}
	want := []string{"internal/core/core.go", "mon fichier.txt", "nouveau.md"}
	if len(s.Changed) != len(want) {
		t.Fatalf("changed = %v", s.Changed)
	}
	for i, w := range want {
		if s.Changed[i] != w {
			t.Errorf("changed[%d] = %q, attendu %q", i, s.Changed[i], w)
		}
	}
}

func TestParseStatusDetachedClean(t *testing.T) {
	var s Status
	parseStatus("# branch.head (detached)\n# branch.ab +0 -0\n", &s)
	if !s.Detached {
		t.Error("devrait détecter la tête détachée")
	}
	if !s.Clean {
		t.Error("devrait être propre")
	}
}

func TestParseStatusUnmerged(t *testing.T) {
	// Entrée non fusionnée « u » : 10 champs avant le chemin (cas d'un conflit de fusion).
	out := "# branch.head main\n" +
		"u UU N... 100644 100644 100644 100644 aaa bbb ccc internal/core/conflit.go\n"
	var s Status
	parseStatus(out, &s)
	if len(s.Changed) != 1 || s.Changed[0] != "internal/core/conflit.go" {
		t.Errorf("chemin de conflit mal extrait : %v", s.Changed)
	}
}
