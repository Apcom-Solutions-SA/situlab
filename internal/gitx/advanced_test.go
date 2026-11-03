// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package gitx

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// run exécute git dans dir pour la préparation des scénarios (ignore le code de sortie si check=false).
func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s : %v\n%s", strings.Join(args, " "), err, out)
	}
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestAuthCfgScoping : l'en-tête d'auth n'est JAMAIS global/non scopé — vide si
// pas de token, et scopé à l'hôte quand baseURL est fourni ; rien si baseURL manque.
func TestAuthCfgScoping(t *testing.T) {
	if authCfg("", false, "https://x") != nil {
		t.Error("sans token, aucune config d'auth")
	}
	if authCfg("tok", false, "") != nil {
		t.Error("sans baseURL, on ne pose PAS d'en-tête global non scopé")
	}
	env := authCfg("tok", false, "https://git.example/")
	joined := strings.Join(env, "\n")
	if !strings.Contains(joined, "http.https://git.example.extraHeader") {
		t.Errorf("l'en-tête doit être scopé à l'hôte : %q", joined)
	}
	if strings.Contains(joined, "GIT_CONFIG_KEY_0=http.extraHeader") {
		t.Error("l'en-tête GLOBAL non scopé ne doit jamais être produit")
	}
}

// TestDiscardRejectsPathspecMagic : DiscardChanges refuse la magie de pathspec sur un chemin fourni.
func TestDiscardRejectsPathspecMagic(t *testing.T) {
	dir := newRepo(t)
	if _, err := DiscardChanges(dir, ":/"); err == nil {
		t.Error("DiscardChanges devrait refuser un pathspec magique (:/) fourni par l'appelant")
	}
	if _, err := DiscardChanges(dir, "-rf"); err == nil {
		t.Error("DiscardChanges devrait refuser un chemin façon option")
	}
}

func TestSafeRevision(t *testing.T) {
	ok := []string{"HEAD~1", "origin/main", "abc1234", "v1.2^", "HEAD@{1}"}
	for _, r := range ok {
		if err := safeRevision(r); err != nil {
			t.Errorf("safeRevision(%q) devrait passer : %v", r, err)
		}
	}
	bad := []string{"", "  ", "--force", "-x", "a b", "a\tb"}
	for _, r := range bad {
		if err := safeRevision(r); err == nil {
			t.Errorf("safeRevision(%q) devrait échouer", r)
		}
	}
}

func TestSafePathspec(t *testing.T) {
	if err := safePathspec("dossier/fichier.txt"); err != nil {
		t.Errorf("chemin légitime refusé : %v", err)
	}
	for _, p := range []string{"", "-rf", "--all"} {
		if err := safePathspec(p); err == nil {
			t.Errorf("safePathspec(%q) devrait échouer", p)
		}
	}
}

// TestMergeFastForward : fusionner une branche en avance avance proprement (pas de conflit).
func TestMergeFastForward(t *testing.T) {
	dir := newRepo(t)
	mustGit(t, dir, "switch", "-c", "feature")
	write(t, dir, "b.txt", "feature\n")
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-m", "feature")
	mustGit(t, dir, "switch", "-") // retour sur la branche initiale
	mustGit(t, dir, "config", "advice.detachedHead", "false")
	if _, err := Merge(dir, "feature"); err != nil {
		t.Fatalf("Merge : %v", err)
	}
	st, _ := StatusOf(dir)
	if !st.Clean || len(st.Conflicted) != 0 {
		t.Fatalf("après fusion fast-forward l'arbre doit être propre : %+v", st)
	}
}

// TestRevert : revert ajoute un commit inverse sans toucher l'historique antérieur.
func TestRevert(t *testing.T) {
	dir := newRepo(t)
	write(t, dir, "a.txt", "un\ndeux\n")
	if _, err := Commit(dir, "ajoute deux"); err != nil {
		t.Fatal(err)
	}
	before := len(Log(dir, 50))
	if _, err := Revert(dir, "HEAD"); err != nil {
		t.Fatalf("Revert : %v", err)
	}
	if after := len(Log(dir, 50)); after != before+1 {
		t.Fatalf("revert devrait ajouter 1 commit : avant=%d après=%d", before, after)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "a.txt"))
	if strings.Contains(string(b), "deux") {
		t.Fatalf("revert devrait avoir retiré « deux » : %q", b)
	}
}

