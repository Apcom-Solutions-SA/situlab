// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package main

import (
	"situlab/internal/core"
	"situlab/internal/forgejo"

	wailsrt "github.com/wailsapp/wails/v2/pkg/runtime"
)

// --- Écritures et lectures API étendues (v1) ---
func (a *App) CreateRepo(instance, owner, name, desc string, private bool, pin string) (*forgejo.Repo, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	r, err := a.svc.CreateRepo(instance, owner, name, desc, private, pin)
	if err == nil {
		a.afterWrite()
	}
	return r, err
}

// DeleteRepo supprime définitivement un dépôt distant (PIN requis).
func (a *App) DeleteRepo(instance, owner, repo, pin string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.lockedErr(a.svc.DeleteRepo(instance, owner, repo, pin))
}

func (a *App) CreateIssue(instance, owner, repo, title, body, pin string) (*forgejo.Issue, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	r, err := a.svc.CreateIssue(instance, owner, repo, title, body, pin)
	if err == nil {
		a.afterWrite()
	}
	return r, err
}

func (a *App) CommentIssue(instance, owner, repo string, index int, body, pin string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.lockedErr(a.svc.CommentIssue(instance, owner, repo, index, body, pin))
}

func (a *App) SetIssueState(instance, owner, repo string, index int, state, pin string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.lockedErr(a.svc.SetIssueState(instance, owner, repo, index, state, pin))
}

func (a *App) EditIssue(instance, owner, repo string, index int, title, body, pin string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.lockedErr(a.svc.EditIssue(instance, owner, repo, index, title, body, pin))
}

func (a *App) CreatePull(instance, owner, repo, title, head, base, body, pin string) (*forgejo.Issue, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	r, err := a.svc.CreatePull(instance, owner, repo, title, head, base, body, pin)
	if err == nil {
		a.afterWrite()
	}
	return r, err
}

func (a *App) MergePull(instance, owner, repo string, index int, method, pin string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.lockedErr(a.svc.MergePull(instance, owner, repo, index, method, pin))
}

func (a *App) RepoIssues(instance, owner, repo, state string) ([]forgejo.Issue, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.RepoIssues(instance, owner, repo, state)
}

func (a *App) RepoPulls(instance, owner, repo, state string) ([]forgejo.Issue, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.RepoPulls(instance, owner, repo, state)
}

func (a *App) RepoReleases(instance, owner, repo string) ([]forgejo.Release, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.RepoReleases(instance, owner, repo)
}

// --- Administration ---
func (a *App) AdminUsers(instance string) ([]forgejo.AdminUser, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.AdminUsers(instance)
}

// ConsolidatedDirectory agrège les comptes de toutes les instances Forgejo (v1.4, lecture seule).
func (a *App) ConsolidatedDirectory() (*core.Directory, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.ConsolidatedDirectory()
}

// AnalyzeTeam lance l'analyse d'équipe (v1.5.0) et émet « report:progress » pour l'attente côté
// fenêtre ; renvoie le rapport (aussi enregistré sur le poste). logins vide = toute l'équipe.
func (a *App) AnalyzeTeam(instance string, logins []string, since, until string) (*core.TeamReport, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.AnalyzeTeam(instance, logins, since, until, func(done, total int, label string) {
		wailsrt.EventsEmit(a.ctx, "report:progress", map[string]any{"done": done, "total": total, "repo": label})
	})
}

// ListTeamReports / LoadTeamReport : historique des analyses enregistrées (« état au… »).
func (a *App) ListTeamReports() ([]core.TeamReportMeta, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.ListTeamReports()
}

func (a *App) LoadTeamReport(id string) (*core.TeamReport, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.LoadTeamReport(id)
}

func (a *App) AdminCreateUser(instance, username, email, password string, admin bool, pin string) (*forgejo.AdminUser, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	u, err := a.svc.AdminCreateUser(instance, username, email, password, admin, pin)
	if err == nil {
		a.afterWrite()
	}
	return u, err
}

func (a *App) AdminUserRepos(instance string, uid int64) ([]forgejo.Repo, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.AdminUserRepos(instance, uid)
}

func (a *App) AdminUserActivities(instance, username string) ([]forgejo.Activity, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.AdminUserActivities(instance, username)
}

