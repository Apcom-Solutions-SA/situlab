// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"fmt"
	"strings"

	"situlab/internal/config"
	"situlab/internal/forgejo"
	"situlab/internal/gitx"
)

// --- Demandes de fusion : détail / diff / fichiers (lecture) ; revue, état, édition (PIN) ---

func (s *Service) PullDetail(instance, owner, repo string, index int) (*forgejo.PullInfo, error) {
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return nil, err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.PullDetail(ctx, owner, repo, index)
}

func (s *Service) PullDiff(instance, owner, repo string, index int) (string, error) {
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return "", err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.PullDiff(ctx, owner, repo, index)
}

func (s *Service) PullFiles(instance, owner, repo string, index int) ([]forgejo.PullFile, error) {
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return nil, err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.PullFiles(ctx, owner, repo, index)
}

// ReviewPull : event = APPROVED | REQUEST_CHANGES | COMMENT (écriture — PIN requis).
func (s *Service) ReviewPull(instance, owner, repo string, index int, event, body, pin string) error {
	if err := s.authorize(instance, config.ActionIssues, pin); err != nil {
		return err
	}
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.ReviewPull(ctx, owner, repo, index, event, body)
}

// SetPullState ferme/rouvre une PR sans fusionner (écriture — PIN requis).
func (s *Service) SetPullState(instance, owner, repo string, index int, state, pin string) error {
	if err := s.authorize(instance, config.ActionIssues, pin); err != nil {
		return err
	}
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.SetPullState(ctx, owner, repo, index, state)
}

// EditPull modifie titre/corps d'une PR (écriture — PIN requis).
func (s *Service) EditPull(instance, owner, repo string, index int, title, body, pin string) error {
	if err := s.authorize(instance, config.ActionIssues, pin); err != nil {
		return err
	}
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.EditPull(ctx, owner, repo, index, title, body)
}

// --- Releases & tags (écritures — PIN requis) ---

func (s *Service) CreateRelease(instance, owner, repo, tag, name, body, target string, draft, prerelease bool, pin string) (*forgejo.Release, error) {
	if err := s.authorize(instance, config.ActionRepoAdmin, pin); err != nil {
		return nil, err
	}
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return nil, err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.CreateRelease(ctx, owner, repo, tag, name, body, target, draft, prerelease)
}

func (s *Service) DeleteRelease(instance, owner, repo string, id int64, pin string) error {
	if err := s.authorize(instance, config.ActionRepoAdmin, pin); err != nil {
		return err
	}
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.DeleteRelease(ctx, owner, repo, id)
}

func (s *Service) CreateTag(instance, owner, repo, tag, target, pin string) error {
	if err := s.authorize(instance, config.ActionRepoAdmin, pin); err != nil {
		return err
	}
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.CreateTag(ctx, owner, repo, tag, target)
}

func (s *Service) DeleteTag(instance, owner, repo, tag, pin string) error {
	if err := s.authorize(instance, config.ActionRepoAdmin, pin); err != nil {
		return err
	}
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.DeleteTag(ctx, owner, repo, tag)
}

// DeleteRemoteBranch supprime une branche distante (admin dépôt, sous PIN). Garde-fou : on refuse la
// branche par défaut. Sert notamment à nettoyer les branches de travail fusionnées côté atelier.
func (s *Service) DeleteRemoteBranch(instance, owner, repo, branch, pin string) error {
	if err := s.authorize(instance, config.ActionRepoAdmin, pin); err != nil {
		return err
	}
	if r, e := s.ViewRepo(instance, owner, repo); e == nil && r.DefaultBranch == branch {
		return fmt.Errorf("refus : « %s » est la branche par défaut de %s/%s", branch, owner, repo)
	}
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.DeleteBranch(ctx, owner, repo, branch)
}

// --- Réglages de dépôt (écritures — PIN requis) ---