// TestResetHard : reset --hard ramène l'arbre à un commit antérieur (destructif).
func TestResetHard(t *testing.T) {
	dir := newRepo(t)
	write(t, dir, "a.txt", "un\nmodif\n")
	if _, err := Commit(dir, "modif"); err != nil {
		t.Fatal(err)
	}
	if _, err := Reset(dir, "hard", "HEAD~1"); err != nil {
		t.Fatalf("Reset : %v", err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "a.txt"))
	if strings.TrimSpace(string(b)) != "un" { // TrimSpace : neutralise le CRLF de checkout sous Windows
		t.Fatalf("reset --hard devrait restaurer le contenu initial, obtenu %q", b)
	}
	if _, err := Reset(dir, "n'importe", "HEAD"); err == nil {
		t.Error("un mode de reset invalide devrait être refusé")
	}
}

// TestAmend : amend réécrit le dernier commit (message + arbre) sans en créer un nouveau.
func TestAmend(t *testing.T) {
	dir := newRepo(t)
	before := len(Log(dir, 50))
	write(t, dir, "a.txt", "un\nplus\n")
	if _, err := Amend(dir, "init corrigé"); err != nil {
		t.Fatalf("Amend : %v", err)
	}
	cs := Log(dir, 50)
	if len(cs) != before {
		t.Fatalf("amend ne doit pas ajouter de commit : avant=%d après=%d", before, len(cs))
	}
	if cs[0].Message != "init corrigé" {
		t.Fatalf("le message du dernier commit devrait être amendé, obtenu %q", cs[0].Message)
	}
}

// TestCommitPaths : seul le chemin indiqué est enregistré ; l'autre reste modifié.
func TestCommitPaths(t *testing.T) {
	dir := newRepo(t)
	write(t, dir, "garde.txt", "à enregistrer\n")
	write(t, dir, "parasite.txt", "à NE PAS enregistrer\n")
	if _, err := CommitPaths(dir, "sélectif", []string{"garde.txt"}); err != nil {
		t.Fatalf("CommitPaths : %v", err)
	}
	st, _ := StatusOf(dir)
	if st.Clean {
		t.Fatal("parasite.txt devrait rester non enregistré")
	}
	found := false
	for _, c := range st.Changed {
		if strings.Contains(c, "parasite.txt") {
			found = true
		}
		if strings.Contains(c, "garde.txt") {
			t.Fatal("garde.txt devrait être enregistré (donc absent des modifications)")
		}
	}
	if !found {
		t.Fatalf("parasite.txt devrait figurer dans les modifications restantes : %+v", st.Changed)
	}
}

// TestTagCreateLocal : création d'un tag local léger et annoté.
// TestSquashCommit : commit-tree fabrique un commit « version propre » avec l'arbre de srcRef, sans
// toucher la branche courante, et avec/sans parent selon baseRef. Socle de la promotion v1.7.2.
func TestSquashCommit(t *testing.T) {
	dir := newRepo(t)
	write(t, dir, "a.txt", "v2\n")
	if _, err := Commit(dir, "v2"); err != nil {
		t.Fatal(err)
	}
	headBefore, _ := run(dir, LocalTimeout, "rev-parse", "HEAD")

	// Sans base : commit racine (aucun parent), arbre == HEAD.
	sha, err := SquashCommit(dir, "HEAD", "", "promotion v2")
	if err != nil {
		t.Fatalf("SquashCommit : %v", err)
	}
	if parents, _ := run(dir, LocalTimeout, "rev-list", "--parents", "-n", "1", strings.TrimSpace(sha)); len(strings.Fields(strings.TrimSpace(parents))) != 1 {
		t.Errorf("commit squash sans base devrait être racine (aucun parent) : %q", parents)
	}
	tSrc, _ := run(dir, LocalTimeout, "rev-parse", "HEAD^{tree}")
	tNew, _ := run(dir, LocalTimeout, "rev-parse", strings.TrimSpace(sha)+"^{tree}")
	if strings.TrimSpace(tSrc) != strings.TrimSpace(tNew) {
		t.Error("l'arbre du commit squash doit être celui de la source")
	}
	// La branche courante n'a pas bougé (plomberie, pas de checkout).
	if headAfter, _ := run(dir, LocalTimeout, "rev-parse", "HEAD"); strings.TrimSpace(headAfter) != strings.TrimSpace(headBefore) {
		t.Error("SquashCommit ne doit pas déplacer HEAD")
	}

	// Avec base : le commit a bien un parent.
	sha2, err := SquashCommit(dir, "HEAD", "HEAD~1", "promotion sur base")
	if err != nil {
		t.Fatalf("SquashCommit base : %v", err)
	}
	if parents, _ := run(dir, LocalTimeout, "rev-list", "--parents", "-n", "1", strings.TrimSpace(sha2)); len(strings.Fields(strings.TrimSpace(parents))) != 2 {
		t.Errorf("commit squash sur base devrait avoir 1 parent : %q", parents)
	}
	// Message vide refusé.
	if _, err := SquashCommit(dir, "HEAD", "", ""); err == nil {
		t.Error("un message vide devrait être refusé")
	}
}

