// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package config

import "testing"

func TestInstanceIsGitHub(t *testing.T) {
	if (Instance{Provider: "github"}).IsGitHub() != true {
		t.Error("github instance should report IsGitHub")
	}
	if (Instance{}).IsGitHub() != false {
		t.Error("empty provider must default to Forgejo (not GitHub)")
	}
	if (Instance{Provider: "forgejo"}).IsGitHub() != false {
		t.Error("forgejo instance must not report IsGitHub")
	}
}