// EditRepoSettings applique un sous-ensemble de réglages : name (renommer), private,
// archived, default_branch. Les champs non fournis (nil) ne sont pas touchés.
func (s *Service) EditRepoSettings(instance, owner, repo string, fields map[string]any, pin string) (*forgejo.Repo, error) {
	if err := s.authorize(instance, config.ActionRepoAdmin, pin); err != nil {
		return nil, err
	}
	if len(fields) == 0 {
		return nil, fmt.Errorf("aucun réglage à modifier")
	}
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return nil, err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.EditRepo(ctx, owner, repo, fields)
}

// --- Labels / assignés / milestones ---

func (s *Service) RepoLabels(instance, owner, repo string) ([]forgejo.Label, error) {
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return nil, err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.ListLabels(ctx, owner, repo)
}

func (s *Service) RepoMilestones(instance, owner, repo string) ([]forgejo.Milestone, error) {
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return nil, err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.ListMilestones(ctx, owner, repo)
}

// SetIssueLabels remplace les étiquettes d'une issue à partir de NOMS d'étiquettes (résolus
// en identifiants via la liste du dépôt). Écriture — PIN requis.
func (s *Service) SetIssueLabels(instance, owner, repo string, index int, names []string, pin string) error {
	if err := s.authorize(instance, config.ActionIssues, pin); err != nil {
		return err
	}
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return err
	}
	ctx, cancel := opCtx()
	defer cancel()
	labels, err := cl.ListLabels(ctx, owner, repo)
	if err != nil {
		return err
	}
	byName := map[string]int64{}
	for _, l := range labels {
		byName[strings.ToLower(l.Name)] = l.ID
	}
	var ids []int64
	var missing []string
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if id, ok := byName[strings.ToLower(n)]; ok {
			ids = append(ids, id)
		} else {
			missing = append(missing, n)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("étiquette(s) introuvable(s) : %s", strings.Join(missing, ", "))
	}
	return cl.SetIssueLabels(ctx, owner, repo, index, ids)
}

func (s *Service) SetIssueAssignees(instance, owner, repo string, index int, assignees []string, pin string) error {
	if err := s.authorize(instance, config.ActionIssues, pin); err != nil {
		return err
	}
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.SetIssueAssignees(ctx, owner, repo, index, assignees)
}

func (s *Service) SetIssueMilestone(instance, owner, repo string, index int, milestoneID int64, pin string) error {
	if err := s.authorize(instance, config.ActionIssues, pin); err != nil {
		return err
	}
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.SetIssueMilestone(ctx, owner, repo, index, milestoneID)
}

// PushPreview résume, AVANT la demande de PIN, ce qu'un envoi/synchro va publier : branche,
// nombre de commits en avance, leurs sujets, les fichiers modifiés et surtout les fichiers NOUVEAUX
// (non suivis). But (v1.7) : transformer la validation « à l'aveugle » en consentement éclairé. Tout
// est borné ; une erreur de lecture renvoie un aperçu partiel plutôt que de bloquer l'envoi.
type PushPreview struct {
	Branch    string   `json:"branch"`
	Remote    string   `json:"remote"`
	Ahead     int      `json:"ahead"`
	Commits   []string `json:"commits"`   // sujets des commits à envoyer
	Changed   []string `json:"changed"`   // fichiers modifiés (suivis) non encore enregistrés
	Untracked []string `json:"untracked"` // fichiers NOUVEAUX non suivis (vigilance)
}

func (s *Service) PushPreview(path string) *PushPreview {
	if !gitx.Available() {
		return nil
	}
	p := &PushPreview{}
	if st, err := gitx.StatusOf(path); err == nil && st.IsRepo {
		p.Branch = st.Branch
		p.Remote = st.Remote
		p.Ahead = st.Ahead
		for _, f := range st.Changed { // ne garde que les fichiers suivis ici (les non suivis sont listés à part)
			p.Changed = append(p.Changed, f)
		}
	}
	p.Commits = gitx.UnpushedSubjects(path, 20)
	p.Untracked = gitx.Untracked(path, 50)
	// Évite de compter deux fois : retire des « modifiés » ce qui est en réalité « nouveau ».
	if len(p.Untracked) > 0 {
		set := make(map[string]bool, len(p.Untracked))
		for _, u := range p.Untracked {
			set[u] = true
		}
		kept := p.Changed[:0]
		for _, c := range p.Changed {
			if !set[c] {
				kept = append(kept, c)
			}
		}
		p.Changed = kept
	}
	return p
}

