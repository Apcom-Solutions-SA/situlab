// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package cli

import (
	"errors"
	"testing"
)

func TestValOr(t *testing.T) {
	if got := valOr("x", "d"); got != "x" {
		t.Errorf("valOr(x,d) = %q", got)
	}
	if got := valOr("", "d"); got != "d" {
		t.Errorf("valOr(vide,d) = %q", got)
	}
	if got := valOr("   ", "d"); got != "d" {
		t.Errorf("valOr(espaces,d) = %q", got)
	}
}

func TestSplitRepo(t *testing.T) {
	o, r, err := splitRepo("owner/repo")
	if err != nil || o != "owner" || r != "repo" {
		t.Fatalf("splitRepo(owner/repo) = %q,%q,%v", o, r, err)
	}
	// Un « / » dans le nom reste dans la partie repo (SplitN à 2).
	if o, r, err := splitRepo("org/sub/repo"); err != nil || o != "org" || r != "sub/repo" {
		t.Errorf("splitRepo(org/sub/repo) = %q,%q,%v", o, r, err)
	}
	for _, bad := range []string{"norepo", "owner/", "/repo", ""} {
		if _, _, err := splitRepo(bad); err == nil {
			t.Errorf("splitRepo(%q) devrait échouer", bad)
		}
	}
}

func TestSplitRepoNum(t *testing.T) {
	o, r, n, err := splitRepoNum([]string{"owner/repo", "42"})
	if err != nil || o != "owner" || r != "repo" || n != 42 {
		t.Fatalf("splitRepoNum = %q,%q,%d,%v", o, r, n, err)
	}
	if _, _, _, err := splitRepoNum([]string{"bad", "1"}); err == nil {
		t.Error("dépôt invalide devrait échouer")
	}
	if _, _, _, err := splitRepoNum([]string{"owner/repo", "x"}); err == nil {
		t.Error("numéro non entier devrait échouer")
	}
}

func TestLooksLikeAuthFailure(t *testing.T) {
	if !looksLikeAuthFailure(errors.New("mot de passe incorrect")) {
		t.Error("« mot de passe » devrait matcher")
	}
	if !looksLikeAuthFailure(errors.New("code 2FA requis")) {
		t.Error("« 2fa » devrait matcher")
	}
	if looksLikeAuthFailure(errors.New("session verrouillée")) {
		t.Error("une erreur locale ne devrait pas matcher")
	}
}
