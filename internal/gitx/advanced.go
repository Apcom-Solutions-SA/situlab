// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package gitx

import (
	"fmt"
	"strconv"
	"strings"
)

// safeRevision refuse une référence/commit dangereux pour l'injection d'options (vide,
// commençant par « - », ou contenant un espace / caractère de contrôle). Contrairement à
// safeRef (noms de branche), elle AUTORISE les caractères de révision légitimes (~ ^ : / @ { })
// car on cible ici des commit-ish comme « HEAD~1 », « origin/main » ou « v1.2^ ».
func safeRevision(rev string) error {
	rev = strings.TrimSpace(rev)
	if rev == "" {
		return fmt.Errorf("référence vide")
	}
	if strings.HasPrefix(rev, "-") {
		return fmt.Errorf("référence invalide : %q", rev)
	}
	if strings.ContainsAny(rev, " \t\n\r") {
		return fmt.Errorf("référence invalide : %q", rev)
	}
	return nil
}

// safePathspec refuse un chemin dangereux pour l'injection d'options (vide ou commençant
// par « - »). L'appelant protège en plus avec le séparateur « -- ».
func safePathspec(p string) error {
	p = strings.TrimSpace(p)
	if p == "" {
		return fmt.Errorf("chemin vide")
	}
	if strings.HasPrefix(p, "-") {
		return fmt.Errorf("chemin invalide : %q", p)
	}
	// Refuse la « magie de pathspec » git (préfixe « : » : « :/ », « :(glob) », « :(exclude) »…).
	// Elle reste interprétée MÊME après le séparateur « -- » et permettrait à CommitPaths
	// d'enregistrer bien plus que les chemins annoncés (contournement du staging sélectif).
	if strings.HasPrefix(p, ":") {
		return fmt.Errorf("chemin invalide (motif git non autorisé) : %q", p)
	}
	return nil
}

// --- Réunir des branches (local, aucun accès distant) ---

// Merge fusionne ref dans la branche courante (git merge --no-edit). En cas de conflit, git
// sort en erreur et laisse l'arbre en état non fusionné : l'appelant détecte le conflit via
// StatusOf (Conflicted) et guide l'utilisateur (résoudre puis Commit, ou MergeAbort).
func Merge(dir, ref string) (string, error) {
	if err := safeRevision(ref); err != nil {
		return "", err
	}
	return run(dir, LocalTimeout, "merge", "--no-edit", ref)
}

// MergeAbort annule une fusion en cours et restaure l'état d'avant (git merge --abort).
func MergeAbort(dir string) (string, error) {
	return run(dir, LocalTimeout, "merge", "--abort")
}

// Rebase rejoue la branche courante au sommet de upstream (git rebase). Purement local :
// aucune connexion réseau (on rejoue sur des refs déjà présentes). Conflit éventuel → l'arbre
// reste en pause, l'appelant guide (résoudre puis RebaseContinue, ou RebaseAbort).
func Rebase(dir, upstream string) (string, error) {
	if err := safeRevision(upstream); err != nil {
		return "", err
	}
	return run(dir, LocalTimeout, "rebase", upstream)
}

// RebaseContinue poursuit un rebase après résolution des conflits (git rebase --continue).
func RebaseContinue(dir string) (string, error) {
	return run(dir, LocalTimeout, "rebase", "--continue")
}

// RebaseAbort annule un rebase en cours et restaure l'état d'avant (git rebase --abort).
func RebaseAbort(dir string) (string, error) {
	return run(dir, LocalTimeout, "rebase", "--abort")
}

// --- Nettoyer l'historique (local) ---

// CherryPick applique le commit indiqué sur la branche courante (git cherry-pick).
func CherryPick(dir, commit string) (string, error) {
	if err := safeRevision(commit); err != nil {
		return "", err
	}
	return run(dir, LocalTimeout, "cherry-pick", commit)
}

// CherryPickContinue poursuit un cherry-pick après résolution des conflits.
func CherryPickContinue(dir string) (string, error) {
	return run(dir, LocalTimeout, "cherry-pick", "--continue")
}

// CherryPickAbort annule un cherry-pick en cours.
func CherryPickAbort(dir string) (string, error) {
	return run(dir, LocalTimeout, "cherry-pick", "--abort")
}

// Revert crée un nouveau commit qui annule les effets du commit indiqué (git revert --no-edit).
// Non destructif : l'historique est préservé, on ajoute un commit « inverse ».
func Revert(dir, commit string) (string, error) {
	if err := safeRevision(commit); err != nil {
		return "", err
	}
	return run(dir, LocalTimeout, "revert", "--no-edit", commit)
}

