// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package cli

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"time"

	"situlab/internal/config"
	"situlab/internal/core"
	"situlab/internal/daemon"
	"situlab/internal/forgejo"
	"situlab/internal/gitx"
	"situlab/internal/ipc"
)

// remote est le client CLI : il relaie chaque appel au DÉMON résident (qui détient la
// session et affiche la modale PIN). Connexion paresseuse — les commandes qui n'ont
// besoin de rien (version, aide) ne démarrent jamais le démon.
type remote struct{ cl *ipc.Client }

func newRemote() *remote { return &remote{} }

// ensure garantit une connexion au démon ; s'il est absent, le démarre puis réessaie.
func (r *remote) ensure() error {
	if r.cl != nil {
		return nil
	}
	path := ipc.SocketPath()
	if cl, err := ipc.Dial(path); err == nil {
		r.cl = cl
		return nil
	}
	if err := startDaemon(); err != nil {
		return err
	}
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(150 * time.Millisecond)
		if cl, err := ipc.Dial(path); err == nil {
			r.cl = cl
			return nil
		}
	}
	return errors.New("impossible de joindre le démon SituLab (il ne répond pas)")
}

// startDaemon lance le binaire SituLab sans argument : il ouvre la fenêtre ET démarre le
// démon (serveur IPC). Process détaché : la CLI n'attend pas.
func startDaemon() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	return cmd.Start()
}

// kindError porte le classement d'erreur du démon pour retrouver les codes de sortie.
type kindError struct {
	msg  string
	kind string
}

func (e kindError) Error() string { return e.msg }
func (e kindError) Is(target error) bool {
	switch e.kind {
	case "noinstance":
		return target == core.ErrNoInstance
	case "pinneeded":
		return target == core.ErrPINNeeded
	case "pinlocked":
		return target == core.ErrPINLocked
	case "nogit":
		return target == core.ErrNoGit
	case "locked":
		return target == core.ErrLocked
	}
	return false
}

// call envoie une opération au démon et décode éventuellement le résultat dans out.
func (r *remote) call(op string, a daemon.Args, out any) error {
	if err := r.ensure(); err != nil {
		return err
	}
	raw, _ := json.Marshal(a)
	resp, err := r.cl.Call(ipc.Request{Op: op, Args: raw})
	if err != nil {
		return err
	}
	if !resp.OK {
		return kindError{msg: resp.Err, kind: resp.Kind}
	}
	if out != nil && len(resp.Data) > 0 && string(resp.Data) != "null" {
		return json.Unmarshal(resp.Data, out)
	}
	return nil
}

// callStr décode un retour { "<key>": "<valeur>" }.
func (r *remote) callStr(op, key string, a daemon.Args) (string, error) {
	var m map[string]string
	if err := r.call(op, a, &m); err != nil {
		return "", err
	}
	return m[key], nil
}

// --- surface appelée par cli.go (mêmes signatures que *core.Service) ---

func (r *remote) Doctor() (core.Doctor, error) {
	var d core.Doctor
	err := r.call(daemon.OpDoctor, daemon.Args{}, &d)
	return d, err
}

func (r *remote) PINSet() (bool, error) {
	var s daemon.Status
	err := r.call(daemon.OpStatus, daemon.Args{}, &s)
	return s.PINSet, err
}

func (r *remote) Unlock() error {
	// Pas de PIN sur le socket : le démon ouvre la fenêtre de saisie sur le poste.
	return r.call(daemon.OpUnlock, daemon.Args{}, nil)
}

func (r *remote) Lock() error {
	return r.call(daemon.OpLock, daemon.Args{}, nil)
}

func (r *remote) Status() (daemon.Status, error) {
	var s daemon.Status
	return s, r.call(daemon.OpStatus, daemon.Args{}, &s)
}

func (r *remote) Instances() ([]config.Instance, error) {
	var v []config.Instance
	err := r.call(daemon.OpInstanceList, daemon.Args{}, &v)
	return v, err
}

func (r *remote) AddInstanceToken(name, provider, url, token string) error {
	return r.call(daemon.OpInstanceAdd, daemon.Args{Name: name, Provider: provider, URL: url, Token: token}, nil)
}

