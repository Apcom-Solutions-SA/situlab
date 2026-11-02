// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package daemon

import (
	"errors"
	"fmt"
	"testing"

	"situlab/internal/core"
)

func TestTargetOf(t *testing.T) {
	cases := []struct {
		name string
		a    Args
		want string
	}{
		{"owner+repo prioritaire", Args{Owner: "o", Repo: "r", Path: "/p"}, "o/r"},
		{"chemin", Args{Path: "/c/tmp/x"}, "/c/tmp/x"},
		{"cloud", Args{Cloud: "nx"}, "nx"},
		{"nom", Args{Name: "projet"}, "projet"},
		{"vide", Args{}, ""},
		{"repo seul sans owner -> pas owner/repo", Args{Repo: "r", Name: "n"}, "n"},
	}
	for _, c := range cases {
		if got := targetOf(c.a); got != c.want {
			t.Errorf("%s : targetOf = %q, attendu %q", c.name, got, c.want)
		}
	}
}

func TestKindOf(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{core.ErrNoInstance, "noinstance"},
		{core.ErrPINNeeded, "pinneeded"},
		{core.ErrPINTimeout, "pintimeout"},
		{core.ErrPINLocked, "pinlocked"},
		{core.ErrNoGit, "nogit"},
		{core.ErrLocked, "locked"},
		{core.ErrCloudOperationForbidden, "cloudforbidden-op"},
		{errors.New("autre chose"), "generic"},
		// Doit traverser l'emballage (errors.Is).
		{fmt.Errorf("contexte : %w", core.ErrNoGit), "nogit"},
	}
	for _, c := range cases {
		if got := kindOf(c.err); got != c.want {
			t.Errorf("kindOf(%v) = %q, attendu %q", c.err, got, c.want)
		}
	}
}
