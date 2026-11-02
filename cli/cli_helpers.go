// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"situlab/internal/core"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func codeFor(err error) int {
	switch {
	case errors.Is(err, core.ErrNoInstance):
		return 2
	case errors.Is(err, core.ErrPINNeeded), errors.Is(err, core.ErrPINLocked), errors.Is(err, core.ErrLocked), errors.Is(err, core.ErrPINTimeout):
		return 3
	case errors.Is(err, core.ErrNoGit):
		return 4
	default:
		return 1
	}
}

// renderError émet une erreur structurée en JSON (ou un texte lisible) sur w.
func renderError(w *os.File, err error) {
	if flagOutput == "json" {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(map[string]any{"error": map[string]string{"message": err.Error()}})
		return
	}
	fmt.Fprintln(w, "erreur : "+err.Error())
}

// emit affiche un résultat en JSON ou en texte (via textFn). Renvoie l'erreur
// d'encodage éventuelle pour qu'un échec de sérialisation ne passe pas en code 0.
func emit(cmd *cobra.Command, data any, textFn func()) error {
	if flagOutput == "json" {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(data)
	}
	textFn()
	return nil
}

// valOr renvoie v s'il est non vide, sinon def.
func valOr(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

// looksLikeAuthFailure indique si l'erreur ressemble à un refus d'authentification Forgejo
// (identifiant/mot de passe/2FA), auquel cas proposer un code 2FA a du sens. Les erreurs
// locales (doublon, session verrouillée, URL invalide, champ manquant) ne matchent pas.
func looksLikeAuthFailure(err error) bool {
	m := strings.ToLower(err.Error())
	return strings.Contains(m, "mot de passe") || strings.Contains(m, "2fa")
}

func splitRepo(s string) (string, string, error) {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", errors.New("format attendu : owner/repo")
	}
	return parts[0], parts[1], nil
}

// splitRepoNum lit les deux arguments communs « owner/repo numéro » des commandes issue/PR. Factorise
// le bloc splitRepo + Atoi recopié sur chaque sous-commande ExactArgs(2).
func splitRepoNum(args []string) (owner, repo string, num int, err error) {
	owner, repo, err = splitRepo(args[0])
	if err != nil {
		return "", "", 0, err
	}
	num, err = strconv.Atoi(args[1])
	if err != nil {
		return "", "", 0, fmt.Errorf("numéro invalide : %s", args[1])
	}
	return owner, repo, num, nil
}

// warnSecretFlag avertit (stderr) qu'un secret passé en OPTION de ligne de commande reste visible dans
// la liste des processus (ps / Gestionnaire des tâches) et l'historique du shell — à éviter.
func warnSecretFlag(what string) {
	fmt.Fprintf(os.Stderr, "Attention : %s passé en option reste visible dans la liste des processus et l'historique du shell ; laissez l'option vide pour une saisie masquée.\n", what)
}

// secretOrPrompt renvoie le secret fourni par drapeau (en avertissant qu'il est exposé), sinon le
// demande au terminal, masqué. Évite d'imposer un secret sur la ligne de commande.
func secretOrPrompt(flagVal, what, label string) (string, error) {
	if flagVal != "" {
		warnSecretFlag(what)
		return flagVal, nil
	}
	return promptSecret(label)
}

func promptSecret(label string) (string, error) {
	fmt.Fprint(os.Stderr, label)
	if term.IsTerminal(int(os.Stdin.Fd())) {
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(b)), nil
	}
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}
