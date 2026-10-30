// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package gitx

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestParseIdent(t *testing.T) {
	cases := []struct{ in, n, e, d string }{
		{"Grégory Liand <gl-lig@pm.me> 1699999999 +0200", "Grégory Liand", "gl-lig@pm.me", "1699999999 +0200"},
		{"A B <a@b> 1 +0000", "A B", "a@b", "1 +0000"},
		{"NoEmail", "NoEmail", "", ""},
	}
	for _, c := range cases {
		if n, e, d := parseIdent(c.in); n != c.n || e != c.e || d != c.d {
			t.Errorf("parseIdent(%q) = (%q,%q,%q), attendu (%q,%q,%q)", c.in, n, e, d, c.n, c.e, c.d)
		}
	}
}

func TestParseCommitObject(t *testing.T) {
	raw := "tree aaa\nparent bbb\nparent ccc\nauthor A <a> 1 +0200\ncommitter C <c> 2 +0300\n" +
		"gpgsig -----BEGIN-----\n junk\n -----END-----\n\nsujet\n\ncorps\n"
	o := parseCommitObject(raw)
	if o.Tree != "aaa" {
		t.Errorf("tree=%q", o.Tree)
	}
	if len(o.Parents) != 2 || o.Parents[0] != "bbb" || o.Parents[1] != "ccc" {
		t.Errorf("parents=%v", o.Parents)
	}
	if o.Author != "A <a> 1 +0200" || o.AuthorDate != "1 +0200" {
		t.Errorf("author=%q date=%q", o.Author, o.AuthorDate)
	}
	if o.CommitterName != "C" || o.CommitterEmail != "c" {
		t.Errorf("committer name=%q email=%q", o.CommitterName, o.CommitterEmail)
	}
	if strings.TrimRight(o.Message, "\n") != "sujet\n\ncorps" {
		t.Errorf("message=%q", o.Message)
	}
}

func gitExec(t *testing.T, dir string, args ...string) {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = dir
	c.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("git %v : %v\n%s", args, err, out)
	}
}

// TestRecommitWithTrailerRoundTrip : recréer un commit avec trailer préserve l'arbre et les identités,
// change le SHA, avance la branche, et le corps SANS le trailer reconstruit exactement le corps d'origine
// (donc le pré-image signé). C'est l'invariant qui rend l'attestation v2 vérifiable et immuable.
func TestRecommitWithTrailerRoundTrip(t *testing.T) {
	if !Available() {
		t.Skip("git indisponible")
	}
	dir := t.TempDir()
	gitExec(t, dir, "init")
	gitExec(t, dir, "config", "user.name", "Gré Test")
	gitExec(t, dir, "config", "user.email", "gre@test")
	if err := os.WriteFile(dir+"/a.txt", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Commit(dir, "sujet du commit\n\ncorps multi\nligne"); err != nil {
		t.Fatal(err)
	}
	sha := HeadSHA(dir)
	obj, err := CommitObject(dir, sha)
	if err != nil {
		t.Fatal(err)
	}
	body0 := strings.TrimRight(obj.Message, "\n")

	newSha, err := RecommitWithTrailer(dir, sha, "SituLab-Approved", "VAL==")
	if err != nil {
		t.Fatalf("RecommitWithTrailer : %v", err)
	}
	if newSha == sha {
		t.Error("le SHA doit changer : le trailer fait partie de l'objet commit")
	}
	if HeadSHA(dir) != newSha {
		t.Error("la branche courante doit pointer sur le nouveau commit")
	}

	o2, err := CommitObject(dir, newSha)
	if err != nil {
		t.Fatal(err)
	}
	if o2.Author != obj.Author || o2.Committer != obj.Committer {
		t.Errorf("identités modifiées : %q/%q vs %q/%q", o2.Author, o2.Committer, obj.Author, obj.Committer)
	}
	// Corps sans trailer == corps d'origine.
	var kept []string
	found := false
	for _, l := range strings.Split(o2.Message, "\n") {
		if strings.HasPrefix(l, "SituLab-Approved: ") {
			found = true
			continue
		}
		kept = append(kept, l)
	}
	if !found {
		t.Fatal("trailer absent du nouveau commit")
	}
	if rebuilt := strings.TrimRight(strings.Join(kept, "\n"), "\n"); rebuilt != body0 {
		t.Errorf("corps reconstruit ≠ corps signé :\n%q\nvs\n%q", rebuilt, body0)
	}
	if st, err := StatusOf(dir); err == nil && !st.Clean {
		t.Error("l'arbre doit rester propre après recommit (même arbre)")
	}
}
