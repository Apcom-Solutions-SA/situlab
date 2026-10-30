// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package gitx

import (
	"fmt"
	"strings"
)

// safeRef refuse un nom de branche dangereux (vide, commençant par « - » façon option,
// ou contenant espaces/caractères de contrôle). Garde-fou contre l'injection d'options.
func safeRef(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("nom de branche vide")
	}
	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("nom de branche invalide : %q", name)
	}
	if strings.ContainsAny(name, " \t\n\r\\~^:?*[") {
		return fmt.Errorf("nom de branche invalide : %q", name)
	}
	return nil
}

// CreateBranch crée une branche locale et bascule dessus (git switch -c).
func CreateBranch(dir, name string) (string, error) {
	if err := safeRef(name); err != nil {
		return "", err
	}
	return run(dir, LocalTimeout, "switch", "-c", name)
}

// SwitchBranch bascule sur une branche existante (git switch). Échoue proprement si des
// modifications non enregistrées empêchent le changement (git ne force rien).
func SwitchBranch(dir, name string) (string, error) {
	if err := safeRef(name); err != nil {
		return "", err
	}
	return run(dir, LocalTimeout, "switch", name)
}

// DiscardChanges annule les modifications locales NON ENREGISTRÉES (index + arbre de travail)
// des fichiers suivis. path vide = tout le dépôt ; sinon un chemin précis. DESTRUCTIF : les
// changements non commités sont perdus. N'affecte ni l'historique ni les fichiers non suivis.
func DiscardChanges(dir, path string) (string, error) {
	pathspec := ":/" // cas « tout le dépôt » : pathspec interne contrôlé (pas d'entrée appelant)
	if p := strings.TrimSpace(path); p != "" {
		if err := safePathspec(p); err != nil { // refuse la magie de pathspec (:/ :(glob) …) et le préfixe « - »
			return "", err
		}
		pathspec = p
	}
	return run(dir, LocalTimeout, "restore", "--staged", "--worktree", "--", pathspec)
}

// Log renvoie l'historique local récent (structuré). limit borne le nombre de commits.
func Log(dir string, limit int) []LocalCommit {
	if limit <= 0 {
		limit = 30
	}
	return parseLog(dir, "", limit)
}

// StashSave met de côté les modifications locales (git stash push). message optionnel.
func StashSave(dir, message string) (string, error) {
	args := []string{"stash", "push"}
	if m := strings.TrimSpace(message); m != "" {
		args = append(args, "-m", m)
	}
	return run(dir, LocalTimeout, args...)
}

// StashList liste les remises de côté (git stash list).
func StashList(dir string) (string, error) {
	return run(dir, LocalTimeout, "stash", "list")
}

// StashPop réapplique et retire la dernière remise de côté (git stash pop).
func StashPop(dir string) (string, error) {
	return run(dir, LocalTimeout, "stash", "pop")
}
