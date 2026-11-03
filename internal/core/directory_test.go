// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"strings"
	"testing"

	"situlab/internal/forgejo"
)

func TestUserDirStatus(t *testing.T) {
	cases := []struct {
		u    forgejo.AdminUser
		want string
	}{
		{forgejo.AdminUser{Active: true}, "actif"},
		{forgejo.AdminUser{Active: false}, "inactif"},
		{forgejo.AdminUser{Active: true, ProhibitLogin: true}, "suspendu"}, // suspendu prime
	}
	for _, c := range cases {
		if got := userDirStatus(c.u); got != c.want {
			t.Errorf("userDirStatus(%+v) = %q, attendu %q", c.u, got, c.want)
		}
	}
}

func TestMergeDirectoryEntries(t *testing.T) {
	byInstance := map[string][]forgejo.AdminUser{
		"A": {
			{Login: "Greg", Email: "g@a", FullName: "Grégory", IsAdmin: true, Active: true},
			{Login: "alice", Active: true, ProhibitLogin: true},
		},
		"B": {
			{Login: "greg", Email: "greg@b", FullName: "G. Liand", Active: true}, // même personne, casse + e-mail différents
			{Login: "bob", Active: true},
		},
	}
	entries := mergeDirectoryEntries([]string{"A", "B"}, byInstance)

	// 3 personnes distinctes (greg fusionné malgré la casse).
	if len(entries) != 3 {
		t.Fatalf("attendu 3 personnes, obtenu %d : %+v", len(entries), entries)
	}
	by := map[string]DirEntry{}
	for _, e := range entries {
		by[e.Login] = e
	}

	greg, ok := by["Greg"] // la 1re occurrence (instance A) fixe le login affiché
	if !ok {
		t.Fatalf("Greg manquant : %+v", entries)
	}
	if !greg.Admin {
		t.Error("Greg devrait être admin (admin sur A)")
	}
	if greg.Email != "g@a" || greg.FullName != "Grégory" {
		t.Errorf("identité de Greg mal reprise : %+v", greg)
	}
	if greg.Presence["A"] != "actif" || greg.Presence["B"] != "actif" {
		t.Errorf("présence de Greg incorrecte : %+v", greg.Presence)
	}
	// #7 : la recherche transverse doit retrouver Greg par l'e-mail/nom de CHAQUE instance.
	for _, term := range []string{"greg", "g@a", "greg@b", "grégory", "g. liand"} {
		if !strings.Contains(greg.Search, term) {
			t.Errorf("Search de Greg devrait contenir %q : %q", term, greg.Search)
		}
	}

	if by["alice"].Presence["A"] != "suspendu" || by["alice"].Presence["B"] != "absent" {
		t.Errorf("alice : attendu A=suspendu B=absent, obtenu %+v", by["alice"].Presence)
	}
	if by["bob"].Presence["A"] != "absent" || by["bob"].Presence["B"] != "actif" {
		t.Errorf("bob : attendu A=absent B=actif, obtenu %+v", by["bob"].Presence)
	}
	if by["alice"].Admin {
		t.Error("alice ne devrait pas être admin")
	}
}