func (r *remote) AddInstanceLogin(name, url, user, password, otp string, adminScope, remember bool) error {
	return r.call(daemon.OpInstanceLogin, daemon.Args{Name: name, URL: url, User: user, Password: password, OTP: otp, AdminScope: adminScope, Remember: remember}, nil)
}

func (r *remote) RemoveInstance(name string) error {
	return r.call(daemon.OpInstanceRm, daemon.Args{Name: name}, nil)
}

func (r *remote) SetDefaultInstance(name string) error {
	return r.call(daemon.OpInstanceDef, daemon.Args{Name: name}, nil)
}

func (r *remote) TestInstance(name string) (*forgejo.User, error) {
	var u forgejo.User
	if err := r.call(daemon.OpInstanceTest, daemon.Args{Instance: name}, &u); err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *remote) Whoami(instance string) (*forgejo.User, error) {
	var u forgejo.User
	if err := r.call(daemon.OpWhoami, daemon.Args{Instance: instance}, &u); err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *remote) Dashboard(instance string) (*core.DashboardData, error) {
	var d core.DashboardData
	if err := r.call(daemon.OpDashboard, daemon.Args{Instance: instance}, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *remote) Repos(instance string) ([]forgejo.Repo, error) {
	var v []forgejo.Repo
	return v, r.call(daemon.OpRepoList, daemon.Args{Instance: instance}, &v)
}

func (r *remote) ViewRepo(instance, owner, repo string) (*forgejo.Repo, error) {
	var x forgejo.Repo
	if err := r.call(daemon.OpRepoView, daemon.Args{Instance: instance, Owner: owner, Repo: repo}, &x); err != nil {
		return nil, err
	}
	return &x, nil
}

func (r *remote) RepoBranches(instance, owner, repo string) ([]forgejo.Branch, error) {
	var v []forgejo.Branch
	return v, r.call(daemon.OpRepoBranches, daemon.Args{Instance: instance, Owner: owner, Repo: repo}, &v)
}

func (r *remote) RepoTags(instance, owner, repo string) ([]forgejo.Tag, error) {
	var v []forgejo.Tag
	return v, r.call(daemon.OpRepoTags, daemon.Args{Instance: instance, Owner: owner, Repo: repo}, &v)
}

func (r *remote) RepoCommits(instance, owner, repo, ref string, limit int) ([]forgejo.Commit, error) {
	var v []forgejo.Commit
	return v, r.call(daemon.OpRepoCommits, daemon.Args{Instance: instance, Owner: owner, Repo: repo, Ref: ref, Limit: limit}, &v)
}

func (r *remote) RepoContents(instance, owner, repo, path, ref string) ([]forgejo.Entry, error) {
	var v []forgejo.Entry
	return v, r.call(daemon.OpRepoTree, daemon.Args{Instance: instance, Owner: owner, Repo: repo, Path: path, Ref: ref}, &v)
}

func (r *remote) RepoFile(instance, owner, repo, path, ref string) (string, error) {
	return r.callStr(daemon.OpRepoFile, "content", daemon.Args{Instance: instance, Owner: owner, Repo: repo, Path: path, Ref: ref})
}

func (r *remote) SetRepoDescription(instance, owner, repo, desc string) error {
	return r.call(daemon.OpRepoSetDesc, daemon.Args{Instance: instance, Owner: owner, Repo: repo, Desc: desc}, nil)
}

func (r *remote) Orgs(instance string) ([]forgejo.Org, error) {
	var v []forgejo.Org
	return v, r.call(daemon.OpOrgList, daemon.Args{Instance: instance}, &v)
}

func (r *remote) Tokens(instance string) ([]forgejo.Token, error) {
	var v []forgejo.Token
	return v, r.call(daemon.OpTokensList, daemon.Args{Instance: instance}, &v)
}

func (r *remote) Clone(instance, owner, repo, dir string) (string, error) {
	return r.callStr(daemon.OpClone, "output", daemon.Args{Instance: instance, Owner: owner, Repo: repo, Dir: dir})
}

func (r *remote) Push(instance, path string) (string, error) {
	return r.callStr(daemon.OpPush, "output", daemon.Args{Instance: instance, Path: path})
}

func (r *remote) Synchronize(instance, path, message string) (*core.SyncResult, error) {
	var x core.SyncResult
	if err := r.call(daemon.OpSync, daemon.Args{Instance: instance, Path: path, Message: message}, &x); err != nil {
		return nil, err
	}
	return &x, nil
}

func (r *remote) Fetch(instance, path string) (string, error) {
	return r.callStr(daemon.OpFetch, "output", daemon.Args{Instance: instance, Path: path})
}

func (r *remote) Pull(instance, path string) (string, error) {
	return r.callStr(daemon.OpPull, "output", daemon.Args{Instance: instance, Path: path})
}

func (r *remote) Diff(path string) (string, error) {
	return r.callStr(daemon.OpDiff, "output", daemon.Args{Path: path})
}

func (r *remote) Commit(path, message string) (string, error) {
	return r.callStr(daemon.OpCommit, "output", daemon.Args{Path: path, Message: message})
}

func (r *remote) CreateRepo(instance, owner, name, desc string, private bool) (*forgejo.Repo, error) {
	var x forgejo.Repo
	if err := r.call(daemon.OpRepoCreate, daemon.Args{Instance: instance, Owner: owner, Name: name, Desc: desc, Private: private}, &x); err != nil {
		return nil, err
	}
	return &x, nil
}

func (r *remote) DeleteRepo(instance, owner, repo string) error {
	return r.call(daemon.OpRepoDelete, daemon.Args{Instance: instance, Owner: owner, Repo: repo}, nil)
}

func (r *remote) CreateIssue(instance, owner, repo, title, body string) (*forgejo.Issue, error) {
	var x forgejo.Issue
	if err := r.call(daemon.OpIssueCreate, daemon.Args{Instance: instance, Owner: owner, Repo: repo, Title: title, Body: body}, &x); err != nil {
		return nil, err
	}
	return &x, nil
}

func (r *remote) RepoIssues(instance, owner, repo, state string) ([]forgejo.Issue, error) {
	var v []forgejo.Issue
	return v, r.call(daemon.OpIssueList, daemon.Args{Instance: instance, Owner: owner, Repo: repo, State: state}, &v)
}

func (r *remote) RepoPulls(instance, owner, repo, state string) ([]forgejo.Issue, error) {
	var v []forgejo.Issue
	return v, r.call(daemon.OpPullList, daemon.Args{Instance: instance, Owner: owner, Repo: repo, State: state}, &v)
}

func (r *remote) CommentIssue(instance, owner, repo string, index int, body string) error {
	return r.call(daemon.OpIssueComment, daemon.Args{Instance: instance, Owner: owner, Repo: repo, Index: index, Body: body}, nil)
}

func (r *remote) SetIssueState(instance, owner, repo string, index int, state string) error {
	return r.call(daemon.OpIssueState, daemon.Args{Instance: instance, Owner: owner, Repo: repo, Index: index, State: state}, nil)
}

func (r *remote) EditIssue(instance, owner, repo string, index int, title, body string) error {
	return r.call(daemon.OpIssueEdit, daemon.Args{Instance: instance, Owner: owner, Repo: repo, Index: index, Title: title, Body: body}, nil)
}

func (r *remote) CreatePull(instance, owner, repo, title, head, base, body string) (*forgejo.Issue, error) {
	var x forgejo.Issue
	if err := r.call(daemon.OpPullCreate, daemon.Args{Instance: instance, Owner: owner, Repo: repo, Title: title, Head: head, Base: base, Body: body}, &x); err != nil {
		return nil, err
	}
	return &x, nil
}

func (r *remote) MergePull(instance, owner, repo string, index int, method string) error {
	return r.call(daemon.OpPullMerge, daemon.Args{Instance: instance, Owner: owner, Repo: repo, Index: index, Method: method}, nil)
}

func (r *remote) AdminUsers(instance string) ([]forgejo.AdminUser, error) {
	var v []forgejo.AdminUser
	return v, r.call(daemon.OpAdminUsers, daemon.Args{Instance: instance}, &v)
}

func (r *remote) ConsolidatedDirectory() (*core.Directory, error) {
	var x core.Directory
	if err := r.call(daemon.OpDirectory, daemon.Args{}, &x); err != nil {
		return nil, err
	}
	return &x, nil
}

func (r *remote) AdminCreateUser(instance, username, email, password string, admin bool) (*forgejo.AdminUser, error) {
	var x forgejo.AdminUser
	if err := r.call(daemon.OpAdminUserNew, daemon.Args{Instance: instance, Name: username, Email: email, Password: password, Admin: admin}, &x); err != nil {
		return nil, err
	}
	return &x, nil
}

func (r *remote) TokenCreate(instance, user, password, otp, name string, scopes, repos []string) (string, error) {
	return r.callStr(daemon.OpTokenMgrNew, "token", daemon.Args{Instance: instance, User: user, Password: password, OTP: otp, Name: name, Scopes: scopes, Repos: repos})
}

func (r *remote) TokenDelete(instance, user, password, otp string, id int64) error {
	return r.call(daemon.OpTokenMgrDel, daemon.Args{Instance: instance, User: user, Password: password, OTP: otp, ID: id}, nil)
}

func (r *remote) LocalRepos() ([]*gitx.Status, error) {
	var v []*gitx.Status
	return v, r.call(daemon.OpLocalList, daemon.Args{}, &v)
}

func (r *remote) LocalStatus(path string) (*gitx.Status, error) {
	var x gitx.Status
	if err := r.call(daemon.OpLocalStatus, daemon.Args{Path: path}, &x); err != nil {
		return nil, err
	}
	return &x, nil
}

func (r *remote) AddLocalRepo(path string) error {
	return r.call(daemon.OpLocalAdd, daemon.Args{Path: path}, nil)
}

func (r *remote) RemoveLocalRepo(path string) error {
	return r.call(daemon.OpLocalRemove, daemon.Args{Path: path}, nil)
}

// --- v1.2 : PR (détail/diff/fichiers/revue/état/édition), releases/tags, réglages dépôt, labels/assignés/milestones ---

func (r *remote) PullDetail(instance, owner, repo string, index int) (*forgejo.PullInfo, error) {
	var x forgejo.PullInfo
	if err := r.call(daemon.OpPullDetail, daemon.Args{Instance: instance, Owner: owner, Repo: repo, Index: index}, &x); err != nil {
		return nil, err
	}
	return &x, nil
}

func (r *remote) PullDiff(instance, owner, repo string, index int) (string, error) {
	return r.callStr(daemon.OpPullDiff, "diff", daemon.Args{Instance: instance, Owner: owner, Repo: repo, Index: index})
}

func (r *remote) PullFiles(instance, owner, repo string, index int) ([]forgejo.PullFile, error) {
	var v []forgejo.PullFile
	return v, r.call(daemon.OpPullFiles, daemon.Args{Instance: instance, Owner: owner, Repo: repo, Index: index}, &v)
}

func (r *remote) ReviewPull(instance, owner, repo string, index int, event, body string) error {
	return r.call(daemon.OpPullReview, daemon.Args{Instance: instance, Owner: owner, Repo: repo, Index: index, Event: event, Body: body}, nil)
}

func (r *remote) SetPullState(instance, owner, repo string, index int, state string) error {
	return r.call(daemon.OpPullState, daemon.Args{Instance: instance, Owner: owner, Repo: repo, Index: index, State: state}, nil)
}

func (r *remote) EditPull(instance, owner, repo string, index int, title, body string) error {
	return r.call(daemon.OpPullEdit, daemon.Args{Instance: instance, Owner: owner, Repo: repo, Index: index, Title: title, Body: body}, nil)
}

func (r *remote) CreateRelease(instance, owner, repo, tag, name, body, target string, draft, prerelease bool) (*forgejo.Release, error) {
	var x forgejo.Release
	if err := r.call(daemon.OpReleaseNew, daemon.Args{Instance: instance, Owner: owner, Repo: repo, Tag: tag, Name: name, Body: body, Target: target, Draft: draft, Prerelease: prerelease}, &x); err != nil {
		return nil, err
	}
	return &x, nil
}

func (r *remote) RepoReleases(instance, owner, repo string) ([]forgejo.Release, error) {
	var v []forgejo.Release
	return v, r.call(daemon.OpReleaseList, daemon.Args{Instance: instance, Owner: owner, Repo: repo}, &v)
}

func (r *remote) DeleteRelease(instance, owner, repo string, id int64) error {
	return r.call(daemon.OpReleaseDel, daemon.Args{Instance: instance, Owner: owner, Repo: repo, ID: id}, nil)
}

func (r *remote) CreateTag(instance, owner, repo, tag, target string) error {
	return r.call(daemon.OpTagNew, daemon.Args{Instance: instance, Owner: owner, Repo: repo, Tag: tag, Target: target}, nil)
}

func (r *remote) DeleteTag(instance, owner, repo, tag string) error {
	return r.call(daemon.OpTagDel, daemon.Args{Instance: instance, Owner: owner, Repo: repo, Tag: tag}, nil)
}

func (r *remote) RenameRepo(instance, owner, repo, newName string) (*forgejo.Repo, error) {
	var x forgejo.Repo
	if err := r.call(daemon.OpRepoRename, daemon.Args{Instance: instance, Owner: owner, Repo: repo, Name: newName}, &x); err != nil {
		return nil, err
	}
	return &x, nil
}

func (r *remote) SetRepoVisibility(instance, owner, repo string, private bool) (*forgejo.Repo, error) {
	var x forgejo.Repo
	if err := r.call(daemon.OpRepoVis, daemon.Args{Instance: instance, Owner: owner, Repo: repo, Private: private}, &x); err != nil {
		return nil, err
	}
	return &x, nil
}

func (r *remote) ArchiveRepo(instance, owner, repo string, archived bool) (*forgejo.Repo, error) {
	var x forgejo.Repo
	if err := r.call(daemon.OpRepoArchive, daemon.Args{Instance: instance, Owner: owner, Repo: repo, Archived: archived}, &x); err != nil {
		return nil, err
	}
	return &x, nil
}

func (r *remote) SetDefaultBranch(instance, owner, repo, branch string) (*forgejo.Repo, error) {
	var x forgejo.Repo
	if err := r.call(daemon.OpRepoDefBranch, daemon.Args{Instance: instance, Owner: owner, Repo: repo, Branch: branch}, &x); err != nil {
		return nil, err
	}
	return &x, nil
}

func (r *remote) RepoLabels(instance, owner, repo string) ([]forgejo.Label, error) {
	var v []forgejo.Label
	return v, r.call(daemon.OpLabelList, daemon.Args{Instance: instance, Owner: owner, Repo: repo}, &v)
}

func (r *remote) RepoMilestones(instance, owner, repo string) ([]forgejo.Milestone, error) {
	var v []forgejo.Milestone
	return v, r.call(daemon.OpMilestoneList, daemon.Args{Instance: instance, Owner: owner, Repo: repo}, &v)
}

func (r *remote) SetIssueLabels(instance, owner, repo string, index int, labels []string) error {
	return r.call(daemon.OpIssueLabels, daemon.Args{Instance: instance, Owner: owner, Repo: repo, Index: index, Labels: labels}, nil)
}

func (r *remote) SetIssueAssignees(instance, owner, repo string, index int, assignees []string) error {
	return r.call(daemon.OpIssueAssign, daemon.Args{Instance: instance, Owner: owner, Repo: repo, Index: index, Assignees: assignees}, nil)
}

func (r *remote) SetIssueMilestone(instance, owner, repo string, index int, milestoneID int64) error {
	return r.call(daemon.OpIssueMilestn, daemon.Args{Instance: instance, Owner: owner, Repo: repo, Index: index, MilestoneID: milestoneID}, nil)
}

// --- Git local (workflow dev) : branches, restore, log, stash (sans PIN) ---

func (r *remote) GitCreateBranch(path, name string) (string, error) {
	return r.callStr(daemon.OpGitBranchNew, "output", daemon.Args{Path: path, Name: name})
}
func (r *remote) GitSwitchBranch(path, name string) (string, error) {
	return r.callStr(daemon.OpGitSwitch, "output", daemon.Args{Path: path, Name: name})
}
func (r *remote) GitDiscard(path, file string) (string, error) {
	return r.callStr(daemon.OpGitDiscard, "output", daemon.Args{Path: path, File: file})
}
func (r *remote) GitLog(path string, limit int) ([]gitx.LocalCommit, error) {
	var v []gitx.LocalCommit
	return v, r.call(daemon.OpGitLog, daemon.Args{Path: path, Limit: limit}, &v)
}
func (r *remote) GitStashSave(path, message string) (string, error) {
	return r.callStr(daemon.OpGitStashSave, "output", daemon.Args{Path: path, Message: message})
}
func (r *remote) GitStashList(path string) (string, error) {
	return r.callStr(daemon.OpGitStashList, "output", daemon.Args{Path: path})
}
func (r *remote) GitStashPop(path string) (string, error) {
	return r.callStr(daemon.OpGitStashPop, "output", daemon.Args{Path: path})
}

// --- Git « complet » (v1.6) : merge/rebase/cherry-pick/revert/reset/amend/staging/tag + push réécrit ---

func (r *remote) GitMerge(path, ref string) (string, error) {
	return r.callStr(daemon.OpGitMerge, "output", daemon.Args{Path: path, Ref: ref})
}
func (r *remote) GitMergeAbort(path string) (string, error) {
	return r.callStr(daemon.OpGitMergeAbort, "output", daemon.Args{Path: path})
}
func (r *remote) GitRebase(path, upstream string) (string, error) {
	return r.callStr(daemon.OpGitRebase, "output", daemon.Args{Path: path, Ref: upstream})
}
func (r *remote) GitRebaseContinue(path string) (string, error) {
	return r.callStr(daemon.OpGitRebaseCont, "output", daemon.Args{Path: path})
}
func (r *remote) GitRebaseAbort(path string) (string, error) {
	return r.callStr(daemon.OpGitRebaseAbort, "output", daemon.Args{Path: path})
}
func (r *remote) GitCherryPick(path, commit string) (string, error) {
	return r.callStr(daemon.OpGitCherryPick, "output", daemon.Args{Path: path, Ref: commit})
}
func (r *remote) GitCherryPickContinue(path string) (string, error) {
	return r.callStr(daemon.OpGitCherryCont, "output", daemon.Args{Path: path})
}
func (r *remote) GitCherryPickAbort(path string) (string, error) {
	return r.callStr(daemon.OpGitCherryAbort, "output", daemon.Args{Path: path})
}
func (r *remote) GitRevert(path, commit string) (string, error) {
	return r.callStr(daemon.OpGitRevert, "output", daemon.Args{Path: path, Ref: commit})
}
func (r *remote) GitReset(path, mode, ref string) (string, error) {
	return r.callStr(daemon.OpGitReset, "output", daemon.Args{Path: path, Method: mode, Ref: ref})
}
func (r *remote) GitAmend(path, message string) (string, error) {
	return r.callStr(daemon.OpGitAmend, "output", daemon.Args{Path: path, Message: message})
}
func (r *remote) GitCommitPaths(path, message string, paths []string) (string, error) {
	return r.callStr(daemon.OpGitCommitPaths, "output", daemon.Args{Path: path, Message: message, Paths: paths})
}
func (r *remote) GitTagCreate(path, name, message string) (string, error) {
	return r.callStr(daemon.OpGitTagCreate, "output", daemon.Args{Path: path, Tag: name, Message: message})
}
func (r *remote) GitPushForce(instance, path string) (string, error) {
	return r.callStr(daemon.OpGitPushForce, "output", daemon.Args{Instance: instance, Path: path})
}
func (r *remote) GitPushTag(instance, path, tag string) (string, error) {
	return r.callStr(daemon.OpGitPushTag, "output", daemon.Args{Instance: instance, Path: path, Tag: tag})
}

// --- Projets atelier↔production (v1.7.1) : lecture ---

func (r *remote) Projects() ([]config.Project, error) {
	var v []config.Project
	return v, r.call(daemon.OpProjectList, daemon.Args{}, &v)
}
func (r *remote) ProjectStatus(name string) (*core.ProjectView, error) {
	var x core.ProjectView
	if err := r.call(daemon.OpProjectStatus, daemon.Args{Name: name}, &x); err != nil {
		return nil, err
	}
	return &x, nil
}
func (r *remote) ProjectPromote(name string, prodIndex int, sourceBranch, version, targetBranch string) (*core.PromoteResult, error) {
	var x core.PromoteResult
	if err := r.call(daemon.OpProjectPromote, daemon.Args{Name: name, Index: prodIndex, Head: sourceBranch, Tag: version, Base: targetBranch}, &x); err != nil {
		return nil, err
	}
	return &x, nil
}
