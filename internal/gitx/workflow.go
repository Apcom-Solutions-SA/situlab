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

// GlobalCredentialHelpers renvoie les gestionnaires d'identifiants git configurés hors du dépôt,
// dans les scopes --system ET --global (ex. « manager », « wincred »). Le scope système est
// essentiel sous Windows : Git Credential Manager s'y installe par défaut, et un diagnostic qui ne
// lirait que --global le manquerait. Utile au diagnostic : si un tel helper (ou une clé SSH) existe,
// la confirmation par PIN de SituLab ne suffit plus à empêcher un accès git depuis le poste.
func GlobalCredentialHelpers() []string {
	var hs []string
	seen := map[string]bool{}
	// Chaque scope est interrogé séparément : s'il est absent, git sort en erreur et on l'ignore.
	// Un scope explicite (--system/--global) ne lit que le FICHIER correspondant : le
	// « -c credential.helper= » du durcissement (valeur de niveau commande) en est d'office exclu,
	// il n'y a donc pas de valeur vide à filtrer de ce côté.
	for _, scope := range []string{"--system", "--global"} {
		out, err := run("", LocalTimeout, "config", scope, "--get-all", "credential.helper")
		if err != nil {
			continue
		}
		for _, l := range strings.Split(out, "\n") {
			if l = strings.TrimSpace(l); l != "" && !seen[l] {
				seen[l] = true
				hs = append(hs, l)
			}
		}
	}
	return hs
}
