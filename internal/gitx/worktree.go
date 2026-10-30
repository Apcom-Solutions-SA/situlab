// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package gitx

import (
	"fmt"
	"strings"
)

// --- Worktrees (arbres de travail multiples) ---
//
// Un worktree permet de travailler sur PLUSIEURS branches EN MÊME TEMPS depuis UN SEUL dépôt : chaque
// worktree est un dossier distinct qui partage le même historique (.git). Pratique pour des sessions
// parallèles sur le même poste (chacune sa branche, son dossier) sans re-cloner. À distinguer d'un
// CLONE — copie indépendante, avec son propre .git et son propre accès réseau.

// Worktree décrit un arbre de travail rattaché à un dépôt.
type Worktree struct {
	Path     string `json:"path"`
	Head     string `json:"head"`     // SHA courant
	Branch   string `json:"branch"`   // branche extraite (vide si HEAD détaché)
	Bare     bool   `json:"bare"`     // dépôt nu (sans arbre de travail)
	Detached bool   `json:"detached"` // HEAD détaché (aucune branche)
	Locked   bool   `json:"locked"`   // worktree verrouillé (git worktree lock)
	Main     bool   `json:"main"`     // worktree principal (le premier de la liste)
}

// WorktreeList énumère les worktrees d'un dépôt (git worktree list --porcelain). Lecture, aucun réseau.
func WorktreeList(dir string) ([]Worktree, error) {
	out, err := run(dir, LocalTimeout, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	var list []Worktree
	var cur *Worktree
	flush := func() {
		if cur != nil {
			list = append(list, *cur)
			cur = nil
		}
	}
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimRight(raw, "\r")
		if line == "" {
			flush()
			continue
		}
		switch {
		case strings.HasPrefix(line, "worktree "):
			flush()
			cur = &Worktree{Path: strings.TrimPrefix(line, "worktree "), Main: len(list) == 0}
		case cur == nil:
			// ligne orpheline avant le premier « worktree » : ignorée
		case strings.HasPrefix(line, "HEAD "):
			cur.Head = strings.TrimPrefix(line, "HEAD ")
		case strings.HasPrefix(line, "branch "):
			cur.Branch = strings.TrimPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/")
		case line == "bare":
			cur.Bare = true
		case line == "detached":
			cur.Detached = true
		case strings.HasPrefix(line, "locked"):
			cur.Locked = true
		}
	}
	flush()
	return list, nil
}

// safeWorktreePath refuse un chemin dangereux (vide, ou commençant par « - » qui serait pris pour une
// option). Les chemins absolus Windows (C:\...) sont légitimes.
func safeWorktreePath(p string) error {
	p = strings.TrimSpace(p)
	if p == "" {
		return fmt.Errorf("chemin de worktree vide")
	}
	if strings.HasPrefix(p, "-") {
		return fmt.Errorf("chemin de worktree invalide : %q", p)
	}
	return nil
}

// WorktreeAdd crée un nouvel arbre de travail dans path. createBranch=true → crée une NOUVELLE branche
// `branch` à partir de HEAD (git worktree add -b branch path). Sinon extrait la branche/commit existant
// `branch` dans path (git worktree add path branch) ; branch vide → HEAD courant. Local, aucun réseau.
// git refuse de lui-même d'extraire une branche déjà ouverte dans un autre worktree.
func WorktreeAdd(dir, path, branch string, createBranch bool) (string, error) {
	if err := safeWorktreePath(path); err != nil {
		return "", err
	}
	if createBranch {
		if err := safeRef(branch); err != nil {
			return "", err
		}
		return run(dir, LocalTimeout, "worktree", "add", "-b", branch, path)
	}
	if strings.TrimSpace(branch) != "" {
		if err := safeRevision(branch); err != nil {
			return "", err
		}
		return run(dir, LocalTimeout, "worktree", "add", path, branch)
	}
	return run(dir, LocalTimeout, "worktree", "add", path)
}

// WorktreeRemove retire un arbre de travail (git worktree remove) et supprime son dossier. force=true
// retire même si l'arbre a des modifications non enregistrées (git refuse sinon). Local.
func WorktreeRemove(dir, path string, force bool) (string, error) {
	if err := safeWorktreePath(path); err != nil {
		return "", err
	}
	if force {
		return run(dir, LocalTimeout, "worktree", "remove", "--force", path)
	}
	return run(dir, LocalTimeout, "worktree", "remove", path)
}

// WorktreePrune nettoie les références des worktrees dont le dossier a disparu (supprimé à la main).
func WorktreePrune(dir string) (string, error) {
	return run(dir, LocalTimeout, "worktree", "prune")
}
