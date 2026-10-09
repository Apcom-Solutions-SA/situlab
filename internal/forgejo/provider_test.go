// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package forgejo

import (
	"errors"
	"net/http"
	"testing"
)

func TestAPIURLByProvider(t *testing.T) {
	fj := NewKind("https://git.apcom.app/", "tok", "1.0", KindForgejo)
	if got := fj.apiURL("/user"); got != "https://git.apcom.app/api/v1/user" {
		t.Errorf("forgejo apiURL = %q", got)
	}
	gh := NewKind("https://github.com", "tok", "1.0", KindGitHub)
	if got := gh.apiURL("/user"); got != "https://api.github.com/user" {
		t.Errorf("github apiURL = %q", got)
	}
}

func TestLimKey(t *testing.T) {
	if New("x", "", "1").limKey() != "limit" {
		t.Error("forgejo limKey != limit")
	}
	if NewKind("x", "", "1", KindGitHub).limKey() != "per_page" {
		t.Error("github limKey != per_page")
	}
}

func TestRequireForgejoBlocksGitHub(t *testing.T) {
	gh := NewKind("https://github.com", "", "1", KindGitHub)
	if err := gh.requireForgejo(); !errors.Is(err, ErrGitHubUnsupported) {
		t.Errorf("github requireForgejo = %v, want ErrGitHubUnsupported", err)
	}
	if err := New("x", "", "1").requireForgejo(); err != nil {
		t.Errorf("forgejo requireForgejo = %v, want nil", err)
	}
}

func TestNormalizeUserGitHub(t *testing.T) {
	gh := NewKind("https://github.com", "", "1", KindGitHub)
	u := &User{Login: "octocat", Name: "The Octocat", SiteAdmin: true}
	gh.normalizeUser(u)
	if u.FullName != "The Octocat" {
		t.Errorf("FullName = %q, want mapped from Name", u.FullName)
	}
	if !u.IsAdmin {
		t.Error("IsAdmin should be mapped from SiteAdmin")
	}
	// Forgejo : pas de remappage.
	fj := New("x", "", "1")
	u2 := &User{Login: "greg", FullName: "Greg", IsAdmin: false, SiteAdmin: true}
	fj.normalizeUser(u2)
	if u2.IsAdmin {
		t.Error("forgejo normalizeUser must not touch IsAdmin")
	}
}

func TestAdminBlockedOnGitHub(t *testing.T) {
	gh := NewKind("https://github.com", "", "1", KindGitHub)
	if _, err := gh.ListUsers(nil); !errors.Is(err, ErrGitHubUnsupported) {
		t.Errorf("ListUsers on github = %v", err)
	}
	if _, err := gh.TokenCreate(nil, "u", "p", "", "n", nil, nil); !errors.Is(err, ErrGitHubUnsupported) {
		t.Errorf("TokenCreate on github = %v", err)
	}
}

func TestGitHubHeaders(t *testing.T) {
	gh := NewKind("https://github.com", "tok", "1", KindGitHub)
	req, _ := http.NewRequest(http.MethodGet, "https://api.github.com/user", nil)
	gh.setHeaders(req)
	if req.Header.Get("Accept") != "application/vnd.github+json" {
		t.Errorf("github Accept = %q", req.Header.Get("Accept"))
	}
	if req.Header.Get("Authorization") != "token tok" {
		t.Errorf("Authorization = %q", req.Header.Get("Authorization"))
	}
}
