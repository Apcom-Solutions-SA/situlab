// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package main

import "testing"

func TestNormalizeVersion(t *testing.T) {
	cases := map[string]string{
		"v1.21.0":      "1.21.0",
		"1.21.0":       "1.21.0",
		"V1.21.0":      "1.21.0",
		"1.21.0-dev":   "1.21.0",
		"1.22.0-rc.1":  "1.22.0",
		"  1.21.1  ":   "1.21.1",
		"1.21.0+build": "1.21.0",
	}
	for in, want := range cases {
		if got := normalizeVersion(in); got != want {
			t.Errorf("normalizeVersion(%q) = %q, attendu %q", in, got, want)
		}
	}
}

func TestCompareVersions(t *testing.T) {
	type c struct {
		a, b string
		want int
	}
	for _, tc := range []c{
		{"1.21.1", "1.21.0", 1},
		{"1.21.0", "1.21.1", -1},
		{"1.21.0", "1.21.0", 0},
		{"1.22.0", "1.21.9", 1},
		{"2.0.0", "1.99.99", 1},
		{"1.21", "1.21.0", 0}, // segment manquant = 0
		{"1.21.0", "1.21", 0},
		{"1.21.10", "1.21.9", 1}, // comparaison numérique, pas lexicale
	} {
		if got := compareVersions(tc.a, tc.b); got != tc.want {
			t.Errorf("compareVersions(%q,%q) = %d, attendu %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestPickAssetForOS(t *testing.T) {
	// Release type avec les trois assets d'une vraie Release SituLab.
	rel := ghReleaseRaw{Assets: []struct {
		Name        string `json:"name"`
		DownloadURL string `json:"browser_download_url"`
	}{
		{Name: "SituLab-1.21.0-linux-amd64.tar.gz", DownloadURL: "u-linux"},
		{Name: "SituLab-1.21.0-macos-universal.dmg", DownloadURL: "u-mac"},
		{Name: "SituLab-1.21.0-windows-amd64-setup.exe", DownloadURL: "u-win"},
	}}
	// On ne teste pas le GOOS courant (variable selon la machine CI) mais la sélection par extension :
	// l'installeur de chaque OS doit être retrouvable par son suffixe caractéristique.
	name, url := pickAssetForOS(rel)
	if name == "" || url == "" {
		t.Fatalf("pickAssetForOS n'a retourné aucun asset pour GOOS courant")
	}
}
