// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package main

import (
	"situlab/internal/core"
	"situlab/internal/gitx"
)

// UICacheLoad / UICacheSave : cache d'interface persistant (1.18.3), stale-while-revalidate
// inter-sessions. Best-effort, local, hors réseau — pas une mutation (ni PIN, ni isWriteOp). Sert
// à peindre d'abord le dernier état connu au lieu d'une boîte vide. Jamais de CORPUS ni de secret
// (garde-fou dans core). Surface GUI uniquement : l'agent (CLI/MCP) n'en a pas besoin.
func (a *App) UICacheLoad(key string) (string, error) {
	if a.svc == nil {
		return "", nil
	}
	return a.svc.UICacheLoad(key)
}

func (a *App) UICacheSave(key, value string) error {
	if a.svc == nil {
		return nil
	}
	return a.svc.UICacheSave(key, value)
}

// LocalTreeAt / LocalTreeDates : arbre du clone local pour l'explorateur (1.18.3) — affichage
// instantané hors-ligne, dates du dernier commit remplies en arrière-plan (shimmer en attendant).
func (a *App) LocalTreeAt(dir, subpath string) ([]gitx.TreeEntry, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.LocalTreeAt(dir, subpath)
}

func (a *App) LocalTreeDates(dir string, paths []string) (map[string]string, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.LocalTreeDates(dir, paths), nil
}

// LocalTreeTotals : tailles agrégées des dossiers + total du sous-arbre (poids total du dépôt à la racine).
func (a *App) LocalTreeTotals(dir, subpath string) (gitx.TreeTotals, error) {
	if err := a.ready(); err != nil {
		return gitx.TreeTotals{}, err
	}
	return a.svc.LocalTreeTotals(dir, subpath)
}

// LocalDetail renvoie l'état détaillé d'un dépôt local.
func (a *App) LocalDetail(path string) (*gitx.Detail, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.LocalDetail(path)
}

// BranchMatrix renvoie l'état par branche d'un dépôt local (ce poste vs en ligne).
func (a *App) BranchMatrix(path string) (*gitx.BranchMatrix, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.BranchMatrix(path)
}

// --- Signature de garantie « SituLab approved » ---

// AddLocalRepo enregistre un dépôt local.
func (a *App) AddLocalRepo(path string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.AddLocalRepo(path)
}

// Push pousse un dépôt local (PIN obligatoire).
func (a *App) Push(instance, path, pin string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	out, err := a.svc.Push(instance, path, pin)
	if err == nil {
		a.afterWrite()
	}
	return out, err
}

// Synchronize est le geste unique « Synchroniser » : récupérer → enregistrer → envoyer, sous une
// seule confirmation PIN (v1.3).
// PublishTag publie un tag LOCAL (version) vers l'atelier (le contenu étant déjà poussé).
func (a *App) PublishTag(instance, owner, repo, localPath, tag, pin string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.lockedErr(a.svc.PublishTag(instance, owner, repo, localPath, tag, pin))
}

func (a *App) Synchronize(instance, path, message, pin string) (*core.SyncResult, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	res, err := a.svc.Synchronize(instance, path, message, pin)
	if err == nil {
		a.afterWrite()
	}
	return res, err
}

// Fetch / Pull / Commit / Diff : opérations de la boucle de travail locale.
func (a *App) Fetch(instance, path string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.svc.Fetch(instance, path)
}

func (a *App) Pull(instance, path string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.svc.Pull(instance, path)
}

// FetchAllLocal récupère en arrière-plan les nouveautés distantes de tous les dépôts suivis
// (best-effort, sans PIN). La GUI l'appelle périodiquement pour garder l'état à jour.
func (a *App) FetchAllLocal() (int, error) {
	if err := a.ready(); err != nil {
		return 0, err
	}
	return a.svc.FetchAllLocal()
}

func (a *App) Commit(path, message string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.svc.Commit(path, message)
}

func (a *App) Diff(path string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.svc.Diff(path)
}

func (a *App) RemoveLocalRepo(path string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.RemoveLocalRepo(path)
}

// SuggestCloneBase propose un dossier parent pour un nouveau clone (voir core.SuggestCloneBase) :
// le répertoire qui regroupe déjà le plus de clones suivis. Vide si aucun — la fenêtre retombe sur
// le choix manuel. Lecture seule.
func (a *App) SuggestCloneBase() string {
	if err := a.ready(); err != nil {
		return ""
	}
	return a.svc.SuggestCloneBase()
}

// Clone copie un dépôt distant sur le poste (action « Copier sur mon poste »).
func (a *App) Clone(instance, owner, repo, dir string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.svc.Clone(instance, owner, repo, dir)
}

// git local (modale « Git local ») : créer/changer de branche, mettre de côté, annuler. Pas de
// PIN (local), mais refusés en lecture seule par le cœur (assertWritable).
func (a *App) GitCreateBranch(path, name string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.svc.GitCreateBranch(path, name)
}

func (a *App) GitSwitchBranch(path, name string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.svc.GitSwitchBranch(path, name)
}

func (a *App) GitStashSave(path, message string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.svc.GitStashSave(path, message)
}

func (a *App) GitStashPop(path string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.svc.GitStashPop(path)
}

func (a *App) GitDiscard(path, file string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.svc.GitDiscard(path, file)
}

// GitMerge fusionne ref dans la branche courante (local, pas de PIN). Exposé au panneau git des Projets.
func (a *App) GitMerge(path, ref string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.svc.GitMerge(path, ref)
}

func (a *App) GitMergeAbort(path string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.svc.GitMergeAbort(path)
}

// GitRebase rejoue la branche courante au sommet de upstream (local, pas de PIN). Exposé au panneau
// git des Projets : réaligner une branche de travail sur l'atelier à jour (reprise après absence).
func (a *App) GitRebase(path, upstream string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.svc.GitRebase(path, upstream)
}

func (a *App) GitRebaseContinue(path string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.svc.GitRebaseContinue(path)
}

func (a *App) GitRebaseAbort(path string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.svc.GitRebaseAbort(path)
}

// GitDeleteBranch supprime une branche locale. force=false laisse git refuser une branche non
// fusionnée (le front prévient alors et re-propose avec force=true).
func (a *App) GitDeleteBranch(path, name string, force bool) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.svc.GitDeleteBranch(path, name, force)
}

// Worktrees : plusieurs arbres de travail (branches) depuis un seul dépôt (local, pas de PIN).
func (a *App) GitWorktrees(path string) ([]gitx.Worktree, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.GitWorktrees(path)
}

func (a *App) GitWorktreeAdd(path, wtPath, branch string, createBranch bool) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.svc.GitWorktreeAdd(path, wtPath, branch, createBranch)
}

func (a *App) GitWorktreeRemove(path, wtPath string, force bool) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.svc.GitWorktreeRemove(path, wtPath, force)
}

func (a *App) GitWorktreePrune(path string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.svc.GitWorktreePrune(path)
}
