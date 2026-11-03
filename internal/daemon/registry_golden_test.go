// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package daemon

import (
	"testing"

	"situlab/internal/config"
)

// allOps : toutes les opérations reconnues par le démon. Sert de contrat de complétude : une op ajoutée
// sans entrée golden fait échouer TestOpClassificationGolden.
var allOps = []string{
	OpDoctor, OpStatus, OpUnlock, OpLock, OpInstanceList, OpInstanceAdd, OpInstanceLogin, OpInstanceRm,
	OpInstanceDef, OpInstanceTest, OpWhoami, OpDashboard, OpRepoList, OpRepoView, OpRepoSetDesc, OpRepoBranches,
	OpRepoTags, OpRepoCommits, OpRepoTree, OpRepoFile, OpOrgList, OpTokensList, OpClone, OpFetch, OpPull,
	OpCommit, OpDiff, OpPush, OpSync, OpRepoCreate, OpRepoDelete, OpIssueList, OpIssueCreate, OpIssueComment,
	OpIssueState, OpIssueEdit, OpPullCreate, OpPullMerge, OpPullList, OpPullDetail, OpPullDiff, OpPullFiles,
	OpPullReview, OpPullState, OpPullEdit, OpReleaseList, OpReleaseNew, OpReleaseDel, OpTagNew, OpTagDel,
	OpRepoRename, OpRepoVis, OpRepoArchive, OpRepoDefBranch, OpRepoDelBranch, OpLabelList, OpMilestoneList,
	OpIssueLabels, OpIssueAssign, OpIssueMilestn, OpLocalAdd, OpLocalList, OpLocalStatus, OpLocalRemove,
	OpGitBranchNew, OpGitSwitch, OpGitBranchDel, OpGitDiscard, OpGitLog, OpGitStashSave, OpGitStashList,
	OpGitStashPop, OpGitMerge, OpGitMergeAbort, OpGitRebase, OpGitRebaseCont, OpGitRebaseAbort, OpGitCherryPick,
	OpGitCherryCont, OpGitCherryAbort, OpGitRevert, OpGitReset, OpGitAmend, OpGitCommitPaths, OpGitTagCreate,
	OpGitWorktreeList, OpGitWorktreeAdd, OpGitWorktreeRm, OpGitWorktreePrn, OpGitPushForce, OpGitPushTag,
	OpAdminUsers, OpDirectory, OpAdminUserNew, OpTokenMgrNew, OpTokenMgrDel, OpPINSet, OpProjectList,
	OpProjectLight, OpProjectStatus, OpProjectPromote, OpAttestStatus, OpAttestGenKey, OpAttestCert,
	OpCloudConns, OpCloudAdd, OpCloudRemove, OpCloudDef, OpCloudAllowed, OpCloudPolicy, OpCloudList, OpCloudGet,
	OpCloudRead, OpCloudMkdir, OpCloudUpload, OpCloudWrite, OpCloudDelete, OpBackupList, OpBackupAdd,
	OpBackupRemove, OpBackupRun, OpKernelList, OpKernelStatus, OpKernelContents, OpKernelPull, OpKernelSave,
	OpSpokeList, OpSpokeStatus, OpSpokeContents, OpSpokePull, OpSpokeSave,
	OpJournalRead, OpJournalLevel, OpJournalClear,
}

type cls struct {
	write  bool
	action config.PINAction
	label  string
}

