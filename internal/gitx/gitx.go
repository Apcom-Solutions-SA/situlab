// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

// Package gitx enveloppe le git du système (os/exec). SituLab ne réimplémente pas
// git : il le pilote. git doit être présent sur la machine (vérifié par `situlab doctor`).
//
// Toutes les invocations passent par run(), qui applique un durcissement systématique :
//   - timeout borné par opération (pas de gel permanent de l'interface) ;
//   - GIT_TERMINAL_PROMPT=0 et credential.interactive=never : échec propre plutôt
//     qu'une invite d'identifiants invisible en mode fenêtre ;
//   - hooks, fsmonitor et credential.helper désactivés, transports ext/file interdits :
//     neutralise les canaux d'exécution de code d'un .git/config de dépôt non fiable
//     (ce n'est pas une garantie absolue d'isolation, mais ferme les vecteurs connus) ;
//   - LC_ALL=C : messages git en anglais, pour un parsing fiable quelle que soit la locale ;
//   - core.quotePath=false : les chemins accentués (français) s'affichent lisiblement ;
//   - le token éventuellement présent dans une URL de stderr est masqué avant exposition.
package gitx

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Timeouts par classe d'opération.
const (
	LocalTimeout = 20 * time.Second  // status, describe, log… (local, rapide)
	NetTimeout   = 180 * time.Second // clone, push, fetch… (réseau)
)

const maxChanged = 100 // plafond de fichiers modifiés listés (au-delà : tronqué)

// Erreurs typées, testables côté appelant via errors.Is.
var (
	ErrGitMissing = errors.New("git n'est pas installé")
	ErrTimeout    = errors.New("l'opération git a expiré")
	// ErrConflicted : un enregistrement a été refusé parce que des fichiers mis en index portent
	// des marques de conflit. Le cœur le remappe sur son message guidé (core.ErrConflict).
	ErrConflicted = errors.New("des fichiers en conflit ne peuvent pas être enregistrés")
)

// stagedHasConflictMarkers détecte des marqueurs de conflit (<<<<<<< ======= >>>>>>>) dans ce qui
// est MIS EN INDEX, via `git diff --cached --check` (qui signale « leftover conflict marker »). Plus
// robuste qu'un contrôle d'état « non fusionné » : attrape aussi des marques résiduelles dans un
// arbre faussement « résolu » (ex. après un `git add -A` qui lève l'état unmerged). --no-ext-diff /
// --no-textconv : ne jamais exécuter de pilote externe d'un dépôt non fiable.
func stagedHasConflictMarkers(dir string) bool {
	out, _ := run(dir, LocalTimeout, "diff", "--cached", "--check", "--no-ext-diff", "--no-textconv")
	return strings.Contains(out, "conflict marker")
}

// redactCreds masque un éventuel couple identifiant:token dans une URL (https://x:y@host).
var redactCreds = regexp.MustCompile(`(://)[^@/\s]+@`)

func scrub(s string) string { return redactCreds.ReplaceAllString(s, "$1***@") }

// Available indique si git est présent dans le PATH.
func Available() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

// Version renvoie la version de git, ou une erreur si absent.
func Version() (string, error) {
	out, err := run("", LocalTimeout, "--version")
	return strings.TrimSpace(out), err
}

// Status décrit l'état synthétique d'un dépôt local.
type Status struct {
	Path         string   `json:"path"`
	IsRepo       bool     `json:"is_repo"`
	Branch       string   `json:"branch"`
	Detached     bool     `json:"detached"`
	Upstream     string   `json:"upstream"`
	Ahead        int      `json:"ahead"`
	Behind       int      `json:"behind"`
	Clean        bool     `json:"clean"`
	Changed      []string `json:"changed"`
	ChangedTrunc bool     `json:"changed_truncated"`
	Conflicted   []string `json:"conflicted"` // fichiers non fusionnés (marques de conflit) — jamais à enregistrer tels quels
	Remote       string   `json:"remote"`
	RemoteName   string   `json:"remote_name"`
}

// run exécute git avec durcissement, timeout et capture. Renvoie stdout et une erreur
// typée/épurée construite à partir de stderr.
func run(dir string, timeout time.Duration, args ...string) (string, error) {
	return runC(dir, timeout, nil, args...)
}