// --- Fonctions git locales (workflow dev) : pas de PIN (local, aucun accès distant) ---

func (s *Service) GitCreateBranch(path, name string) (string, error) {
	if !gitx.Available() {
		return "", ErrNoGit
	}
	if err := s.assertWritable(); err != nil {
		return "", err
	}
	return gitx.CreateBranch(path, name)
}

func (s *Service) GitSwitchBranch(path, name string) (string, error) {
	if !gitx.Available() {
		return "", ErrNoGit
	}
	if err := s.assertWritable(); err != nil { // réécrit l'arbre de travail → bloqué en lecture seule
		return "", err
	}
	return gitx.SwitchBranch(path, name)
}

// GitDeleteBranch supprime une branche LOCALE (local, pas de PIN ; bloqué en lecture seule). force=false
// laisse git REFUSER une branche non fusionnée (garde-fou anti-perte) ; force=true supprime quand même.
// L'appelant (GUI) prévient d'abord l'humain quand la branche n'est pas fusionnée.
func (s *Service) GitDeleteBranch(path, name string, force bool) (string, error) {
	if !gitx.Available() {
		return "", ErrNoGit
	}
	if err := s.assertWritable(); err != nil {
		return "", err
	}
	return gitx.DeleteBranch(path, name, force)
}

// GitDiscard annule les modifications locales non enregistrées. DESTRUCTIF.
func (s *Service) GitDiscard(path, file string) (string, error) {
	if !gitx.Available() {
		return "", ErrNoGit
	}
	if err := s.assertWritable(); err != nil { // destructif local : bloqué aussi en lecture seule (v1.4)
		return "", err
	}
	return gitx.DiscardChanges(path, file)
}

func (s *Service) GitLog(path string, limit int) ([]gitx.LocalCommit, error) {
	if !gitx.Available() {
		return nil, ErrNoGit
	}
	return gitx.Log(path, limit), nil
}

func (s *Service) GitStashSave(path, message string) (string, error) {
	if !gitx.Available() {
		return "", ErrNoGit
	}
	if err := s.assertWritable(); err != nil { // retire le travail en cours de l'arbre → bloqué en lecture seule
		return "", err
	}
	return gitx.StashSave(path, message)
}

func (s *Service) GitStashList(path string) (string, error) {
	if !gitx.Available() {
		return "", ErrNoGit
	}
	return gitx.StashList(path)
}

func (s *Service) GitStashPop(path string) (string, error) {
	if !gitx.Available() {
		return "", ErrNoGit
	}
	if err := s.assertWritable(); err != nil { // réapplique/modifie l'arbre → bloqué en lecture seule
		return "", err
	}
	return gitx.StashPop(path)
}

// --- Worktrees : plusieurs arbres de travail (branches) depuis un seul dépôt (local, pas de PIN) ---

// GitWorktrees liste les arbres de travail d'un dépôt local (lecture).
func (s *Service) GitWorktrees(path string) ([]gitx.Worktree, error) {
	if !gitx.Available() {
		return nil, ErrNoGit
	}
	return gitx.WorktreeList(path)
}

// GitWorktreeAdd crée un arbre de travail dans wtPath. createBranch=true → nouvelle branche `branch` ;
// sinon extrait la branche/commit `branch` existant. Réécrit le disque → bloqué en lecture seule.
func (s *Service) GitWorktreeAdd(path, wtPath, branch string, createBranch bool) (string, error) {
	if !gitx.Available() {
		return "", ErrNoGit
	}
	if err := s.assertWritable(); err != nil {
		return "", err
	}
	return gitx.WorktreeAdd(path, wtPath, branch, createBranch)
}

// GitWorktreeRemove retire un arbre de travail (supprime son dossier). force=true si modifications.
func (s *Service) GitWorktreeRemove(path, wtPath string, force bool) (string, error) {
	if !gitx.Available() {
		return "", ErrNoGit
	}
	if err := s.assertWritable(); err != nil {
		return "", err
	}
	return gitx.WorktreeRemove(path, wtPath, force)
}

