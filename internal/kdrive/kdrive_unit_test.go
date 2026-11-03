// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package kdrive

import "testing"

func TestDigits(t *testing.T) {
	cases := map[string]string{
		"kDrive-123_456": "123456",
		"abc":            "",
		"007":            "007",
		"":               "",
	}
	for in, want := range cases {
		if got := digits(in); got != want {
			t.Errorf("digits(%q) = %q, attendu %q", in, got, want)
		}
	}
}

func TestClean(t *testing.T) {
	cases := map[string]string{
		"foo/bar/": "/foo/bar",
		"\\a\\b":   "/a/b",
		"/a/../b":  "/b",
		"  /x  ":   "/x",
		"rel/path": "/rel/path",
		"/":        "/",
	}
	for in, want := range cases {
		if got := clean(in); got != want {
			t.Errorf("clean(%q) = %q, attendu %q", in, got, want)
		}
	}
}