// Reset déplace HEAD (et éventuellement l'index/l'arbre) vers ref. mode ∈ soft|mixed|hard|keep|merge.
// DESTRUCTIF en mode hard (perte des modifications non enregistrées). L'appelant garde ce geste
// derrière assertWritable.
func Reset(dir, mode, ref string) (string, error) {
	switch mode {
	case "soft", "mixed", "hard", "keep", "merge":
	default:
		return "", fmt.Errorf("mode de reset invalide : %q (attendu soft|mixed|hard|keep|merge)", mode)
	}
	if err := safeRevision(ref); err != nil {
		return "", err
	}
	return run(dir, LocalTimeout, "reset", "--"+mode, ref)
}

// Amend réécrit le dernier commit : intègre l'état courant de l'arbre (add -A) puis l'amende.
// message vide = on conserve le message existant (--no-edit). DESTRUCTIF (réécrit un commit) :
// à n'utiliser que sur un commit non encore publié. Garde-fou assertWritable côté appelant.
func Amend(dir, message string) (string, error) {
	if _, err := run(dir, LocalTimeout, "add", "-A"); err != nil {
		return "", err
	}
	// Même garde-fou que Commit : ne jamais réécrire un commit en y figeant des marques de
	// conflit (sinon PushForceWithLease les publierait).
	if stagedHasConflictMarkers(dir) {
		return "", ErrConflicted
	}
	if strings.TrimSpace(message) == "" {
		return run(dir, LocalTimeout, "commit", "--amend", "--no-edit")
	}
	return run(dir, LocalTimeout, "commit", "--amend", "-m", message)
}

// CommitPaths enregistre UNIQUEMENT les chemins indiqués (staging sélectif) avec un message.
// Permet un commit soigné sans embarquer de fichier parasite. Les chemins sont validés puis
// isolés par « -- ».
func CommitPaths(dir, message string, paths []string) (string, error) {
	if strings.TrimSpace(message) == "" {
		return "", fmt.Errorf("le message ne peut pas être vide")
	}
	if len(paths) == 0 {
		return "", fmt.Errorf("aucun chemin à enregistrer")
	}
	for _, p := range paths {
		if err := safePathspec(p); err != nil {
			return "", err
		}
	}
	addArgs := append([]string{"add", "--"}, paths...)
	if _, err := run(dir, LocalTimeout, addArgs...); err != nil {
		return "", err
	}
	// Même garde-fou que Commit : ne jamais enregistrer de marques de conflit, même sur un
	// sous-ensemble de chemins.
	if stagedHasConflictMarkers(dir) {
		return "", ErrConflicted
	}
	commitArgs := append([]string{"commit", "-m", message, "--"}, paths...)
	return run(dir, LocalTimeout, commitArgs...)
}

// TagCreate crée un tag local. message vide = tag léger ; sinon tag annoté (-a -m). Purement
// local — la publication passe par PushTag (geste distant, PIN requis).
func TagCreate(dir, name, message string) (string, error) {
	if err := safeRef(name); err != nil {
		return "", err
	}
	if strings.TrimSpace(message) == "" {
		return run(dir, LocalTimeout, "tag", "--", name)
	}
	return run(dir, LocalTimeout, "tag", "-a", "-m", message, "--", name)
}

// --- Promotion atelier → production ---
//
// Modèle : la prod reçoit une VERSION PROPRE (un seul commit = l'arbre de l'atelier), pas les
// micro-commits. SquashCommit fabrique ce commit par plomberie (commit-tree) SANS toucher la
// branche de travail de l'utilisateur ni l'index : il n'y a ni checkout ni switch.