// GitWorktreePrune nettoie les références des worktrees dont le dossier a disparu.
func (s *Service) GitWorktreePrune(path string) (string, error) {
	if !gitx.Available() {
		return "", ErrNoGit
	}
	if err := s.assertWritable(); err != nil {
		return "", err
	}
	return gitx.WorktreePrune(path)
}

// --- Git « complet » (v1.6) : gestes de dev avancés. Exposés en CLI/MCP (l'IA travaille là).
//     Gestes LOCAUX (merge, rebase, cherry-pick, revert, reset, amend, staging sélectif, tag
//     local) : aucun accès distant → PAS de PIN, mais assertWritable car ils réécrivent l'arbre
//     ou l'historique. Gestes DISTANTS (force-with-lease, push de tag) : même portillon PIN que
//     push/Synchronize — le modèle de sécurité n'est pas affaibli. ---

// mapConflict : si une opération de fusion a échoué ET que l'arbre est désormais en conflit,
// renvoie ErrConflict (message guidé stable) en conservant l'état pour résolution. Sinon
// propage l'erreur d'origine. Centralise le traitement « conflit » de merge/rebase/cherry-pick.
func mapConflict(path, out string, err error) (string, error) {
	if err == nil {
		return out, nil
	}
	if st, e := gitx.StatusOf(path); e == nil && len(st.Conflicted) > 0 {
		return out, ErrConflict
	}
	return out, err
}

// GitMerge fusionne ref dans la branche courante (local, pas de PIN).
func (s *Service) GitMerge(path, ref string) (string, error) {
	if !gitx.Available() {
		return "", ErrNoGit
	}
	if err := s.assertWritable(); err != nil {
		return "", err
	}
	out, err := gitx.Merge(path, ref)
	return mapConflict(path, out, err)
}

// GitMergeAbort annule une fusion en cours.
func (s *Service) GitMergeAbort(path string) (string, error) {
	if !gitx.Available() {
		return "", ErrNoGit
	}
	if err := s.assertWritable(); err != nil {
		return "", err
	}
	return gitx.MergeAbort(path)
}

// GitRebase rejoue la branche courante au sommet de upstream (local, pas de PIN).
func (s *Service) GitRebase(path, upstream string) (string, error) {
	if !gitx.Available() {
		return "", ErrNoGit
	}
	if err := s.assertWritable(); err != nil {
		return "", err
	}
	out, err := gitx.Rebase(path, upstream)
	return mapConflict(path, out, err)
}

// GitRebaseContinue poursuit un rebase après résolution des conflits.
func (s *Service) GitRebaseContinue(path string) (string, error) {
	if !gitx.Available() {
		return "", ErrNoGit
	}
	if err := s.assertWritable(); err != nil {
		return "", err
	}
	out, err := gitx.RebaseContinue(path)
	return mapConflict(path, out, err)
}

// GitRebaseAbort annule un rebase en cours.
func (s *Service) GitRebaseAbort(path string) (string, error) {
	if !gitx.Available() {
		return "", ErrNoGit
	}
	if err := s.assertWritable(); err != nil {
		return "", err
	}
	return gitx.RebaseAbort(path)
}

// GitCherryPick applique un commit précis sur la branche courante (local, pas de PIN).
func (s *Service) GitCherryPick(path, commit string) (string, error) {
	if !gitx.Available() {
		return "", ErrNoGit
	}
	if err := s.assertWritable(); err != nil {
		return "", err
	}
	out, err := gitx.CherryPick(path, commit)
	return mapConflict(path, out, err)
}

// GitCherryPickContinue poursuit un cherry-pick après résolution des conflits.
func (s *Service) GitCherryPickContinue(path string) (string, error) {
	if !gitx.Available() {
		return "", ErrNoGit
	}
	if err := s.assertWritable(); err != nil {
		return "", err
	}
	out, err := gitx.CherryPickContinue(path)
	return mapConflict(path, out, err)
}

// GitCherryPickAbort annule un cherry-pick en cours.
func (s *Service) GitCherryPickAbort(path string) (string, error) {
	if !gitx.Available() {
		return "", ErrNoGit
	}
	if err := s.assertWritable(); err != nil {
		return "", err
	}
	return gitx.CherryPickAbort(path)
}

