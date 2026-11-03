// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package gitx

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestParseTrack(t *testing.T) {
	cases := map[string][2]int{
		"[ahead 2, behind 3]": {2, 3},
		"[ahead 5]":           {5, 0},
		"[behind 1]":          {0, 1},
		"":                    {0, 0},
		"[gone]":              {0, 0},
	}
	for in, want := range cases {
		a, b := parseTrack(in)
		if a != want[0] || b != want[1] {
			t.Errorf("parseTrack(%q) = (%d,%d), want (%d,%d)", in, a, b, want[0], want[1])
		}
	}
}

func TestSafeRef(t *testing.T) {
	ok := []string{"main", "dev", "feature/x", "release-1.2"}
	for _, n := range ok {
		if err := safeRef(n); err != nil {
			t.Errorf("safeRef(%q) devrait passer : %v", n, err)
		}
	}
	bad := []string{"", "-rf", "--force", "a b", "a~b", "a^b", "a:b", "a?b", "a*b", "a[b"}
	for _, n := range bad {
		if err := safeRef(n); err == nil {
			t.Errorf("safeRef(%q) devrait être refusé", n)
		}
	}
}

// TestAuthCfgKeepsTokenOutOfArgv : l'en-tête d'auth (porteur du jeton) doit être fourni par
// l'ENVIRONNEMENT (GIT_CONFIG_*), jamais par un argument « -c » visible dans la liste des
// processus. Garde-fou contre une régression (jeton hors argv).
func TestAuthCfgKeepsTokenOutOfArgv(t *testing.T) {
	const tok = "secret-token-xyz"
	const base = "https://git.example.org"
	for _, github := range []bool{false, true} {
		env := authCfg(tok, github, base)
		if len(env) == 0 {
			t.Fatalf("authCfg(github=%v) vide", github)
		}
		var valueHasToken bool
		for _, e := range env {
			if strings.HasPrefix(e, "-c") {
				t.Errorf("authCfg a produit un argument -c (jeton exposable en argv) : %q", e)
			}
			if !strings.HasPrefix(e, "GIT_CONFIG_") {
				t.Errorf("authCfg doit ne produire que des GIT_CONFIG_* ; obtenu %q", e)
			}
			if strings.HasPrefix(e, "GIT_CONFIG_VALUE_0=") && strings.Contains(e, tok) {
				valueHasToken = true
			}
			// L'en-tête doit être scopé à l'hôte de l'instance (S2/#3) : jamais global, sinon
			// le jeton suivrait une redirection cross-host.
			if strings.HasPrefix(e, "GIT_CONFIG_KEY_0=") && e != "GIT_CONFIG_KEY_0=http."+base+".extraHeader" {
				t.Errorf("clé non scopée à l'hôte : %q", e)
			}
		}
		if !github && !valueHasToken {
			t.Error("le jeton Forgejo devrait être dans GIT_CONFIG_VALUE_0 (environnement)")
		}
	}
	if authCfg("", false, base) != nil {
		t.Error("authCfg sans jeton doit être nil")
	}
}

// bareAndClone monte un dépôt bare « origin » et son clone de travail : un commit sur main poussé
// en amont, puis un HEAD symbolique distant via `git remote set-head` (crée refs/remotes/origin/HEAD).
// Renvoie le chemin du clone. Helper local : newRepo ne gère pas le couple bare + distant. On réutilise
// mustGit/write (déjà définis dans le paquet) plutôt que de réécrire l'enveloppe os/exec.
func bareAndClone(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	bare := filepath.Join(root, "origin.git")
	clone := filepath.Join(root, "work")
	mustGit(t, root, "init", "--bare", "-b", "main", bare)
	mustGit(t, root, "clone", bare, clone)
	mustGit(t, clone, "config", "user.email", "t@t.t")
	mustGit(t, clone, "config", "user.name", "T")
	mustGit(t, clone, "config", "commit.gpgsign", "false")
	write(t, clone, "a.txt", "un\n")
	mustGit(t, clone, "add", "-A")
	mustGit(t, clone, "commit", "-m", "init")
	mustGit(t, clone, "branch", "-M", "main") // garantit « main » quel que soit le défaut local
	mustGit(t, clone, "push", "-u", "origin", "main")
	mustGit(t, clone, "remote", "set-head", "origin", "main")
	return clone
}