func (a *App) AdminSuspendUser(instance, username string, suspend bool, pin string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.lockedErr(a.svc.AdminSuspendUser(instance, username, suspend, pin))
}

func (a *App) AdminDeleteUser(instance, username string, purge bool, pin string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.lockedErr(a.svc.AdminDeleteUser(instance, username, purge, pin))
}

func (a *App) TokenList(instance, user, password, otp string) ([]forgejo.Token, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.TokenList(instance, user, password, otp)
}

func (a *App) TokenCreate(instance, user, password, otp, name string, scopes, repos []string, pin string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	tok, err := a.svc.TokenCreate(instance, user, password, otp, name, scopes, repos, pin)
	if err == nil {
		a.afterWrite()
	}
	return tok, err
}

func (a *App) TokenDelete(instance, user, password, otp string, id int64, pin string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.lockedErr(a.svc.TokenDelete(instance, user, password, otp, id, pin))
}

// --- v1.2 : PR (détail/diff/revue/état), releases, réglages dépôt, labels/assignés/milestones ---

func (a *App) PullDetail(instance, owner, repo string, index int) (*forgejo.PullInfo, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.PullDetail(instance, owner, repo, index)
}

func (a *App) PullDiff(instance, owner, repo string, index int) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.svc.PullDiff(instance, owner, repo, index)
}

func (a *App) PullFiles(instance, owner, repo string, index int) ([]forgejo.PullFile, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.PullFiles(instance, owner, repo, index)
}

func (a *App) ReviewPull(instance, owner, repo string, index int, event, body, pin string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.lockedErr(a.svc.ReviewPull(instance, owner, repo, index, event, body, pin))
}

func (a *App) SetPullState(instance, owner, repo string, index int, state, pin string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.lockedErr(a.svc.SetPullState(instance, owner, repo, index, state, pin))
}

func (a *App) EditPull(instance, owner, repo string, index int, title, body, pin string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.lockedErr(a.svc.EditPull(instance, owner, repo, index, title, body, pin))
}

func (a *App) CreateRelease(instance, owner, repo, tag, name, body, target string, draft, prerelease bool, pin string) (*forgejo.Release, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	r, err := a.svc.CreateRelease(instance, owner, repo, tag, name, body, target, draft, prerelease, pin)
	if err == nil {
		a.afterWrite()
	}
	return r, err
}

func (a *App) DeleteRelease(instance, owner, repo string, id int64, pin string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.lockedErr(a.svc.DeleteRelease(instance, owner, repo, id, pin))
}

func (a *App) EditRepoSettings(instance, owner, repo string, fields map[string]any, pin string) (*forgejo.Repo, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	r, err := a.svc.EditRepoSettings(instance, owner, repo, fields, pin)
	if err == nil {
		a.afterWrite()
	}
	return r, err
}

// DeleteRemoteBranch supprime une branche distante (PIN ; refuse la branche par défaut).
func (a *App) DeleteRemoteBranch(instance, owner, repo, branch, pin string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.lockedErr(a.svc.DeleteRemoteBranch(instance, owner, repo, branch, pin))
}

func (a *App) RepoLabels(instance, owner, repo string) ([]forgejo.Label, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.RepoLabels(instance, owner, repo)
}

func (a *App) RepoMilestones(instance, owner, repo string) ([]forgejo.Milestone, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.RepoMilestones(instance, owner, repo)
}

func (a *App) SetIssueLabels(instance, owner, repo string, index int, names []string, pin string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.lockedErr(a.svc.SetIssueLabels(instance, owner, repo, index, names, pin))
}

func (a *App) SetIssueAssignees(instance, owner, repo string, index int, assignees []string, pin string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.lockedErr(a.svc.SetIssueAssignees(instance, owner, repo, index, assignees, pin))
}

func (a *App) SetIssueMilestone(instance, owner, repo string, index int, milestoneID int64, pin string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.lockedErr(a.svc.SetIssueMilestone(instance, owner, repo, index, milestoneID, pin))
}

// --- Clouds de fichiers souverains (CORPUS, v1.10) ---
// La fenêtre (humain) a un accès COMPLET : c'est elle qui choisit ce que l'IA pourra voir.
// L'allowlist n'est imposée qu'aux agents (CLI/MCP, via le démon), jamais ici.