// GitRevert ajoute un commit annulant un commit précis (local, pas de PIN, non destructif).
func (s *Service) GitRevert(path, commit string) (string, error) {
	if !gitx.Available() {
		return "", ErrNoGit
	}
	if err := s.assertWritable(); err != nil {
		return "", err
	}
	out, err := gitx.Revert(path, commit)
	return mapConflict(path, out, err)
}

// GitReset déplace HEAD vers ref (mode soft|mixed|hard|keep|merge). DESTRUCTIF en hard : bloqué
// en lecture seule. Local, pas de PIN.
func (s *Service) GitReset(path, mode, ref string) (string, error) {
	if !gitx.Available() {
		return "", ErrNoGit
	}
	if err := s.assertWritable(); err != nil {
		return "", err
	}
	return gitx.Reset(path, mode, ref)
}

// GitAmend réécrit le dernier commit (intègre l'arbre courant + nouveau message si fourni).
// DESTRUCTIF (réécriture) : bloqué en lecture seule. Local, pas de PIN.
func (s *Service) GitAmend(path, message string) (string, error) {
	if !gitx.Available() {
		return "", ErrNoGit
	}
	if err := s.assertWritable(); err != nil {
		return "", err
	}
	out, err := gitx.Amend(path, message)
	if err == nil {
		s.attestCommit(path, gitx.HeadSHA(path))
	}
	return out, asConflict(err)
}

// GitCommitPaths enregistre uniquement les chemins indiqués (staging sélectif). Local, pas de PIN.
func (s *Service) GitCommitPaths(path, message string, paths []string) (string, error) {
	if !gitx.Available() {
		return "", ErrNoGit
	}
	if err := s.assertWritable(); err != nil {
		return "", err
	}
	out, err := gitx.CommitPaths(path, message, paths)
	if err == nil {
		s.attestCommit(path, gitx.HeadSHA(path))
	}
	return out, asConflict(err)
}

// GitTagCreate crée un tag local (annoté si message non vide). Local, pas de PIN ; la
// publication passe par GitPushTag (distant).
func (s *Service) GitTagCreate(path, name, message string) (string, error) {
	if !gitx.Available() {
		return "", ErrNoGit
	}
	if err := s.assertWritable(); err != nil {
		return "", err
	}
	return gitx.TagCreate(path, name, message)
}

// GitPushForce pousse la branche courante en force-with-lease (après rebase/amend). Geste
// DISTANT → PIN requis (même portillon que push/Synchronize).
func (s *Service) GitPushForce(instance, path, pinCode string) (string, error) {
	if !gitx.Available() {
		return "", ErrNoGit
	}
	if err := s.authorizePush(instance, path, pinCode); err != nil {
		return "", err
	}
	return gitx.PushForceWithLease(path, s.instanceToken(instance), s.isGitHub(instance), s.instanceBaseURL(instance))
}

// GitPushTag publie un tag vers origin. Geste DISTANT → PIN requis.
func (s *Service) GitPushTag(instance, path, tag, pinCode string) (string, error) {
	if !gitx.Available() {
		return "", ErrNoGit
	}
	if err := s.authorizePush(instance, path, pinCode); err != nil {
		return "", err
	}
	return gitx.PushTag(path, tag, s.instanceToken(instance), s.isGitHub(instance), s.instanceBaseURL(instance))
}

// FileHistory : commits qui ont touché un fichier dans un clone LOCAL (lecture, pas de PIN).
func (s *Service) FileHistory(path, file string, limit int) ([]gitx.LocalCommit, error) {
	if !gitx.Available() {
		return nil, ErrNoGit
	}
	return gitx.FileLog(path, file, limit), nil
}

// FileDiffAt : diff d'un commit restreint à un fichier (clone local ; lecture, pas de PIN).
func (s *Service) FileDiffAt(path, sha, file string) (string, error) {
	if !gitx.Available() {
		return "", ErrNoGit
	}
	return gitx.FileDiffAt(path, sha, file)
}
