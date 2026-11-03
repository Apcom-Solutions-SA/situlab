// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import "testing"

// TestDedupReposByRemote : un même dépôt distant vu par deux connexions (même remote normalisé) est
// réduit à une seule entrée ; des dépôts réellement distincts sont conservés ; un remote vide ne
// fusionne jamais deux dépôts différents (repli instance|owner|repo). (#11, 1.19)
func TestDedupReposByRemote(t *testing.T) {
	in := []matrixRepo{
		{instance: "github apcom", owner: "Apcom-Solutions-SA", repo: "situlab", remote: "github.com/apcom-solutions-sa/situlab"},
		{instance: "github GLiG", owner: "Apcom-Solutions-SA", repo: "situlab", remote: "github.com/apcom-solutions-sa/situlab"},  // doublon (même remote)
		{instance: "github apcom", owner: "Apcom-Solutions-SA", repo: "glim_v2", remote: "github.com/apcom-solutions-sa/glim_v2"}, // distinct
		{instance: "x", owner: "o", repo: "sans-remote-1"},                                                                        // remote vide
		{instance: "x", owner: "o", repo: "sans-remote-2"},                                                                        // remote vide, distinct
		{instance: "x", owner: "o", repo: "sans-remote-1"},                                                                        // remote vide, doublon du 1er sans-remote
	}
	out := dedupRepos(in)
	if len(out) != 4 {
		t.Fatalf("attendu 4 entrées après dédoublonnage, obtenu %d : %+v", len(out), out)
	}
	// La première occurrence du doublon (connexion « github apcom ») est conservée.
	if out[0].instance != "github apcom" || out[0].repo != "situlab" {
		t.Fatalf("première occurrence attendue (github apcom/situlab), obtenu %+v", out[0])
	}
	// dédoublonnage non destructif de l'entrée (nouveau backing array).
	if len(in) != 6 {
		t.Fatalf("l'entrée ne doit pas être mutée, len(in)=%d", len(in))
	}
}
