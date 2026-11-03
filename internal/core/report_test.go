// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestTeamReportTotalsJSON verrouille la sérialisation des totaux en clés MINUSCULES : la page
// Statistiques lit rep.totals.commits/… — des clés capitalisées remettraient les KPI à 0.
func TestTeamReportTotalsJSON(t *testing.T) {
	rep := &TeamReport{}
	rep.Totals.Commits = 5
	rep.Totals.Additions = 120
	rep.Totals.Authors = 2
	b, err := json.Marshal(rep)
	if err != nil {
		t.Fatalf("Marshal : %v", err)
	}
	s := string(b)
	for _, want := range []string{`"commits":5`, `"additions":120`, `"authors":2`} {
		if !strings.Contains(s, want) {
			t.Errorf("le JSON du rapport doit contenir %s — obtenu %s", want, s)
		}
	}
	if strings.Contains(s, `"Commits"`) || strings.Contains(s, `"Authors"`) {
		t.Errorf("les champs de Totals ne doivent pas être capitalisés : %s", s)
	}
}
