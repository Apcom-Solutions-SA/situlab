// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package daemon

// Op* sont les opérations reconnues par le démon (partagées avec le client CLI).
const (
	OpDoctor        = "doctor"
	OpStatus        = "status"
	OpUnlock        = "unlock"
	OpLock          = "lock"
	OpInstanceList  = "instance.list"
	OpInstanceAdd   = "instance.add"
	OpInstanceLogin = "instance.login"
	OpInstanceRm    = "instance.remove"
	OpInstanceDef   = "instance.default"
	OpInstanceTest  = "instance.test"
	OpWhoami        = "whoami"
	OpDashboard     = "dashboard"
	OpRepoList      = "repo.list"
	OpRepoView      = "repo.view"
	OpRepoSetDesc   = "repo.setdesc"
	OpRepoBranches  = "repo.branches"
	OpRepoTags      = "repo.tags"
	OpRepoCommits   = "repo.commits"
	OpRepoTree      = "repo.tree"
	OpRepoFile      = "repo.file"
	OpOrgList       = "org.list"
	OpTokensList    = "tokens.list"
	OpClone         = "clone"
	OpFetch         = "fetch"
	OpPull          = "pull"
	OpCommit        = "commit"
	OpDiff          = "diff"
	OpPush          = "push"
	OpSync          = "sync"
	OpRepoCreate    = "repo.create"
	OpRepoDelete    = "repo.delete"
	OpIssueList     = "issue.list"
	OpIssueCreate   = "issue.create"
	OpIssueComment  = "issue.comment"
	OpIssueState    = "issue.state"
	OpIssueEdit     = "issue.edit"
	OpPullCreate    = "pull.create"
	OpPullMerge     = "pull.merge"
	OpPullList      = "pull.list"
	OpPullDetail    = "pull.detail"
	OpPullDiff      = "pull.diff"
	OpPullFiles     = "pull.files"
	OpPullReview    = "pull.review"
	OpPullState     = "pull.state"
	OpPullEdit      = "pull.edit"
	OpReleaseList   = "release.list"
	OpReleaseNew    = "release.create"
	OpReleaseDel    = "release.delete"
	OpTagNew        = "tag.create"
	OpTagDel        = "tag.delete"
	OpRepoRename    = "repo.rename"
	OpRepoVis       = "repo.visibility"
	OpRepoArchive   = "repo.archive"
	OpRepoDefBranch = "repo.default-branch"
	OpRepoDelBranch = "repo.delete-branch"
	OpLabelList     = "label.list"
	OpMilestoneList = "milestone.list"
	OpIssueLabels   = "issue.labels"
	OpIssueAssign   = "issue.assignees"
	OpIssueMilestn  = "issue.milestone"
	OpLocalAdd      = "local.add"
	OpLocalList     = "local.list"
	OpLocalStatus   = "local.status"
	OpLocalRemove   = "local.remove"
	OpGitBranchNew  = "git.branch-create"
	OpGitSwitch     = "git.switch"
	OpGitBranchDel  = "git.branch-delete" // suppression LOCALE (pas de PIN ; git refuse une branche non fusionnée sans force)
	OpGitDiscard    = "git.discard"
	OpGitLog        = "git.log"
	OpGitStashSave  = "git.stash-save"
	OpGitStashList  = "git.stash-list"
	OpGitStashPop   = "git.stash-pop"
	// Git « complet » (v1.6) — gestes de dev avancés (CLI/MCP). Locaux : pas de PIN.
	OpGitMerge       = "git.merge"
	OpGitMergeAbort  = "git.merge-abort"
	OpGitRebase      = "git.rebase"
	OpGitRebaseCont  = "git.rebase-continue"
	OpGitRebaseAbort = "git.rebase-abort"
	OpGitCherryPick  = "git.cherry-pick"
	OpGitCherryCont  = "git.cherry-pick-continue"
	OpGitCherryAbort = "git.cherry-pick-abort"
	OpGitRevert      = "git.revert"
	OpGitReset       = "git.reset"
	OpGitAmend       = "git.amend"
	OpGitCommitPaths = "git.commit-paths"
	OpGitTagCreate   = "git.tag-create" // tag LOCAL (publication = OpGitPushTag)
	// Worktrees (v1.9.8) — plusieurs arbres de travail depuis un dépôt (locaux : pas de PIN).
	OpGitWorktreeList = "git.worktree-list"
	OpGitWorktreeAdd  = "git.worktree-add"
	OpGitWorktreeRm   = "git.worktree-remove"
	OpGitWorktreePrn  = "git.worktree-prune"
	// Gestes distants réécrits — PIN requis (même portillon que push/sync).
	OpGitPushForce = "git.push-force"
	OpGitPushTag   = "git.push-tag"
	OpAdminUsers   = "admin.users"
	OpDirectory    = "admin.directory"
	OpAdminUserNew = "admin.user-create"
	OpTokenMgrNew  = "tokenmgr.create"
	OpTokenMgrDel  = "tokenmgr.delete"
	OpPINSet       = "pin.set"
	// Projets atelier↔production (v1.7.1) — lecture (la création reste en fenêtre).
	OpProjectList    = "project.list"
	OpProjectLight   = "project.light" // carte IA : dépôts + liens local↔atelier↔vitrine + kernel/spoke/tag
	OpProjectStatus  = "project.status"
	OpProjectPromote = "project.promote" // publication atelier→prod (PIN)
	// Signature de garantie « SituLab approved » (lecture libre ; génération/installation sous session).
	OpAttestStatus = "attest.status"
	OpAttestGenKey = "attest.genkey"
	OpAttestCert   = "attest.install-cert"
	// Clouds de fichiers souverains (CORPUS, v1.10). Connexion + exploration + gestes de fichiers.
	// L'allowlist de dossiers (que l'IA voit) est appliquée ICI, à la frontière agent ; la fenêtre
	// (humain, via app.go in-process) n'y est pas soumise. Les réglages allowlist/politique PIN
	// restent FENÊTRE UNIQUEMENT (aucune Op).
	OpCloudConns   = "cloud.connections" // liste des connexions cloud (sans secret)
	OpCloudAdd     = "cloud.add"
	OpCloudRemove  = "cloud.remove"
	OpCloudDef     = "cloud.default"
	OpCloudAllowed = "cloud.allowed" // dossiers autorisés à l'IA pour une connexion
	OpCloudPolicy  = "cloud.policy"  // niveaux de droits résolus (write/delete/rights) pour un chemin (lecture, v1.18.2)
	OpCloudList    = "cloud.list"    // contenu d'un dossier
	OpCloudGet     = "cloud.get"     // télécharger (octets)
	OpCloudRead    = "cloud.read"    // lire le texte d'un fichier
	OpCloudMkdir   = "cloud.mkdir"
	OpCloudUpload  = "cloud.upload" // envoyer un fichier local
	OpCloudWrite   = "cloud.write"  // écrire/remplacer un fichier (contenu fourni)
	OpCloudDelete  = "cloud.delete"
	// Sauvegardes git → cloud (v1.10).
	OpBackupList   = "backup.list"
	OpBackupAdd    = "backup.add"
	OpBackupRemove = "backup.remove"
	OpBackupRun    = "backup.run"
	// KERNEL (v1.11) : la mémoire, lisible. Lecture (liste, état, carte du contenu) + gestes mémoire
	// (récupérer, enregistrer). La DÉSIGNATION (ajout/retrait/actif) reste FENÊTRE UNIQUEMENT (aucune Op).
	OpKernelList     = "kernel.list"
	OpKernelStatus   = "kernel.status"
	OpKernelContents = "kernel.contents"
	OpKernelPull     = "kernel.pull"
	OpKernelSave     = "kernel.save" // récupérer → enregistrer → envoyer (PIN, même portillon que push)
	// SPOKES (v1.11) : projections de mission. Désignation réservée à la fenêtre (aucune Op).
	OpSpokeList     = "spoke.list"
	OpSpokeStatus   = "spoke.status"
	OpSpokeContents = "spoke.contents"
	OpSpokePull     = "spoke.pull"
	OpSpokeSave     = "spoke.save"
	// Journal local des opérations (1.20) : lecture + réglage du niveau + purge. Pas de PIN (réglage
	// local, aucun secret). journal.level sans valeur = lecture du niveau courant.
	OpJournalRead  = "journal.read"
	OpJournalLevel = "journal.level"
	OpJournalClear = "journal.clear"
)
