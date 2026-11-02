// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package journal

import (
	"strings"
	"testing"
)

func TestLevelRetention(t *testing.T) {
	d := t.TempDir()
	j := New(d, LevelErrors)
	j.Log(Entry{Op: "push", OK: true})            // succès au niveau errors : ignoré
	j.Log(Entry{Op: "push", OK: false, Err: "x"}) // échec : retenu
	got, _ := j.Read(10, false)
	if len(got) != 1 || got[0].OK {
		t.Fatalf("niveau errors : seul l'échec doit rester, obtenu %+v", got)
	}
	j.SetLevel(LevelAll)
	j.Log(Entry{Op: "fetch", OK: true})
	got, _ = j.Read(10, false)
	if len(got) != 2 {
		t.Fatalf("niveau all : succès aussi retenu, obtenu %d", len(got))
	}
	j.SetLevel(LevelOff)
	j.Log(Entry{Op: "pull", OK: false, Err: "y"})
	got, _ = j.Read(10, false)
	if len(got) != 2 {
		t.Fatalf("niveau off : rien de plus, obtenu %d", len(got))
	}
}

func TestScrubSecretsInError(t *testing.T) {
	d := t.TempDir()
	j := New(d, LevelAll)
	j.Log(Entry{Op: "push", OK: false, Err: "fatal: https://user:ghp_secrettoken@git.example.org/x refused"})
	got, _ := j.Read(10, false)
	if len(got) != 1 || strings.Contains(got[0].Err, "ghp_secrettoken") || strings.Contains(got[0].Err, "user:") {
		t.Fatalf("le jeton dans l'URL doit être masqué, obtenu %q", got[0].Err)
	}
	if !strings.Contains(got[0].Err, "***@") {
		t.Fatalf("masquage attendu (***@), obtenu %q", got[0].Err)
	}
}

func TestReadOrderAndLimit(t *testing.T) {
	d := t.TempDir()
	j := New(d, LevelAll)
	for _, op := range []string{"a", "b", "c", "d"} {
		j.Log(Entry{Op: op, OK: true})
	}
	got, _ := j.Read(2, false)
	if len(got) != 2 || got[0].Op != "d" || got[1].Op != "c" {
		t.Fatalf("les plus récentes d'abord, limitées à 2 ; obtenu %+v", got)
	}
	errs, _ := j.Read(10, true)
	if len(errs) != 0 {
		t.Fatalf("onlyErr ne doit rien renvoyer (que des succès), obtenu %d", len(errs))
	}
}

func TestRotation(t *testing.T) {
	d := t.TempDir()
	j := New(d, LevelAll)
	j.maxBytes = 300 // rotation rapide
	j.keep = 2
	for i := 0; i < 50; i++ {
		j.Log(Entry{Op: "op", Target: "repo-long-name-to-fill-the-line", OK: true})
	}
	// Après rotation, la lecture doit toujours rendre des entrées récentes, sans erreur.
	got, _ := j.Read(5, false)
	if len(got) == 0 {
		t.Fatal("après rotation, la lecture doit renvoyer des entrées")
	}
}

func TestClear(t *testing.T) {
	d := t.TempDir()
	j := New(d, LevelAll)
	j.Log(Entry{Op: "x", OK: true})
	if err := j.Clear(); err != nil {
		t.Fatal(err)
	}
	got, _ := j.Read(10, false)
	if len(got) != 0 {
		t.Fatalf("après Clear, journal vide attendu, obtenu %d", len(got))
	}
}

func TestNormalizeLevel(t *testing.T) {
	for in, want := range map[string]Level{"off": LevelOff, "OFF": LevelOff, "all": LevelAll, "": LevelErrors, "bogus": LevelErrors, "errors": LevelErrors} {
		if got := NormalizeLevel(in); got != want {
			t.Errorf("NormalizeLevel(%q) = %q, attendu %q", in, got, want)
		}
	}
}
