// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"strings"
	"testing"
)

// promotionPlan traduit le mode choisi en actions concrètes ; c'est la source de vérité du « promouvoir
// ≠ publier une Release ». On vérifie les trois méthodes, le défaut, et que l'explication GitHub insiste
// sur la CI (le tag ne crée pas la Release à lui seul).
func TestPromotionPlan(t *testing.T) {
	cases := []struct {
		mode             string
		github           bool
		wantTag, wantRel bool
		mustMention      string
	}{
		{PromoteModeCode, true, false, false, "sans tag"},
		{PromoteModeTag, true, true, false, "CI"},
		{PromoteModeTag, false, true, false, "tag"},
		{PromoteModeRelease, true, true, true, "INSTALLEURS"},
		{PromoteModeRelease, false, true, true, "Release"},
		{"", true, true, false, "CI"}, // défaut = tag
		{"inconnu", true, true, false, "CI"},
	}
	for _, c := range cases {
		tag, rel, explain := promotionPlan(c.mode, c.github)
		if tag != c.wantTag || rel != c.wantRel {
			t.Errorf("mode=%q github=%v : tag=%v rel=%v (attendu %v/%v)", c.mode, c.github, tag, rel, c.wantTag, c.wantRel)
		}
		if c.mustMention != "" && !strings.Contains(explain, c.mustMention) {
			t.Errorf("mode=%q github=%v : explication %q devrait mentionner %q", c.mode, c.github, explain, c.mustMention)
		}
	}
}