// runC est comme run mais injecte une configuration SECRÈTE (ex. l'en-tête d'authentification
// porteur du jeton) via l'ENVIRONNEMENT (GIT_CONFIG_*), jamais par la ligne de commande : les
// arguments d'un processus sont lisibles par les autres processus du poste, pas son
// environnement. Le durcissement `-c` (non secret) reste en arguments.
func runC(dir string, timeout time.Duration, extraEnv []string, args ...string) (string, error) {
	if !Available() {
		return "", ErrGitMissing
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	full := append([]string{
		"-c", "core.quotePath=false",
		"-c", "core.hooksPath=", // neutralise les hooks d'un dépôt non fiable
		"-c", "core.fsmonitor=", // neutralise un fsmonitor piégé
		"-c", "credential.helper=", // réinitialise la liste : neutralise un helper du .git/config local (RCE)
		"-c", "credential.interactive=never",
		"-c", "protocol.ext.allow=never", // bloque les transports ext:: (exécution de commande)
		"-c", "protocol.file.allow=never", // bloque file:// (sous-modules malveillants)
		"-c", "diff.external=", // neutralise un pilote de diff externe (dépôt piégé → exécution)
		"-c", "commit.gpgsign=false", // SituLab gère sa propre attestation ; ne jamais déclencher la signature gpg/ssh du poste (en-tête gpgsig parasite)
	}, args...)

	cmd := exec.CommandContext(ctx, "git", full...)
	if dir != "" {
		cmd.Dir = dir
	}
	// LC_ALL=C : messages d'erreur en anglais (parsing fiable sur poste non anglais).
	cmd.Env = append(cmd.Environ(), "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never", "LC_ALL=C")
	cmd.Env = append(cmd.Env, extraEnv...) // en-tête d'auth (jeton) : environnement, pas argv
	hideProcess(cmd)

	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return out.String(), ErrTimeout
	}
	if err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		return out.String(), errors.New(scrub(msg))
	}
	return out.String(), nil
}

// StatusOf lit l'état git d'un dossier via `git status --porcelain=v2 --branch`.
// Distingue « pas un dépôt » d'une vraie panne (droits, chemin disparu).
func StatusOf(dir string) (*Status, error) {
	s := &Status{Path: dir}
	if _, err := run(dir, LocalTimeout, "rev-parse", "--is-inside-work-tree"); err != nil {
		// git renvoie un code 128 avec un message « not a git repository » attendu.
		if strings.Contains(strings.ToLower(err.Error()), "not a git repository") {
			s.IsRepo = false
			return s, nil
		}
		return s, err // panne réelle : on la remonte au lieu de mentir « pas un dépôt »
	}
	s.IsRepo = true
	out, err := run(dir, LocalTimeout, "status", "--porcelain=v2", "--branch")
	if err != nil {
		return s, err
	}
	parseStatus(out, s)
	if name, remote := detectRemote(dir); name != "" {
		s.RemoteName, s.Remote = name, remote
	}
	return s, nil
}

// parseStatus remplit un Status à partir de la sortie `status --porcelain=v2 --branch`.
// Fonction pure (testable sans git).
func parseStatus(out string, s *Status) {
	changed := 0
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		switch {
		case strings.HasPrefix(line, "# branch.head "):
			head := strings.TrimPrefix(line, "# branch.head ")
			if head == "(detached)" {
				s.Detached = true
			} else {
				s.Branch = head
			}
		case strings.HasPrefix(line, "# branch.upstream "):
			s.Upstream = strings.TrimPrefix(line, "# branch.upstream ")
		case strings.HasPrefix(line, "# branch.ab "):
			parts := strings.Fields(strings.TrimPrefix(line, "# branch.ab "))
			if len(parts) == 2 {
				s.Ahead, _ = strconv.Atoi(strings.TrimPrefix(parts[0], "+"))
				s.Behind, _ = strconv.Atoi(strings.TrimPrefix(parts[1], "-"))
			}
		case strings.HasPrefix(line, "1 "), strings.HasPrefix(line, "2 "),
			strings.HasPrefix(line, "u "), strings.HasPrefix(line, "? "):
			changed++
			if len(s.Changed) < maxChanged {
				s.Changed = append(s.Changed, changedPath(line))
			}
			if strings.HasPrefix(line, "u ") { // entrée non fusionnée (conflit)
				s.Conflicted = append(s.Conflicted, changedPath(line))
			}
		}
	}
	s.ChangedTrunc = changed > len(s.Changed)
	s.Clean = changed == 0
}