// makeConflict crée un conflit de fusion non résolu (a.txt porte des marques) et renvoie le dépôt.
func makeConflict(t *testing.T) string {
	t.Helper()
	dir := newRepo(t)
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		_ = cmd.Run() // le merge sort en erreur (conflit) : on l'ignore
	}
	run("switch", "-c", "a")
	write(t, dir, "a.txt", "version A\n")
	run("commit", "-am", "A")
	run("switch", "-c", "b", "HEAD~1")
	write(t, dir, "a.txt", "version B\n")
	run("commit", "-am", "B")
	run("merge", "a") // conflit sur a.txt
	st, _ := StatusOf(dir)
	if len(st.Conflicted) == 0 {
		t.Skip("conflit non reproduit sur cet environnement git")
	}
	return dir
}

// TestCommitRefusesConflictMarkers : le garde-fou central — Commit NE DOIT JAMAIS enregistrer des
// fichiers porteurs de marques de conflit, même si « git add -A » a levé l'état « non fusionné ».
func TestCommitRefusesConflictMarkers(t *testing.T) {
	dir := makeConflict(t)
	if _, err := Commit(dir, "tentative"); !errors.Is(err, ErrConflicted) {
		t.Fatalf("Commit devrait refuser des marques de conflit (ErrConflicted), obtenu %v", err)
	}
	if _, err := Amend(dir, "tentative"); !errors.Is(err, ErrConflicted) {
		t.Fatalf("Amend devrait refuser des marques de conflit (ErrConflicted), obtenu %v", err)
	}
}

// TestCommitRefusesResidualMarkers : même hors fusion active, un fichier contenant des marques
// résiduelles ne doit pas être enregistré (détection par `git diff --cached --check`).
func TestCommitRefusesResidualMarkers(t *testing.T) {
	dir := newRepo(t)
	write(t, dir, "a.txt", "début\n<<<<<<< HEAD\nmien\n=======\nleur\n>>>>>>> autre\nfin\n")
	if _, err := Commit(dir, "marques résiduelles"); !errors.Is(err, ErrConflicted) {
		t.Fatalf("Commit devrait refuser des marques résiduelles (ErrConflicted), obtenu %v", err)
	}
}

func TestTagCreateLocal(t *testing.T) {
	dir := newRepo(t)
	if _, err := TagCreate(dir, "v0", ""); err != nil {
		t.Fatalf("TagCreate léger : %v", err)
	}
	if _, err := TagCreate(dir, "v1", "première"); err != nil {
		t.Fatalf("TagCreate annoté : %v", err)
	}
	if _, err := TagCreate(dir, "--bad", ""); err == nil {
		t.Error("un nom de tag façon option devrait être refusé")
	}
	out, err := run(dir, LocalTimeout, "tag")
	if err != nil || !strings.Contains(out, "v0") || !strings.Contains(out, "v1") {
		t.Fatalf("les tags v0/v1 devraient exister : %q (%v)", out, err)
	}
}
