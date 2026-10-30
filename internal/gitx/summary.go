// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package gitx

import "strings"

// Sum est un résumé léger pour la liste du tableau de bord (peu d'appels git).
type Sum struct {
	Status
	Describe string `json:"describe"`
	LastDate string `json:"last_date"` // date ISO du dernier commit local (pour afficher « maj … » sans n° de commit)
}

// SummaryOf renvoie l'état compact d'un dépôt local : statut (branche, avance/retard,
// modifs, remote) + version décrite. Adapté à un rafraîchissement fréquent sur
// beaucoup de dépôts.
func SummaryOf(dir string) (*Sum, error) {
	st, err := StatusOf(dir)
	if err != nil {
		return &Sum{Status: *st}, err
	}
	s := &Sum{Status: *st}
	if st.IsRepo {
		s.Describe = describe(dir)
		if out, err := run(dir, LocalTimeout, "log", "-1", "--format=%cI"); err == nil {
			s.LastDate = strings.TrimSpace(out)
		}
	}
	return s, nil
}

// describe renvoie la version décrite (`git describe --tags --always`), ou "".
func describe(dir string) string {
	if out, err := run(dir, LocalTimeout, "describe", "--tags", "--always"); err == nil {
		return strings.TrimSpace(out)
	}
	return ""
}