// goldenCls : classification ATTENDUE, capturée sur le comportement d'avant le registre (les trois
// switch isWriteOp/actionOf/actionLabel). Le refactor doit la reproduire à l'identique.
var goldenCls = map[string]cls{
	"admin.directory":          {false, "user_admin", "admin"},
	"admin.user-create":        {true, "user_admin", "create"},
	"admin.users":              {false, "user_admin", "admin"},
	"attest.genkey":            {false, "user_admin", "admin"},
	"attest.install-cert":      {false, "user_admin", "admin"},
	"attest.status":            {false, "user_admin", "admin"},
	"backup.add":               {false, "user_admin", "admin"},
	"backup.list":              {false, "user_admin", "admin"},
	"backup.remove":            {false, "user_admin", "admin"},
	"backup.run":               {true, "user_admin", "admin"},
	"clone":                    {false, "user_admin", "admin"},
	"cloud.add":                {false, "user_admin", "admin"},
	"cloud.allowed":            {false, "user_admin", "admin"},
	"cloud.connections":        {false, "user_admin", "admin"},
	"cloud.default":            {false, "user_admin", "admin"},
	"cloud.delete":             {true, "user_admin", "admin"},
	"cloud.get":                {false, "user_admin", "admin"},
	"cloud.list":               {false, "user_admin", "admin"},
	"cloud.mkdir":              {true, "user_admin", "admin"},
	"cloud.policy":             {false, "user_admin", "admin"},
	"cloud.read":               {false, "user_admin", "admin"},
	"cloud.remove":             {false, "user_admin", "admin"},
	"cloud.upload":             {true, "user_admin", "admin"},
	"cloud.write":              {true, "user_admin", "admin"},
	"commit":                   {false, "user_admin", "admin"},
	"dashboard":                {false, "user_admin", "admin"},
	"diff":                     {false, "user_admin", "admin"},
	"doctor":                   {false, "user_admin", "admin"},
	"fetch":                    {false, "user_admin", "admin"},
	"git.amend":                {false, "user_admin", "admin"},
	"git.branch-create":        {false, "user_admin", "admin"},
	"git.branch-delete":        {false, "user_admin", "admin"},
	"git.cherry-pick":          {false, "user_admin", "admin"},
	"git.cherry-pick-abort":    {false, "user_admin", "admin"},
	"git.cherry-pick-continue": {false, "user_admin", "admin"},
	"git.commit-paths":         {false, "user_admin", "admin"},
	"git.discard":              {false, "user_admin", "admin"},
	"git.log":                  {false, "user_admin", "admin"},
	"git.merge":                {false, "user_admin", "admin"},
	"git.merge-abort":          {false, "user_admin", "admin"},
	"git.push-force":           {true, "push", "push"},
	"git.push-tag":             {true, "push", "push"},
	"git.rebase":               {false, "user_admin", "admin"},
	"git.rebase-abort":         {false, "user_admin", "admin"},
	"git.rebase-continue":      {false, "user_admin", "admin"},
	"git.reset":                {false, "user_admin", "admin"},
	"git.revert":               {false, "user_admin", "admin"},
	"git.stash-list":           {false, "user_admin", "admin"},
	"git.stash-pop":            {false, "user_admin", "admin"},
	"git.stash-save":           {false, "user_admin", "admin"},
	"git.switch":               {false, "user_admin", "admin"},
	"git.tag-create":           {false, "user_admin", "admin"},
	"git.worktree-add":         {false, "user_admin", "admin"},
	"git.worktree-list":        {false, "user_admin", "admin"},
	"git.worktree-prune":       {false, "user_admin", "admin"},
	"git.worktree-remove":      {false, "user_admin", "admin"},
	"instance.add":             {false, "user_admin", "admin"},
	"instance.default":         {true, "user_admin", "admin"},
	"instance.list":            {false, "user_admin", "admin"},
	"instance.login":           {false, "user_admin", "admin"},
	"instance.remove":          {false, "user_admin", "admin"},
	"instance.test":            {false, "user_admin", "admin"},
	"issue.assignees":          {true, "issues", "issue"},
	"issue.comment":            {true, "issues", "issue"},
	"issue.create":             {true, "issues", "issue"},
	"issue.edit":               {true, "issues", "issue"},
	"issue.labels":             {true, "issues", "issue"},
	"issue.list":               {false, "user_admin", "admin"},
	"issue.milestone":          {true, "issues", "issue"},
	"issue.state":              {true, "issues", "issue"},
	"journal.clear":            {false, "user_admin", "admin"},
	"journal.level":            {false, "user_admin", "admin"},
	"journal.read":             {false, "user_admin", "admin"},
	"kernel.contents":          {false, "user_admin", "admin"},
	"kernel.list":              {false, "user_admin", "admin"},
	"kernel.pull":              {false, "user_admin", "admin"},
	"kernel.save":              {true, "push", "push"},
	"kernel.status":            {false, "user_admin", "admin"},
	"label.list":               {false, "user_admin", "admin"},
	"local.add":                {false, "user_admin", "admin"},
	"local.list":               {false, "user_admin", "admin"},
	"local.remove":             {false, "user_admin", "admin"},
	"local.status":             {false, "user_admin", "admin"},
	"lock":                     {false, "user_admin", "admin"},
	"milestone.list":           {false, "user_admin", "admin"},
	"org.list":                 {false, "user_admin", "admin"},
	"pin.set":                  {false, "user_admin", "admin"},
	"project.light":            {false, "user_admin", "admin"},
	"project.list":             {false, "user_admin", "admin"},
	"project.promote":          {true, "push", "promote"},
	"project.status":           {false, "user_admin", "admin"},
	"pull":                     {false, "user_admin", "admin"},
	"pull.create":              {true, "issues", "issue"},
	"pull.detail":              {false, "user_admin", "admin"},
	"pull.diff":                {false, "user_admin", "admin"},
	"pull.edit":                {true, "issues", "issue"},
	"pull.files":               {false, "user_admin", "admin"},
	"pull.list":                {false, "user_admin", "admin"},
	"pull.merge":               {true, "issues", "issue"},
	"pull.review":              {true, "issues", "issue"},
	"pull.state":               {true, "issues", "issue"},
	"push":                     {true, "push", "push"},
	"release.create":           {true, "repo_admin", "create"},
	"release.delete":           {true, "repo_admin", "delete"},
	"release.list":             {false, "user_admin", "admin"},
	"repo.archive":             {true, "repo_admin", "config"},
	"repo.branches":            {false, "user_admin", "admin"},
	"repo.commits":             {false, "user_admin", "admin"},
	"repo.create":              {true, "repo_admin", "create"},
	"repo.default-branch":      {true, "repo_admin", "config"},
	"repo.delete":              {true, "repo_admin", "delete"},
	"repo.delete-branch":       {true, "repo_admin", "delete"},
	"repo.file":                {false, "user_admin", "admin"},
	"repo.list":                {false, "user_admin", "admin"},
	"repo.rename":              {true, "repo_admin", "config"},
	"repo.setdesc":             {true, "repo_admin", "config"},
	"repo.tags":                {false, "user_admin", "admin"},
	"repo.tree":                {false, "user_admin", "admin"},
	"repo.view":                {false, "user_admin", "admin"},
	"repo.visibility":          {true, "repo_admin", "config"},
	"spoke.contents":           {false, "user_admin", "admin"},
	"spoke.list":               {false, "user_admin", "admin"},
	"spoke.pull":               {false, "user_admin", "admin"},
	"spoke.save":               {true, "push", "push"},
	"spoke.status":             {false, "user_admin", "admin"},
	"status":                   {false, "user_admin", "admin"},
	"sync":                     {true, "push", "push"},
	"tag.create":               {true, "repo_admin", "create"},
	"tag.delete":               {true, "repo_admin", "delete"},
	"tokenmgr.create":          {true, "user_admin", "create"},
	"tokenmgr.delete":          {true, "user_admin", "delete"},
	"tokens.list":              {false, "user_admin", "admin"},
	"unlock":                   {false, "user_admin", "admin"},
	"whoami":                   {false, "user_admin", "admin"},
}

