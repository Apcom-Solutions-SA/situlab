// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"runtime"
	"testing"
)

// Le verrou par dépôt sérialise les opérations git d'un même chemin ; des chemins différents restent
// indépendants ; tryLockRepo signale correctement l'occupation (le fetch de fond saute alors ce dépôt).
func TestRepoLock(t *testing.T) {
	s := &Service{}

	// Verrou pris : une tentative non bloquante échoue.
	release := s.lockRepo("/a/repo")
	if _, ok := s.tryLockRepo("/a/repo"); ok {
		t.Fatal("tryLockRepo devrait échouer tant que le dépôt est verrouillé")
	}
	// Un AUTRE dépôt reste disponible en parallèle.
	rel2, ok := s.tryLockRepo("/b/repo")
	if !ok {
		t.Fatal("un dépôt différent doit rester verrouillable indépendamment")
	}
	rel2()

	// Libéré : de nouveau disponible.
	release()
	rel3, ok := s.tryLockRepo("/a/repo")
	if !ok {
		t.Fatal("après libération, le dépôt doit être de nouveau verrouillable")
	}
	rel3()
}

// Optimisation 1.20 : toute libération du verrou de dépôt SIGNALE que l'état git a pu changer, pour que
// l'UI se rafraîchisse sans attendre la récupération périodique. Le relais est nil-safe quand aucun n'a
// été posé (CLI headless, tests).
func TestGitChangeSignalOnUnlock(t *testing.T) {
	s := &Service{}

	// Sans relais posé : aucun panic (nil-safe), ni à la libération, ni par appel direct.
	s.lockRepo("/a/repo")()
	s.SignalGitChange()

	// Relais posé : la libération du verrou le déclenche, et l'appel direct aussi.
	n := 0
	s.SetGitChanged(func() { n++ })
	s.lockRepo("/a/repo")() // mutation locale/distante → un signal à la libération
	if n != 1 {
		t.Fatalf("la libération du verrou devrait signaler une fois, obtenu %d", n)
	}
	s.SignalGitChange() // frontière (démon/fenêtre) pour un geste d'API distante
	if n != 2 {
		t.Fatalf("SignalGitChange direct devrait signaler, total attendu 2, obtenu %d", n)
	}
}

// Sur un système de fichiers insensible à la casse (Windows), deux graphies du même chemin doivent
// partager le même verrou — sinon deux git tourneraient en parallèle sur le même .git.
func TestRepoLockCaseFolding(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("normalisation de casse spécifique à Windows")
	}
	s := &Service{}
	release := s.lockRepo(`C:\Tmp\Repo`)
	defer release()
	if _, ok := s.tryLockRepo(`c:\tmp\repo`); ok {
		t.Fatal("deux graphies du même chemin doivent partager le verrou")
	}
}
