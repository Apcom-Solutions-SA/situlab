// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"path/filepath"
	"testing"
)

func TestIsDigits(t *testing.T) {
	for in, want := range map[string]bool{"123": true, "007": true, "": false, "12a": false, " 1": false} {
		if got := isDigits(in); got != want {
			t.Errorf("isDigits(%q) = %v, attendu %v", in, got, want)
		}
	}
}

func TestFirstSegment(t *testing.T) {
	for in, want := range map[string]string{"/a/b": "a", "//x/y": "x", "a/b": "a", "": "", "/": ""} {
		if got := firstSegment(in); got != want {
			t.Errorf("firstSegment(%q) = %q, attendu %q", in, got, want)
		}
	}
}

func TestCloudKeyID(t *testing.T) {
	if got := cloudKeyID("nx"); got != "filecloud/nx" {
		t.Errorf("cloudKeyID = %q", got)
	}
}

func TestPathUnder(t *testing.T) {
	root := filepath.Join("a", "b")
	cases := []struct {
		p    string
		want bool
	}{
		{filepath.Join("a", "b", "c"), true}, // enfant
		{filepath.Join("a", "b"), true},      // identique (rel « . »)
		{filepath.Join("a"), false},          // parent
		{filepath.Join("x", "y"), false},     // ailleurs
	}
	for _, c := range cases {
		if got := pathUnder(c.p, root); got != c.want {
			t.Errorf("pathUnder(%q, %q) = %v, attendu %v", c.p, root, got, c.want)
		}
	}
}

func TestCleanCloudPaths(t *testing.T) {
	got := cleanCloudPaths([]string{"/a", "/a", "/b/", "b"})
	// Dédoublonné et normalisé : /a une fois, /b une fois (« /b/ » et « b » = /b).
	if len(got) != 2 || got[0] != "/a" || got[1] != "/b" {
		t.Fatalf("cleanCloudPaths = %v, attendu [/a /b]", got)
	}
}