// TestOpClassificationGolden : le registre reproduit EXACTEMENT l'ancienne classification (isWriteOp,
// actionOf, actionLabel) pour toutes les opérations. Fige le comportement avant toute évolution.
func TestOpClassificationGolden(t *testing.T) {
	for _, op := range allOps {
		want, ok := goldenCls[op]
		if !ok {
			t.Errorf("op %q absente du golden (ajoute-la à goldenCls)", op)
			continue
		}
		if got := isWriteOp(op); got != want.write {
			t.Errorf("isWriteOp(%q) = %v, golden %v", op, got, want.write)
		}
		if got := actionOf(op); got != want.action {
			t.Errorf("actionOf(%q) = %q, golden %q", op, got, want.action)
		}
		if got := actionLabel(op); got != want.label {
			t.Errorf("actionLabel(%q) = %q, golden %q", op, got, want.label)
		}
	}
	if len(goldenCls) != len(allOps) {
		t.Errorf("golden %d entrées, allOps %d : doivent coïncider (op ajoutée ?)", len(goldenCls), len(allOps))
	}
}

// TestOpRegistryWithinAllOps : aucune entrée du registre ne vise une op inconnue (clé mal tapée).
func TestOpRegistryWithinAllOps(t *testing.T) {
	known := make(map[string]bool, len(allOps))
	for _, op := range allOps {
		known[op] = true
	}
	for op := range opRegistry {
		if !known[op] {
			t.Errorf("opRegistry contient %q, absente d'allOps (clé erronée ?)", op)
		}
	}
}

// TestSignalsGitChange : le signal « git:changed » reste exactement l'ancien ensemble (tout write + clone).
func TestSignalsGitChange(t *testing.T) {
	for _, op := range allOps {
		want := goldenCls[op].write || op == OpClone
		if got := signalsGitChange(op); got != want {
			t.Errorf("signalsGitChange(%q) = %v, attendu %v", op, got, want)
		}
	}
}