// SquashCommit crée un commit dont l'ARBRE est celui de srcRef, en UN seul commit par-dessus
// baseRef (parent ; vide = commit racine). Renvoie le SHA. N'altère ni la branche courante ni
// l'index (git commit-tree). message ne peut pas être vide.
func SquashCommit(dir, srcRef, baseRef, message string) (string, error) {
	if strings.TrimSpace(message) == "" {
		return "", fmt.Errorf("le message de promotion ne peut pas être vide")
	}
	if err := safeRevision(srcRef); err != nil {
		return "", err
	}
	tree, err := run(dir, LocalTimeout, "rev-parse", srcRef+"^{tree}")
	if err != nil {
		return "", err
	}
	args := []string{"commit-tree", strings.TrimSpace(tree), "-m", message}
	if b := strings.TrimSpace(baseRef); b != "" {
		if err := safeRevision(b); err != nil {
			return "", err
		}
		parent, err := run(dir, LocalTimeout, "rev-parse", b)
		if err == nil && strings.TrimSpace(parent) != "" {
			args = append(args, "-p", strings.TrimSpace(parent))
		}
	}
	sha, err := run(dir, LocalTimeout, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(sha), nil
}

// FetchURL récupère une branche d'un dépôt distant arbitraire (sans l'ajouter comme remote
// permanent) ; la référence récupérée est disponible via FETCH_HEAD. Sert à connaître la base de
// la production avant une promotion. Renvoie le SHA de FETCH_HEAD, ou "" si la branche n'existe pas.
func FetchURL(dir, url, branch, token string, github bool, baseURL string) (string, error) {
	if err := safeRef(branch); err != nil {
		return "", err
	}
	if _, err := runC(dir, NetTimeout, authCfg(token, github, baseURL), "fetch", "--", url, branch); err != nil {
		// Distinguer « la branche n'existe pas encore » (1re promotion : base légitimement vide) d'une
		// VRAIE panne (réseau, auth, droits). On ne retombe sur base vide que pour une ref MANQUANTE —
		// pas sur un « repository not found » (dépôt/accès), sinon une promotion écraserait la prod en
		// la croyant vierge.
		m := strings.ToLower(err.Error())
		if strings.Contains(m, "couldn't find remote ref") || strings.Contains(m, "no such ref") {
			return "", nil
		}
		return "", err
	}
	sha, err := run(dir, LocalTimeout, "rev-parse", "FETCH_HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(sha), nil
}

// PushCommitTo publie un commit (par son SHA) vers une branche d'un dépôt distant arbitraire :
// `git push <url> <sha>:refs/heads/<branch>`. force ajoute --force-with-lease (promotion qui
// réécrit la vitrine). Auth par jeton injecté (jamais en argv).
func PushCommitTo(dir, url, sha, branch, token string, github bool, baseURL string, force bool) (string, error) {
	if err := safeRevision(sha); err != nil {
		return "", err
	}
	if err := safeRef(branch); err != nil {
		return "", err
	}
	refspec := sha + ":refs/heads/" + branch
	args := []string{"push"}
	if force {
		args = append(args, "--force-with-lease")
	}
	args = append(args, "--", url, refspec)
	return runC(dir, NetTimeout, authCfg(token, github, baseURL), args...)
}

// --- Pousser après réécriture (distant : token + PIN côté appelant) ---

// PushForceWithLease force la mise à jour de la branche distante APRÈS un rebase/amend, mais
// seulement si le distant n'a pas bougé À NOTRE INSU (--force-with-lease). C'est le « force » sûr :
// il refuse d'écraser un travail distant apparu entre-temps.
//
// --force-if-includes (git ≥ 2.30) est INDISPENSABLE ici : sans lui, le bail nu se contente de
// comparer à la ref de suivi locale (refs/remotes/origin/…), que l'auto-fetch de fond de SituLab
// avance tout seul — un `push --force-with-lease` nu pourrait alors écraser silencieusement un
// commit distant juste récupéré. --force-if-includes vérifie que notre historique local inclut
// bien ce que le distant a eu entre-temps (via le reflog de la ref de suivi), fermant cette faille.
func PushForceWithLease(dir, token string, github bool, baseURL string) (string, error) {
	return runC(dir, NetTimeout, authCfg(token, github, baseURL), "push", "--force-with-lease", "--force-if-includes")
}

// UnpushedSubjects renvoie les sujets (1re ligne) des commits locaux PAS encore en ligne, pour
// que l'humain voie ce qu'il s'apprête à publier. S'il n'y a pas d'amont (branche neuve), renvoie
// les `max` derniers commits (tous « nouveaux » du point de vue distant). Borné à `max`.
func UnpushedSubjects(dir string, max int) []string {
	if max <= 0 {
		max = 20
	}
	spec := "@{upstream}..HEAD"
	if _, err := run(dir, LocalTimeout, "rev-parse", "--abbrev-ref", "@{upstream}"); err != nil {
		spec = "HEAD" // pas d'amont : on montre les derniers commits
	}
	out, err := run(dir, LocalTimeout, "log", spec, "--format=%s", "-n", strconv.Itoa(max))
	if err != nil {
		return nil
	}
	var subs []string
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			subs = append(subs, l)
		}
	}
	return subs
}

// Untracked liste les fichiers NON SUIVIS (nouveaux) respectant .gitignore — le vrai point de
// vigilance avant un envoi (fichier parasite/sensible embarqué). Borné à `max`.
func Untracked(dir string, max int) []string {
	if max <= 0 {
		max = 100
	}
	out, err := run(dir, LocalTimeout, "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return nil
	}
	var fs []string
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			fs = append(fs, l)
			if len(fs) >= max {
				break
			}
		}
	}
	return fs
}

// PushTag publie un tag précis vers origin (git push origin <tag>). Le nom est validé.
func PushTag(dir, tag, token string, github bool, baseURL string) (string, error) {
	if err := safeRef(tag); err != nil {
		return "", err
	}
	return runC(dir, NetTimeout, authCfg(token, github, baseURL), "push", remoteName(dir), "tag", tag)
}
