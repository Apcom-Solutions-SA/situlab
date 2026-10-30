// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"testing"

	"situlab/internal/config"
)

func TestRefKey(t *testing.T) {
	a := refKey(config.RepoRef{Instance: "Git.Apcom", Owner: "LiG", Repo: "KERNEL"})
	b := refKey(config.RepoRef{Instance: "git.apcom", Owner: "lig", Repo: "kernel"})
	if a != b {
		t.Fatalf("refKey devrait être insensible à la casse : %q vs %q", a, b)
	}
}

func TestSplitFull(t *testing.T) {
	if o, r := splitFull("LiG/KERNEL", "KERNEL"); o != "LiG" || r != "KERNEL" {
		t.Fatalf("splitFull(owner/repo) = %q/%q", o, r)
	}
	if o, r := splitFull("", "solo"); o != "" || r != "solo" {
		t.Fatalf("splitFull sans / devrait retomber sur name : %q/%q", o, r)
	}
}

func TestBaseName(t *testing.T) {
	cases := map[string]string{
		`C:\tmp\KERNEL`:  "KERNEL",
		`C:\tmp\KERNEL\`: "KERNEL",
		"/c/tmp/site":    "site",
		"solo":           "solo",
	}
	for in, want := range cases {
		if got := baseName(in); got != want {
			t.Fatalf("baseName(%q) = %q, veut %q", in, got, want)
		}
	}
}

func TestRowScore(t *testing.T) {
	pending := MatrixRow{Atelier: MatrixCell{Present: true}, Pending: true, Local: MatrixCell{Present: true}}
	ranged := MatrixRow{Atelier: MatrixCell{Present: true}, Local: MatrixCell{Present: true}}
	orphan := MatrixRow{Local: MatrixCell{Present: true}}
	if !(rowScore(pending) > rowScore(ranged) && rowScore(ranged) > rowScore(orphan)) {
		t.Fatalf("ordre attendu pending > rangé > orphelin : %d %d %d", rowScore(pending), rowScore(ranged), rowScore(orphan))
	}
}