// changedPath extrait le chemin d'une ligne porcelain v2, en gérant les renommages
// (type « 2 », chemin et origine séparés par une TABULATION) et les champs multiples.
func changedPath(line string) string {
	if strings.HasPrefix(line, "? ") {
		return strings.TrimPrefix(line, "? ")
	}
	// Nombre de champs AVANT le chemin selon le type d'enregistrement porcelain v2 :
	// « 1 » = 8 ; « 2 » (renommage) = 9 puis <chemin>\t<origine> ; « u » (non fusionné) = 10.
	nFields := 8
	switch {
	case strings.HasPrefix(line, "2 "):
		nFields = 9
	case strings.HasPrefix(line, "u "):
		nFields = 10
	}
	parts := strings.SplitN(line, " ", nFields+1)
	if len(parts) <= nFields {
		return line
	}
	path := parts[nFields]
	if i := strings.IndexByte(path, '\t'); i >= 0 { // renommage : on garde la destination
		path = path[:i]
	}
	return path
}

// detectRemote renvoie le nom et l'URL du distant de suivi de la branche courante,
// à défaut « origin », à défaut le premier distant configuré.
func detectRemote(dir string) (name, url string) {
	if head, err := run(dir, LocalTimeout, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}"); err == nil {
		if i := strings.IndexByte(strings.TrimSpace(head), '/'); i > 0 {
			name = strings.TrimSpace(head)[:i]
		}
	}
	if name == "" {
		if _, err := run(dir, LocalTimeout, "remote", "get-url", "origin"); err == nil {
			name = "origin"
		} else if list, err := run(dir, LocalTimeout, "remote"); err == nil {
			if first := strings.SplitN(strings.TrimSpace(list), "\n", 2)[0]; first != "" {
				name = strings.TrimSpace(first)
			}
		}
	}
	if name == "" {
		return "", ""
	}
	if u, err := run(dir, LocalTimeout, "remote", "get-url", name); err == nil {
		url = scrub(strings.TrimSpace(u)) // masque un éventuel identifiant intégré (https://user:token@host)
	}
	return name, url
}

// authCfg construit l'authentification HTTPS (en-tête Authorization) sous forme de VARIABLES
// D'ENVIRONNEMENT git (GIT_CONFIG_COUNT/KEY_0/VALUE_0), de sorte que le jeton n'apparaisse
// JAMAIS dans la ligne de commande (lisible par les autres processus). Vide si pas de token.
//
// Deux schémas selon le fournisseur :
//   - Forgejo/Gitea : « Authorization: token <token> » (éprouvé) ;
//   - GitHub : Basic « x-access-token:<PAT> » — méthode documentée par GitHub pour un
//     Personal Access Token en HTTPS (le schéma « token » n'est pas garanti côté git).
//
// baseURL (l'URL de l'instance) scope l'en-tête à CET hôte : git n'enverra le jeton qu'aux URL
// commençant par baseURL, jamais à la cible d'une éventuelle redirection cross-host.
func authCfg(token string, github bool, baseURL string) []string {
	if token == "" {
		return nil
	}
	header := "Authorization: token " + token
	if github {
		b64 := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + token))
		header = "Authorization: Basic " + b64
	}
	// Scoping STRUCTUREL : l'en-tête d'auth n'est posé que pour l'hôte de l'instance
	// (http.<baseURL>.extraHeader). Sans baseURL, on refuse d'injecter un en-tête GLOBAL non scopé
	// (il partirait vers n'importe quel hôte) : mieux vaut pas d'auth qu'un jeton non borné (le
	// dépôt public fonctionnera, le privé échouera proprement).
	b := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if b == "" {
		return nil
	}
	return []string{
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=http." + b + ".extraHeader",
		"GIT_CONFIG_VALUE_0=" + header,
	}
}