// repoOnMainWithTags crée un dépôt local sur « main » avec deux commits, tag v1.0 sur le premier
// et v1.1 sur HEAD. Helper local : newRepo ne pose ni la branche « main » ni de tags.
func repoOnMainWithTags(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mustGit(t, dir, "init", "-b", "main")
	mustGit(t, dir, "config", "user.email", "t@t.t")
	mustGit(t, dir, "config", "user.name", "T")
	mustGit(t, dir, "config", "commit.gpgsign", "false")
	write(t, dir, "a.txt", "un\n")
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-m", "c1")
	mustGit(t, dir, "tag", "v1.0")
	write(t, dir, "a.txt", "un\ndeux\n")
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-m", "c2")
	mustGit(t, dir, "tag", "v1.1")
	return dir
}

// TestMatrixOfExcludesRemoteHEAD : le HEAD symbolique d'un distant (refs/remotes/origin/HEAD) ne
// doit jamais apparaître comme branche — ni sous « HEAD », ni sous « origin ». Cœur du correctif :
// seule la vraie branche distante (origin/main → main) est retenue.
func TestMatrixOfExcludesRemoteHEAD(t *testing.T) {
	if !Available() {
		t.Skip("git indisponible")
	}
	clone := bareAndClone(t)

	m, err := MatrixOf(clone)
	if err != nil {
		t.Fatalf("MatrixOf : %v", err)
	}
	if !m.IsRepo {
		t.Fatal("le clone devrait être reconnu comme dépôt")
	}
	var main *BranchRow
	for i := range m.Branches {
		switch m.Branches[i].Name {
		case "HEAD", "origin":
			t.Errorf("le HEAD symbolique distant ne doit pas apparaître comme branche : %q", m.Branches[i].Name)
		case "main":
			main = &m.Branches[i]
		}
	}
	if main == nil {
		t.Fatalf("la branche « main » devrait figurer dans la matrice : %+v", m.Branches)
	}
	if !main.HasRemote {
		t.Errorf("« main » devrait être vue en ligne (HasRemote) : %+v", *main)
	}

	// Un dossier quelconque n'est pas un dépôt : IsRepo faux, sans erreur.
	m2, err := MatrixOf(t.TempDir())
	if err != nil {
		t.Fatalf("MatrixOf(non-dépôt) ne doit pas renvoyer d'erreur : %v", err)
	}
	if m2.IsRepo {
		t.Error("un dossier vide ne devrait pas être reconnu comme dépôt")
	}
}

// TestTagsFor : les tags joignables depuis une réf sont renvoyés du plus récent au plus ancien
// (tri -version:refname) ; une réf inexistante donne une liste vide.
func TestTagsFor(t *testing.T) {
	if !Available() {
		t.Skip("git indisponible")
	}
	dir := repoOnMainWithTags(t)

	if got := TagsFor(dir, "main"); len(got) != 2 || got[0] != "v1.1" || got[1] != "v1.0" {
		t.Fatalf("TagsFor(main) = %v, attendu [v1.1 v1.0]", got)
	}
	if got := TagsFor(dir, "origin/inexistante"); len(got) != 0 {
		t.Errorf("TagsFor(réf inexistante) devrait être vide, obtenu %v", got)
	}
}

// TestNearestTagOf : le tag le plus proche d'une réf est son plus récent joignable ; chaîne vide
// pour une réf inexistante.
func TestNearestTagOf(t *testing.T) {
	if !Available() {
		t.Skip("git indisponible")
	}
	dir := repoOnMainWithTags(t)

	if got := NearestTagOf(dir, "main"); got != "v1.1" {
		t.Fatalf("NearestTagOf(main) = %q, attendu v1.1", got)
	}
	if got := NearestTagOf(dir, "origin/inexistante"); got != "" {
		t.Errorf("NearestTagOf(réf inexistante) devrait être vide, obtenu %q", got)
	}
}