// Clone clone une URL dans un dossier cible (token injecté pour un dépôt privé HTTPS).
// Le `--` protège contre une URL commençant par `-` (injection d'option).
// baseURL (l'URL de l'instance) scope l'en-tête d'authentification à cet hôte.
func Clone(cloneURL, dir, token string, github bool, baseURL string) (string, error) {
	return runC("", NetTimeout, authCfg(token, github, baseURL), "clone", "--", cloneURL, dir)
}

// Fetch récupère les références distantes sans toucher à l'arbre de travail.
func Fetch(dir, token string, github bool, baseURL string) (string, error) {
	return runC(dir, NetTimeout, authCfg(token, github, baseURL), "fetch", "--prune")
}

// Pull met à jour la branche locale en fast-forward seulement (jamais de merge
// surprise ni de réécriture d'historique). Si autostash est vrai, les modifications en
// cours sont mises de côté puis réappliquées automatiquement (--autostash), ce qui permet
// de récupérer sans devoir enregistrer d'abord.
func Pull(dir, token string, github bool, baseURL string, autostash bool) (string, error) {
	args := []string{"pull", "--ff-only"}
	if autostash {
		args = append(args, "--autostash")
	}
	return runC(dir, NetTimeout, authCfg(token, github, baseURL), args...)
}

// HasRemote indique si le dépôt a au moins un distant configuré (git remote). Permet de
// renvoyer une erreur claire plutôt qu'une sortie git brute (« No remote repository specified »).
func HasRemote(dir string) bool {
	out, err := run(dir, LocalTimeout, "remote")
	return err == nil && strings.TrimSpace(out) != ""
}

// remoteName renvoie le distant à viser pour un envoi : le distant de suivi s'il existe, sinon « origin ».
func remoteName(dir string) string {
	if name, _ := detectRemote(dir); name != "" {
		return name
	}
	return "origin"
}

// Push pousse la branche courante vers le distant (token injecté si fourni). Si la branche
// n'a pas d'« upstream » (cas d'une branche fraîchement créée), on le définit automatiquement
// (push -u <remote> HEAD) pour que pousser une nouvelle branche fonctionne du premier coup.
func Push(dir, token string, github bool, baseURL string) (string, error) {
	args := []string{"push"}
	if st, err := StatusOf(dir); err == nil && st.IsRepo && st.Upstream == "" {
		args = []string{"push", "-u", remoteName(dir), "HEAD"}
	}
	return runC(dir, NetTimeout, authCfg(token, github, baseURL), args...)
}

// Commit enregistre toutes les modifications de l'arbre de travail avec un message.
// (La zone d'index est gérée pour l'utilisateur : « enregistrer un point ».)
func Commit(dir, message string) (string, error) {
	if strings.TrimSpace(message) == "" {
		return "", errors.New("le message ne peut pas être vide")
	}
	if _, err := run(dir, LocalTimeout, "add", "-A"); err != nil {
		return "", err
	}
	// Garde-fou d'invariant : ne JAMAIS enregistrer de marques de conflit. `git add -A` lève
	// l'état « non fusionné » (la protection native de git est alors neutralisée), donc on
	// revérifie explicitement l'index avant de committer.
	if stagedHasConflictMarkers(dir) {
		return "", ErrConflicted
	}
	return run(dir, LocalTimeout, "commit", "-m", message)
}

// Diff renvoie le diff unifié des modifications non enregistrées de l'arbre de travail
// (fichiers suivis). Tronqué à une taille raisonnable pour l'affichage.
func Diff(dir string) (string, error) {
	// --no-ext-diff / --no-textconv : ne jamais exécuter de pilote de diff/textconv configuré
	// dans un dépôt non fiable (exécution de commande). Complète « diff.external= » global.
	out, err := run(dir, LocalTimeout, "diff", "--no-ext-diff", "--no-textconv", "HEAD", "--")
	if err != nil {
		return "", err
	}
	const max = 200 * 1024
	if len(out) > max {
		out = out[:max] + "\n… (diff tronqué)"
	}
	return out, nil
}
