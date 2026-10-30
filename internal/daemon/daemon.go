// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

// Package daemon est le PROCESSUS RÉSIDENT de SituLab : il détient la session
// déverrouillée (la DEK du trousseau) et exécute, pour le compte des clients CLI
// one-shot, les opérations du noyau. Il écoute sur le socket local (package ipc).
//
// Rôle de sécurité : le PIN ne transite jamais par le socket. Quand une opération l'exige
// — déverrouiller pour lire, confirmer pour écrire — le démon le demande à l'humain via un
// PINPrompter (dans l'app Wails : une fenêtre de saisie qui surgit sur le bureau). Le
// client (souvent un agent) reçoit seulement « fait » ou « refusé ».
package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"situlab/internal/config"
	"situlab/internal/core"
	"situlab/internal/ipc"
)

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
)

// Args porte les paramètres de toutes les opérations (seuls les champs utiles sont remplis).
type Args struct {
	Instance    string   `json:"instance,omitempty"`
	Owner       string   `json:"owner,omitempty"`
	Repo        string   `json:"repo,omitempty"`
	Ref         string   `json:"ref,omitempty"`
	Limit       int      `json:"limit,omitempty"`
	Path        string   `json:"path,omitempty"`
	Dir         string   `json:"dir,omitempty"`
	Message     string   `json:"message,omitempty"`
	Name        string   `json:"name,omitempty"`
	URL         string   `json:"url,omitempty"`
	Token       string   `json:"token,omitempty"`
	Desc        string   `json:"desc,omitempty"`
	Private     bool     `json:"private,omitempty"`
	Title       string   `json:"title,omitempty"`
	Body        string   `json:"body,omitempty"`
	State       string   `json:"state,omitempty"`
	User        string   `json:"user,omitempty"`
	Password    string   `json:"password,omitempty"`
	OTP         string   `json:"otp,omitempty"`
	Scopes      []string `json:"scopes,omitempty"`
	Repos       []string `json:"repos,omitempty"`
	ID          int64    `json:"id,omitempty"`
	Email       string   `json:"email,omitempty"`
	Provider    string   `json:"provider,omitempty"` // instance.add : "forgejo" (défaut) | "github"
	Index       int      `json:"index,omitempty"`    // numéro d'issue/PR
	Head        string   `json:"head,omitempty"`     // pull.create : branche source
	Base        string   `json:"base,omitempty"`     // pull.create : branche cible
	Method      string   `json:"method,omitempty"`   // pull.merge : merge|rebase|squash
	Mode        string   `json:"mode,omitempty"`     // project.promote : code|tag|release
	Event       string   `json:"event,omitempty"`    // pull.review : APPROVED|REQUEST_CHANGES|COMMENT
	Tag         string   `json:"tag,omitempty"`      // release/tag
	Target      string   `json:"target,omitempty"`   // release/tag : branche/commit cible
	Draft       bool     `json:"draft,omitempty"`
	Prerelease  bool     `json:"prerelease,omitempty"`
	Archived    bool     `json:"archived,omitempty"`
	Branch      string   `json:"branch,omitempty"`    // repo.default-branch
	Labels      []string `json:"labels,omitempty"`    // issue.labels (noms)
	Assignees   []string `json:"assignees,omitempty"` // issue.assignees (logins)
	MilestoneID int64    `json:"milestone_id,omitempty"`
	File        string   `json:"file,omitempty"`  // git.discard : fichier précis (vide = tout)
	Paths       []string `json:"paths,omitempty"` // git.commit-paths : staging sélectif
	Admin       bool     `json:"admin,omitempty"`
	AdminScope  bool     `json:"admin_scope,omitempty"` // onboarding : demander la portée admin
	Remember    bool     `json:"remember,omitempty"`    // onboarding : mémoriser le mot de passe (chiffré par le PIN)
	Force       bool     `json:"force,omitempty"`       // git.branch-delete / git.worktree-remove : forcer malgré refus git
	Create      bool     `json:"create,omitempty"`      // git.worktree-add : créer une NOUVELLE branche (sinon extraire l'existante)
	Intent      string   `json:"intent,omitempty"`      // Point 4 : phrase libre de l'agent (« pourquoi »), AFFICHÉE telle quelle dans la fenêtre PIN, distincte des actions constatées par SituLab. N'autorise rien.
	// Clouds de fichiers (v1.10)
	Cloud   string `json:"cloud,omitempty"`   // nom de la connexion cloud
	Content string `json:"content,omitempty"` // cloud.write : contenu texte à écrire
	Local   string `json:"local,omitempty"`   // cloud.upload : chemin local source ; cloud.get/download : destination locale
	Dest    string `json:"dest,omitempty"`    // backup.add : dossier cloud de destination
}

// Status est l'état renvoyé par OpStatus.
type Status struct {
	Unlocked  bool `json:"unlocked"`
	PINSet    bool `json:"pin_set"`
	Instances int  `json:"instances"`
}

// PINOutcome décrit le RÉSULTAT d'une demande de PIN, pour que le client (souvent un agent) distingue
// clairement l'état au lieu d'un échec opaque : confirmé / refusé par l'humain / délai écoulé / pas
// d'interface pour demander. L'invariant est préservé : le PIN se saisit dans la fenêtre, jamais ici.
type PINOutcome int

const (
	PINConfirmed   PINOutcome = iota // l'humain a saisi et validé le PIN dans la fenêtre
	PINRefused                       // l'humain a annulé la fenêtre
	PINTimedOut                      // aucune réponse dans le délai de garde (la fenêtre a pu rester ouverte)
	PINUnavailable                   // aucune interface disponible pour demander le PIN
)

// PINPrompter demande le PIN à l'humain sur le poste. reason décrit l'action à autoriser.
// Renvoie (pin, PINConfirmed) si saisi, ("", …) sinon selon la cause (voir PINOutcome).
type PINPrompter interface {
	// detail porte un aperçu optionnel de l'action (ex. *core.PushPreview pour un envoi) afin
	// d'enrichir le consentement ; nil quand il n'y a rien de plus à montrer que `reason`.
	// action est une CLÉ de classe d'action (push|promote|delete|create|config|issue|admin|unlock)
	// pour que la fenêtre adapte son visuel (couleur/icône) — ce n'est jamais une autorisation en soi.
	// intent est la phrase LIBRE de l'agent (son « pourquoi »), affichée distinctement des actions que
	// SituLab constate ; vide quand l'humain agit directement. N'autorise rien non plus : pur affichage.
	PromptPIN(reason string, detail any, action, intent string) (string, PINOutcome)
}

// Daemon exécute les opérations sur un noyau partagé, avec consentement humain par modale.
type Daemon struct {
	svc      *core.Service
	prompter PINPrompter
}

// New construit le démon autour d'un noyau et d'un prompteur de PIN.
func New(svc *core.Service, prompter PINPrompter) *Daemon {
	return &Daemon{svc: svc, prompter: prompter}
}

// Serve démarre l'écoute sur le socket local.
func (d *Daemon) Serve(path string) (*ipc.Server, error) {
	return ipc.Listen(path, d.handle)
}

func (d *Daemon) handle(req ipc.Request) ipc.Response {
	var a Args
	if len(req.Args) > 0 {
		if err := json.Unmarshal(req.Args, &a); err != nil {
			return ipc.Response{OK: false, Err: "paramètres illisibles", Kind: "generic"}
		}
	}
	// Lecture seule (v1.4) : refuser une écriture AVANT de solliciter la modale PIN, pour ne pas
	// demander un PIN qui serait de toute façon rejeté. Le cœur reste la source de vérité (gatePIN
	// + assertWritable) pour toutes les surfaces et les mutations locales.
	if isWriteOp(req.Op) && d.svc.ReadOnly() {
		return ipc.Response{OK: false, Err: core.ErrReadOnly.Error(), Kind: kindOf(core.ErrReadOnly)}
	}
	data, err := d.dispatch(req.Op, a)
	if err != nil {
		return ipc.Response{OK: false, Err: err.Error(), Kind: kindOf(err)}
	}
	// Verrouillage après écriture (option) : une mutation réussie reverrouille la session,
	// pour réduire au minimum la fenêtre pendant laquelle le jeton est déchiffré en mémoire.
	if isWriteOp(req.Op) && d.svc.LockAfterWrite() {
		d.svc.Lock()
	}
	raw, merr := json.Marshal(data)
	if merr != nil {
		return ipc.Response{OK: false, Err: "résultat non sérialisable", Kind: "generic"}
	}
	return ipc.Response{OK: true, Data: raw}
}

// IsWriteOp expose isWriteOp au client CLI : savoir à l'avance qu'une opération est une MUTATION
// (donc susceptible d'ouvrir la fenêtre PIN sur le poste) permet d'avertir l'agent AVANT de bloquer.
func IsWriteOp(op string) bool { return isWriteOp(op) }

// isWriteOp liste les opérations de mutation (celles qui ont exigé le PIN). Sert au
// verrouillage-après-écriture. On exclut les flux d'installation (ajout/reconnexion
// d'instance) qui ne sont pas des écritures de contenu.
func isWriteOp(op string) bool {
	switch op {
	case OpPush, OpSync, OpRepoCreate, OpRepoDelete, OpRepoSetDesc,
		OpRepoRename, OpRepoVis, OpRepoArchive, OpRepoDefBranch, OpRepoDelBranch,
		OpIssueCreate, OpIssueComment, OpIssueState, OpIssueEdit,
		OpIssueLabels, OpIssueAssign, OpIssueMilestn,
		OpPullCreate, OpPullMerge, OpPullReview, OpPullState, OpPullEdit,
		OpReleaseNew, OpReleaseDel, OpTagNew, OpTagDel,
		OpAdminUserNew, OpTokenMgrNew, OpTokenMgrDel,
		OpGitPushForce, OpGitPushTag, OpProjectPromote,
		OpCloudMkdir, OpCloudUpload, OpCloudWrite, OpCloudDelete, OpBackupRun,
		OpKernelSave, OpSpokeSave, OpInstanceDef:
		return true
	}
	return false
}

// kindOf classe une erreur pour que le client CLI retrouve son code de sortie stable.
func kindOf(err error) string {
	switch {
	case errors.Is(err, core.ErrNoInstance):
		return "noinstance"
	case errors.Is(err, core.ErrPINNeeded):
		return "pinneeded"
	case errors.Is(err, core.ErrPINTimeout):
		return "pintimeout"
	case errors.Is(err, core.ErrPINLocked):
		return "pinlocked"
	case errors.Is(err, core.ErrNoGit):
		return "nogit"
	case errors.Is(err, core.ErrLocked):
		return "locked"
	case errors.Is(err, core.ErrCloudOperationForbidden):
		return "cloudforbidden-op"
	default:
		return "generic"
	}
}

// ensureUnlocked garantit une session de lecture : si verrouillée, la modale demande le PIN (invite
// générique de déverrouillage, pour les lectures).
func (d *Daemon) ensureUnlocked() error {
	return d.ensureUnlockedCtx("Déverrouiller SituLab", nil, "unlock", "")
}

// ensureUnlockedCtx est ensureUnlocked AVEC le contexte de l'action en attente. Quand la politique
// n'exige pas le PIN mais que la session est verrouillée, l'invite UNIQUE montre l'action + aperçu
// (Point 5 : un seul PIN quand verrouillé, et le BON message) au lieu d'un générique « Déverrouiller ».
// Le même secret déverrouille ET ouvre la session (core.Unlock→openSession) : une seule saisie.
func (d *Daemon) ensureUnlockedCtx(reason string, detail any, action, intent string) error {
	if d.svc.Unlocked() {
		return nil
	}
	if !d.svc.PINSet() {
		return errors.New("aucun PIN configuré : définissez-en un dans la fenêtre SituLab")
	}
	pin, outcome := d.prompter.PromptPIN(reason, detail, action, intent)
	switch outcome {
	case PINConfirmed:
		return d.svc.Unlock(pin)
	case PINTimedOut:
		return core.ErrPINTimeout
	default: // refusé / indisponible
		return core.ErrLocked
	}
}

// writePIN obtient le PIN d'autorisation d'une écriture EXCLUSIVEMENT via la modale sur le
// poste : le PIN ne transite jamais par le socket (invariant). Modale refusée/indisponible
// => ErrPINNeeded (code 3). C'est le point de consentement humain, non contournable par le
// client (agent) même s'il connaît le PIN.
// gate applique la politique PIN par connexion (v1.7) pour une opération d'écriture : selon le
// rôle de l'instance et sa politique, soit on demande le PIN à l'humain (writePIN, avec `reason`),
// soit la session déverrouillée suffit (ensureUnlocked, pas de PIN). Le cœur revérifie la même
// politique (fail-closed), donc sauter la modale ici ne peut jamais SUR-autoriser.
func (d *Daemon) gate(op string, a Args, reason string) (string, error) {
	act := actionOf(op)
	action := actionLabel(op)
	intent := clampIntent(a.Intent) // phrase libre de l'agent (Point 4), bornée ; affichage seul
	needPIN := d.svc.RequiresPIN(a.Instance, act)
	if act == config.ActionPush { // la politique du push suit le DÉPÔT réel, pas la connexion nommée
		needPIN = d.svc.RequiresPINForPush(a.Instance, a.Path)
	}
	var detail any
	if act == config.ActionPush && a.Path != "" { // aperçu éclairé pour un envoi/synchro
		detail = d.svc.PushPreview(a.Path)
	}
	if needPIN {
		return d.writePIN(reason, detail, action, intent)
	}
	// Politique « sans PIN » mais session verrouillée : l'invite UNIQUE de déverrouillage porte le
	// contexte de l'action (Point 5), pas un générique « Déverrouiller SituLab ».
	if err := d.ensureUnlockedCtx(reason, detail, action, intent); err != nil {
		return "", err
	}
	return "", nil
}

// clampIntent borne la phrase d'intention de l'agent (affichée dans la fenêtre PIN). On protège la
// modale d'un texte démesuré ; l'échappement du contenu (XSS) est fait côté frontend à l'affichage.
func clampIntent(s string) string {
	s = strings.TrimSpace(s)
	const max = 400
	if r := []rune(s); len(r) > max {
		return string(r[:max]) + "…"
	}
	return s
}

// actionOf classe une opération d'écriture dans une classe d'action soumise à la politique PIN.
// Défaut prudent (op inconnue) : administration utilisateur, toujours sous PIN.
func actionOf(op string) config.PINAction {
	switch op {
	case OpPush, OpSync, OpGitPushForce, OpGitPushTag, OpProjectPromote, OpKernelSave, OpSpokeSave:
		return config.ActionPush
	case OpIssueCreate, OpIssueComment, OpIssueState, OpIssueEdit,
		OpIssueLabels, OpIssueAssign, OpIssueMilestn,
		OpPullCreate, OpPullMerge, OpPullReview, OpPullState, OpPullEdit:
		return config.ActionIssues
	case OpRepoCreate, OpRepoDelete, OpRepoSetDesc,
		OpRepoRename, OpRepoVis, OpRepoArchive, OpRepoDefBranch, OpRepoDelBranch,
		OpReleaseNew, OpReleaseDel, OpTagNew, OpTagDel:
		return config.ActionRepoAdmin
	case OpAdminUserNew, OpTokenMgrNew, OpTokenMgrDel:
		return config.ActionUserAdmin
	}
	return config.ActionUserAdmin
}

// actionLabel classe une opération en clé de VISUEL pour la fenêtre PIN (plus fine que PINAction :
// distingue notamment les suppressions, à montrer en rouge). N'autorise rien : pur habillage.
func actionLabel(op string) string {
	switch op {
	case OpPush, OpSync, OpGitPushForce, OpGitPushTag, OpKernelSave, OpSpokeSave:
		return "push"
	case OpProjectPromote:
		return "promote"
	case OpRepoDelete, OpRepoDelBranch, OpReleaseDel, OpTagDel, OpTokenMgrDel:
		return "delete"
	case OpRepoCreate, OpReleaseNew, OpTagNew, OpTokenMgrNew, OpAdminUserNew:
		return "create"
	case OpRepoRename, OpRepoSetDesc, OpRepoVis, OpRepoArchive, OpRepoDefBranch:
		return "config"
	case OpIssueCreate, OpIssueComment, OpIssueState, OpIssueEdit,
		OpIssueLabels, OpIssueAssign, OpIssueMilestn,
		OpPullCreate, OpPullMerge, OpPullReview, OpPullState, OpPullEdit:
		return "issue"
	}
	return "admin"
}

func (d *Daemon) writePIN(reason string, detail any, action, intent string) (string, error) {
	if !d.svc.PINSet() {
		return "", errors.New("aucun PIN configuré : définissez-en un d'abord")
	}
	pin, outcome := d.prompter.PromptPIN(reason, detail, action, intent)
	switch outcome {
	case PINConfirmed:
		return pin, nil
	case PINTimedOut:
		return "", core.ErrPINTimeout
	default: // refusé / indisponible
		return "", core.ErrPINNeeded
	}
}

// gateCloud applique la politique PIN de la connexion CLOUD pour une mutation (même modèle que
// gate() côté git, mais sur la politique cloud write/delete). Le cœur revérifie (fail-closed).
func (d *Daemon) gateCloud(op string, a Args, finalPath, reason string) (string, error) {
	act := config.CloudWrite
	label := "create"
	if op == OpCloudDelete {
		act = config.CloudDelete
		label = "delete"
	}
	// Interdiction formelle (v1.18.2) : refus À LA FRONTIÈRE AGENT, AVANT toute fenêtre PIN. L'agent
	// ne peut même pas tenter ; l'humain, lui, agit en fenêtre via app.go (non soumis à cette borne).
	if d.svc.CloudOpForbidden(a.Cloud, finalPath, act) {
		return "", core.ErrCloudOperationForbidden
	}
	intent := clampIntent(a.Intent)
	if d.svc.CloudRequiresPINAt(a.Cloud, finalPath, act) {
		return d.writePIN(reason, nil, label, intent)
	}
	if err := d.ensureUnlockedCtx(reason, nil, label, intent); err != nil {
		return "", err
	}
	return "", nil
}

// cloudGuard impose l'allowlist de dossiers À LA FRONTIÈRE AGENT (CLI/MCP passent par le démon).
// La fenêtre (humain) appelle le cœur en direct (app.go) et n'est donc pas soumise à cette borne :
// c'est l'humain qui CHOISIT le périmètre, l'agent qui s'y tient. Fail-closed.
func (d *Daemon) cloudGuard(cloud, p string) error {
	if !d.svc.CloudPathAllowed(cloud, p) {
		return core.ErrCloudForbidden
	}
	return nil
}

// filterProjectCorpus borne le mapping CORPUS d'une carte projet à l'allowlist IA (frontière agent).
// Chaque chemin relié hors allowlist est retiré ; un lien devenu vide disparaît. Fail-closed.
func (d *Daemon) filterProjectCorpus(views []core.ProjectLightView) []core.ProjectLightView {
	for i := range views {
		if len(views[i].Corpus) == 0 {
			continue
		}
		links := views[i].Corpus[:0]
		for _, l := range views[i].Corpus {
			paths := l.Paths[:0]
			for _, p := range l.Paths {
				if d.svc.CloudPathAllowed(l.Cloud, p.Path) {
					paths = append(paths, p)
				}
			}
			if len(paths) > 0 {
				l.Paths = paths
				links = append(links, l)
			}
		}
		views[i].Corpus = links
	}
	return views
}

func (d *Daemon) dispatch(op string, a Args) (any, error) {
	switch op {

	// --- sans session ---
	case OpDoctor:
		return d.svc.Doctor(), nil
	case OpStatus:
		return Status{Unlocked: d.svc.Unlocked(), PINSet: d.svc.PINSet(), Instances: len(d.svc.Instances())}, nil
	case OpLock:
		d.svc.Lock()
		return nil, nil
	case OpUnlock:
		// Déverrouillage via la modale sur le poste : le PIN ne transite pas par le socket.
		return nil, d.ensureUnlocked()
	case OpInstanceList:
		return d.svc.Instances(), nil
	case OpInstanceDef:
		return nil, d.svc.SetDefaultInstance(a.Name)
	case OpInstanceRm:
		return nil, d.svc.RemoveInstance(a.Name)
	case OpLocalAdd:
		return nil, d.svc.AddLocalRepo(a.Path)
	case OpLocalList:
		return d.svc.LocalRepos()
	case OpLocalStatus:
		return d.svc.LocalStatus(a.Path)
	case OpAttestStatus:
		return d.svc.AttestHistory(a.Path, a.Limit) // lecture locale (pas de session requise)
	case OpAttestGenKey:
		return d.withSession(func() (any, error) {
			pub, err := d.svc.GenerateSignKey(a.Instance)
			return map[string]string{"public_key": pub}, err
		})
	case OpAttestCert:
		return d.withSession(func() (any, error) { return nil, d.svc.InstallSignCert(a.Instance, a.Body) })
	case OpLocalRemove:
		return nil, d.svc.RemoveLocalRepo(a.Path)
	case OpGitBranchNew:
		out, err := d.svc.GitCreateBranch(a.Path, a.Name)
		return mapStrErr("output", out, err)
	case OpGitSwitch:
		out, err := d.svc.GitSwitchBranch(a.Path, a.Name)
		return mapStrErr("output", out, err)
	case OpGitBranchDel:
		out, err := d.svc.GitDeleteBranch(a.Path, a.Name, a.Force)
		return mapStrErr("output", out, err)
	case OpGitDiscard:
		out, err := d.svc.GitDiscard(a.Path, a.File)
		return mapStrErr("output", out, err)
	case OpGitLog:
		return d.svc.GitLog(a.Path, a.Limit)
	case OpGitStashSave:
		out, err := d.svc.GitStashSave(a.Path, a.Message)
		return mapStrErr("output", out, err)
	case OpGitStashList:
		out, err := d.svc.GitStashList(a.Path)
		return mapStrErr("output", out, err)
	case OpGitStashPop:
		out, err := d.svc.GitStashPop(a.Path)
		return mapStrErr("output", out, err)

	// --- Git « complet » (v1.6) : gestes LOCAUX (pas de PIN, gardés par assertWritable dans le cœur) ---
	case OpGitMerge:
		out, err := d.svc.GitMerge(a.Path, a.Ref)
		return mapStrErr("output", out, err)
	case OpGitMergeAbort:
		out, err := d.svc.GitMergeAbort(a.Path)
		return mapStrErr("output", out, err)
	case OpGitRebase:
		out, err := d.svc.GitRebase(a.Path, a.Ref)
		return mapStrErr("output", out, err)
	case OpGitRebaseCont:
		out, err := d.svc.GitRebaseContinue(a.Path)
		return mapStrErr("output", out, err)
	case OpGitRebaseAbort:
		out, err := d.svc.GitRebaseAbort(a.Path)
		return mapStrErr("output", out, err)
	case OpGitCherryPick:
		out, err := d.svc.GitCherryPick(a.Path, a.Ref)
		return mapStrErr("output", out, err)
	case OpGitCherryCont:
		out, err := d.svc.GitCherryPickContinue(a.Path)
		return mapStrErr("output", out, err)
	case OpGitCherryAbort:
		out, err := d.svc.GitCherryPickAbort(a.Path)
		return mapStrErr("output", out, err)
	case OpGitRevert:
		out, err := d.svc.GitRevert(a.Path, a.Ref)
		return mapStrErr("output", out, err)
	case OpGitReset:
		out, err := d.svc.GitReset(a.Path, a.Method, a.Ref)
		return mapStrErr("output", out, err)
	case OpGitAmend:
		out, err := d.svc.GitAmend(a.Path, a.Message)
		return mapStrErr("output", out, err)
	case OpGitCommitPaths:
		out, err := d.svc.GitCommitPaths(a.Path, a.Message, a.Paths)
		return mapStrErr("output", out, err)
	case OpGitTagCreate:
		out, err := d.svc.GitTagCreate(a.Path, a.Tag, a.Message)
		return mapStrErr("output", out, err)
	case OpGitWorktreeList:
		return d.svc.GitWorktrees(a.Path)
	case OpGitWorktreeAdd:
		out, err := d.svc.GitWorktreeAdd(a.Path, a.Dir, a.Name, a.Create)
		return mapStrErr("output", out, err)
	case OpGitWorktreeRm:
		out, err := d.svc.GitWorktreeRemove(a.Path, a.Dir, a.Force)
		return mapStrErr("output", out, err)
	case OpGitWorktreePrn:
		out, err := d.svc.GitWorktreePrune(a.Path)
		return mapStrErr("output", out, err)
	case OpCommit:
		out, err := d.svc.Commit(a.Path, a.Message)
		return mapStrErr("output", out, err)
	case OpDiff:
		out, err := d.svc.Diff(a.Path)
		return mapStrErr("output", out, err)
	case OpPINSet:
		// INVARIANT : le PIN ne transite jamais par le socket. Sa définition/changement est une
		// action humaine, faite EXCLUSIVEMENT dans la fenêtre (App.SetPin, in-process). Refuser
		// ici ferme aussi l'oracle de bruteforce qu'offrirait une vérification de l'ancien PIN
		// soumis par le socket.
		return nil, errors.New("le PIN se définit dans la fenêtre SituLab (Réglages → Sécurité), jamais par la ligne de commande")

	// --- lectures : session requise (modale de déverrouillage si besoin) ---
	// EXCEPTION BOOTSTRAP ASSUMÉE : ajouter/connecter une instance crée un jeton côté serveur et ne
	// réclame qu'une SESSION déverrouillée, pas une confirmation writePIN par opération. C'est
	// l'amorçage de l'accès : l'utilisateur vient de saisir son PIN pour déverrouiller (voire de le
	// créer juste avant à l'onboarding) et fournit ses identifiants serveur ; exiger un second PIN
	// ici n'ajouterait aucune garantie. Les MUTATIONS ultérieures (push, issues, admin…) restent,
	// elles, protégées par writePIN.
	case OpInstanceAdd:
		if err := d.ensureUnlocked(); err != nil {
			return nil, err
		}
		if a.Provider != "" && a.Provider != "forgejo" {
			return nil, d.svc.AddInstanceToken(a.Name, a.Provider, a.URL, a.Token)
		}
		return nil, d.svc.AddInstance(a.Name, a.URL, a.Token)
	case OpInstanceLogin:
		if err := d.ensureUnlocked(); err != nil {
			return nil, err
		}
		return nil, d.svc.AddInstanceLogin(a.Name, a.URL, a.User, a.Password, a.OTP, a.AdminScope, a.Remember)
	case OpInstanceTest:
		return d.withSession(func() (any, error) { return d.svc.TestInstance(a.Instance) })
	case OpWhoami:
		return d.withSession(func() (any, error) { return d.svc.Whoami(a.Instance) })
	case OpDashboard:
		return d.withSession(func() (any, error) { return d.svc.Dashboard(a.Instance) })
	case OpRepoList:
		return d.withSession(func() (any, error) { return d.svc.Repos(a.Instance) })
	case OpRepoView:
		return d.withSession(func() (any, error) { return d.svc.ViewRepo(a.Instance, a.Owner, a.Repo) })
	case OpRepoSetDesc:
		pin, err := d.gate(op, a, fmt.Sprintf("Modifier la description de %s/%s", a.Owner, a.Repo))
		if err != nil {
			return nil, err
		}
		return nil, d.svc.SetRepoDescription(a.Instance, a.Owner, a.Repo, a.Desc, pin)
	case OpRepoBranches:
		return d.withSession(func() (any, error) { return d.svc.RepoBranches(a.Instance, a.Owner, a.Repo) })
	case OpRepoTags:
		return d.withSession(func() (any, error) { return d.svc.RepoTags(a.Instance, a.Owner, a.Repo) })
	case OpRepoCommits:
		return d.withSession(func() (any, error) { return d.svc.RepoCommits(a.Instance, a.Owner, a.Repo, a.Ref, a.Limit) })
	case OpRepoTree:
		return d.withSession(func() (any, error) { return d.svc.RepoContents(a.Instance, a.Owner, a.Repo, a.Path, a.Ref) })
	case OpRepoFile:
		return d.withSession(func() (any, error) {
			c, err := d.svc.RepoFile(a.Instance, a.Owner, a.Repo, a.Path, a.Ref)
			if err != nil {
				return nil, err
			}
			return map[string]string{"path": a.Path, "content": c}, nil
		})
	case OpOrgList:
		return d.withSession(func() (any, error) { return d.svc.Orgs(a.Instance) })
	case OpTokensList:
		// Auth BASIQUE (identifiants) : Forgejo refuse de lister les jetons via un jeton.
		return d.withSession(func() (any, error) { return d.svc.TokenList(a.Instance, a.User, a.Password, a.OTP) })
	case OpClone:
		return d.withSession(func() (any, error) {
			out, err := d.svc.Clone(a.Instance, a.Owner, a.Repo, a.Dir)
			return mapStrErr("output", out, err)
		})
	case OpFetch:
		return d.withSession(func() (any, error) {
			out, err := d.svc.Fetch(a.Instance, a.Path)
			return mapStrErr("output", out, err)
		})
	case OpPull:
		return d.withSession(func() (any, error) {
			out, err := d.svc.Pull(a.Instance, a.Path)
			return mapStrErr("output", out, err)
		})
	case OpIssueList:
		return d.withSession(func() (any, error) { return d.svc.RepoIssues(a.Instance, a.Owner, a.Repo, a.State) })
	case OpPullList:
		return d.withSession(func() (any, error) { return d.svc.RepoPulls(a.Instance, a.Owner, a.Repo, a.State) })
	case OpReleaseList:
		return d.withSession(func() (any, error) { return d.svc.RepoReleases(a.Instance, a.Owner, a.Repo) })
	case OpPullDetail:
		return d.withSession(func() (any, error) { return d.svc.PullDetail(a.Instance, a.Owner, a.Repo, a.Index) })
	case OpPullDiff:
		return d.withSession(func() (any, error) {
			diff, err := d.svc.PullDiff(a.Instance, a.Owner, a.Repo, a.Index)
			return mapStrErr("diff", diff, err)
		})
	case OpPullFiles:
		return d.withSession(func() (any, error) { return d.svc.PullFiles(a.Instance, a.Owner, a.Repo, a.Index) })
	case OpLabelList:
		return d.withSession(func() (any, error) { return d.svc.RepoLabels(a.Instance, a.Owner, a.Repo) })
	case OpMilestoneList:
		return d.withSession(func() (any, error) { return d.svc.RepoMilestones(a.Instance, a.Owner, a.Repo) })
	case OpAdminUsers:
		return d.withSession(func() (any, error) { return d.svc.AdminUsers(a.Instance) })
	case OpDirectory:
		return d.withSession(func() (any, error) { return d.svc.ConsolidatedDirectory() })
	case OpProjectList:
		return d.svc.Projects(), nil // config locale, pas de session requise
	case OpProjectLight:
		// Carte IA : on FILTRE le mapping CORPUS par l'allowlist IA à la frontière agent (v1.18.2),
		// miroir d'OpCloudList : un agent ne doit pas apprendre l'existence de dossiers reliés hors de
		// son périmètre. La fenêtre (app.go) voit le mapping complet (non filtré).
		return d.filterProjectCorpus(d.svc.ProjectsLight()), nil
	case OpProjectStatus:
		return d.withSession(func() (any, error) { return d.svc.ProjectStatus(a.Name) })
	case OpProjectPromote:
		// Publication vers la PRODUCTION : toujours confirmée par le PIN dans la fenêtre (le cœur
		// réapplique la politique de la connexion prod). L'IA prépare, l'humain promeut.
		pin, err := d.writePIN("Publier vers la PRODUCTION la version de l'atelier, projet « "+a.Name+" »", nil, "promote", clampIntent(a.Intent))
		if err != nil {
			return nil, err
		}
		return d.svc.ProjectPromote(a.Name, a.Index, core.PromoteOptions{SourceBranch: a.Head, Version: a.Tag, TargetBranch: a.Base, Mode: a.Mode, Notes: a.Body}, pin)

	// --- écritures : PIN confirmé EXCLUSIVEMENT par la modale sur le poste (jamais le socket) ---
	case OpPush:
		pin, err := d.gate(op, a, "Autoriser l'envoi en ligne de « "+a.Path+" »")
		if err != nil {
			return nil, err
		}
		out, perr := d.svc.Push(a.Instance, a.Path, pin)
		return mapStrErr("output", out, perr)
	case OpSync:
		pin, err := d.gate(op, a, "Autoriser la synchronisation de « "+a.Path+" » (enregistrer, récupérer, envoyer)")
		if err != nil {
			return nil, err
		}
		return d.svc.Synchronize(a.Instance, a.Path, a.Message, pin)
	case OpRepoCreate:
		pin, err := d.gate(op, a, "Autoriser la création du dépôt « "+a.Name+" »")
		if err != nil {
			return nil, err
		}
		return d.svc.CreateRepo(a.Instance, a.Owner, a.Name, a.Desc, a.Private, pin)
	case OpRepoDelete:
		pin, err := d.gate(op, a, "Autoriser la SUPPRESSION du dépôt « "+a.Owner+"/"+a.Repo+" »")
		if err != nil {
			return nil, err
		}
		return nil, d.svc.DeleteRepo(a.Instance, a.Owner, a.Repo, pin)
	case OpIssueCreate:
		pin, err := d.gate(op, a, "Autoriser la création d'une issue")
		if err != nil {
			return nil, err
		}
		return d.svc.CreateIssue(a.Instance, a.Owner, a.Repo, a.Title, a.Body, pin)
	case OpIssueComment:
		pin, err := d.gate(op, a, fmt.Sprintf("Autoriser un commentaire sur %s/%s #%d", a.Owner, a.Repo, a.Index))
		if err != nil {
			return nil, err
		}
		return nil, d.svc.CommentIssue(a.Instance, a.Owner, a.Repo, a.Index, a.Body, pin)
	case OpIssueState:
		verb := "Rouvrir"
		if a.State == "closed" {
			verb = "Fermer"
		}
		pin, err := d.gate(op, a, fmt.Sprintf("%s l'issue %s/%s #%d", verb, a.Owner, a.Repo, a.Index))
		if err != nil {
			return nil, err
		}
		return nil, d.svc.SetIssueState(a.Instance, a.Owner, a.Repo, a.Index, a.State, pin)
	case OpIssueEdit:
		pin, err := d.gate(op, a, fmt.Sprintf("Modifier l'issue %s/%s #%d", a.Owner, a.Repo, a.Index))
		if err != nil {
			return nil, err
		}
		return nil, d.svc.EditIssue(a.Instance, a.Owner, a.Repo, a.Index, a.Title, a.Body, pin)
	case OpPullCreate:
		pin, err := d.gate(op, a, fmt.Sprintf("Ouvrir une demande de fusion %s → %s sur %s/%s", a.Head, a.Base, a.Owner, a.Repo))
		if err != nil {
			return nil, err
		}
		return d.svc.CreatePull(a.Instance, a.Owner, a.Repo, a.Title, a.Head, a.Base, a.Body, pin)
	case OpPullMerge:
		pin, err := d.gate(op, a, fmt.Sprintf("Fusionner la demande de fusion %s/%s #%d", a.Owner, a.Repo, a.Index))
		if err != nil {
			return nil, err
		}
		return nil, d.svc.MergePull(a.Instance, a.Owner, a.Repo, a.Index, a.Method, pin)
	case OpPullReview:
		pin, err := d.gate(op, a, fmt.Sprintf("Déposer une revue (%s) sur %s/%s #%d", a.Event, a.Owner, a.Repo, a.Index))
		if err != nil {
			return nil, err
		}
		return nil, d.svc.ReviewPull(a.Instance, a.Owner, a.Repo, a.Index, a.Event, a.Body, pin)
	case OpPullState:
		pin, err := d.gate(op, a, fmt.Sprintf("Changer l'état de la demande de fusion %s/%s #%d", a.Owner, a.Repo, a.Index))
		if err != nil {
			return nil, err
		}
		return nil, d.svc.SetPullState(a.Instance, a.Owner, a.Repo, a.Index, a.State, pin)
	case OpPullEdit:
		pin, err := d.gate(op, a, fmt.Sprintf("Modifier la demande de fusion %s/%s #%d", a.Owner, a.Repo, a.Index))
		if err != nil {
			return nil, err
		}
		return nil, d.svc.EditPull(a.Instance, a.Owner, a.Repo, a.Index, a.Title, a.Body, pin)
	case OpReleaseNew:
		pin, err := d.gate(op, a, fmt.Sprintf("Publier la version « %s » sur %s/%s", a.Tag, a.Owner, a.Repo))
		if err != nil {
			return nil, err
		}
		return d.svc.CreateRelease(a.Instance, a.Owner, a.Repo, a.Tag, a.Name, a.Body, a.Target, a.Draft, a.Prerelease, pin)
	case OpReleaseDel:
		pin, err := d.gate(op, a, fmt.Sprintf("Supprimer une version sur %s/%s", a.Owner, a.Repo))
		if err != nil {
			return nil, err
		}
		return nil, d.svc.DeleteRelease(a.Instance, a.Owner, a.Repo, a.ID, pin)
	case OpTagNew:
		pin, err := d.gate(op, a, fmt.Sprintf("Créer le tag « %s » sur %s/%s", a.Tag, a.Owner, a.Repo))
		if err != nil {
			return nil, err
		}
		return nil, d.svc.CreateTag(a.Instance, a.Owner, a.Repo, a.Tag, a.Target, pin)
	case OpTagDel:
		pin, err := d.gate(op, a, fmt.Sprintf("Supprimer le tag « %s » sur %s/%s", a.Tag, a.Owner, a.Repo))
		if err != nil {
			return nil, err
		}
		return nil, d.svc.DeleteTag(a.Instance, a.Owner, a.Repo, a.Tag, pin)
	case OpRepoRename:
		pin, err := d.gate(op, a, fmt.Sprintf("Renommer %s/%s en « %s »", a.Owner, a.Repo, a.Name))
		if err != nil {
			return nil, err
		}
		return d.svc.EditRepoSettings(a.Instance, a.Owner, a.Repo, map[string]any{"name": a.Name}, pin)
	case OpRepoVis:
		pin, err := d.gate(op, a, fmt.Sprintf("Changer la visibilité de %s/%s", a.Owner, a.Repo))
		if err != nil {
			return nil, err
		}
		return d.svc.EditRepoSettings(a.Instance, a.Owner, a.Repo, map[string]any{"private": a.Private}, pin)
	case OpRepoArchive:
		pin, err := d.gate(op, a, fmt.Sprintf("Archiver/désarchiver %s/%s", a.Owner, a.Repo))
		if err != nil {
			return nil, err
		}
		return d.svc.EditRepoSettings(a.Instance, a.Owner, a.Repo, map[string]any{"archived": a.Archived}, pin)
	case OpRepoDefBranch:
		pin, err := d.gate(op, a, fmt.Sprintf("Changer la branche par défaut de %s/%s en « %s »", a.Owner, a.Repo, a.Branch))
		if err != nil {
			return nil, err
		}
		return d.svc.EditRepoSettings(a.Instance, a.Owner, a.Repo, map[string]any{"default_branch": a.Branch}, pin)
	case OpRepoDelBranch:
		pin, err := d.gate(op, a, fmt.Sprintf("Supprimer la branche distante « %s » de %s/%s", a.Branch, a.Owner, a.Repo))
		if err != nil {
			return nil, err
		}
		return nil, d.svc.DeleteRemoteBranch(a.Instance, a.Owner, a.Repo, a.Branch, pin)
	case OpIssueLabels:
		pin, err := d.gate(op, a, fmt.Sprintf("Modifier les étiquettes de %s/%s #%d", a.Owner, a.Repo, a.Index))
		if err != nil {
			return nil, err
		}
		return nil, d.svc.SetIssueLabels(a.Instance, a.Owner, a.Repo, a.Index, a.Labels, pin)
	case OpIssueAssign:
		pin, err := d.gate(op, a, fmt.Sprintf("Modifier les assignés de %s/%s #%d", a.Owner, a.Repo, a.Index))
		if err != nil {
			return nil, err
		}
		return nil, d.svc.SetIssueAssignees(a.Instance, a.Owner, a.Repo, a.Index, a.Assignees, pin)
	case OpIssueMilestn:
		pin, err := d.gate(op, a, fmt.Sprintf("Modifier le jalon de %s/%s #%d", a.Owner, a.Repo, a.Index))
		if err != nil {
			return nil, err
		}
		return nil, d.svc.SetIssueMilestone(a.Instance, a.Owner, a.Repo, a.Index, a.MilestoneID, pin)
	case OpAdminUserNew:
		pin, err := d.gate(op, a, "Autoriser la création de l'utilisateur « "+a.Name+" »")
		if err != nil {
			return nil, err
		}
		return d.svc.AdminCreateUser(a.Instance, a.Name, a.Email, a.Password, a.Admin, pin)
	// Jetons : forger/supprimer un jeton (droits d'écriture) passe par la politique PIN
	// (ActionUserAdmin) EN PLUS de l'auth mot de passe/2FA — un jeton d'écriture ne doit pas
	// s'obtenir sans PIN sur une simple session déverrouillée.
	case OpTokenMgrNew:
		pin, err := d.gate(op, a, "Autoriser la création d'un jeton d'accès sur « "+a.Instance+" »")
		if err != nil {
			return nil, err
		}
		tok, terr := d.svc.TokenCreate(a.Instance, a.User, a.Password, a.OTP, a.Name, a.Scopes, a.Repos, pin)
		return mapStrErr("token", tok, terr)
	case OpTokenMgrDel:
		pin, err := d.gate(op, a, "Autoriser la suppression d'un jeton d'accès sur « "+a.Instance+" »")
		if err != nil {
			return nil, err
		}
		return nil, d.svc.TokenDelete(a.Instance, a.User, a.Password, a.OTP, a.ID, pin)

	// --- Git « complet » (v1.6) : gestes DISTANTS réécrits — PIN confirmé par la modale ---
	case OpGitPushForce:
		pin, err := d.gate(op, a, "Autoriser la mise à jour forcée (sûre) de « "+a.Path+" » en ligne")
		if err != nil {
			return nil, err
		}
		out, perr := d.svc.GitPushForce(a.Instance, a.Path, pin)
		return mapStrErr("output", out, perr)
	case OpGitPushTag:
		pin, err := d.gate(op, a, "Autoriser la publication du tag « "+a.Tag+" » en ligne")
		if err != nil {
			return nil, err
		}
		out, perr := d.svc.GitPushTag(a.Instance, a.Path, a.Tag, pin)
		return mapStrErr("output", out, perr)

	// --- Clouds de fichiers souverains (CORPUS, v1.10) ---
	case OpCloudConns:
		return d.svc.FileClouds(), nil // noms/types, aucun secret
	case OpCloudAllowed:
		return map[string][]string{"dirs": d.svc.CloudAllowedDirs(a.Cloud)}, nil
	case OpCloudPolicy:
		w, del, r := d.svc.CloudEffectivePolicy(a.Cloud, a.Path)
		return map[string]string{"path": config.CloudCleanPath(a.Path), "write": string(w), "delete": string(del), "rights": string(r)}, nil
	case OpCloudAdd:
		if err := d.ensureUnlocked(); err != nil {
			return nil, err
		}
		return nil, d.svc.AddFileCloud(a.Name, a.Provider, a.URL, a.User, a.Password)
	case OpCloudRemove:
		return nil, d.svc.RemoveFileCloud(a.Name)
	case OpCloudDef:
		return nil, d.svc.SetDefaultCloud(a.Name)
	case OpCloudList:
		return d.withSession(func() (any, error) {
			if err := d.cloudGuard(a.Cloud, a.Path); err != nil {
				return nil, err
			}
			ents, err := d.svc.CloudList(a.Cloud, a.Path)
			if err != nil {
				return nil, err
			}
			// Perimetre IA : on ne divulgue pas les noms/tailles des enfants EXCLUS (denied_dirs), meme
			// quand le dossier parent est autorise. Filtrage a la frontiere agent (la fenetre liste tout).
			kept := ents[:0]
			for _, e := range ents {
				if d.svc.CloudPathAllowed(a.Cloud, e.Path) {
					kept = append(kept, e)
				}
			}
			return kept, nil
		})
	case OpCloudGet:
		return d.withSession(func() (any, error) {
			if err := d.cloudGuard(a.Cloud, a.Path); err != nil {
				return nil, err
			}
			if a.Local == "" {
				return nil, errors.New("destination locale requise")
			}
			// Cote LOCAL : fail-closed. Un agent ne peut ecrire QUE sous un depot suivi, jamais dans le
			// dossier SituLab (device.key/keyring/daemon.token) ni ailleurs sur le poste.
			if !d.svc.AgentLocalPathAllowed(a.Local) {
				return nil, errors.New("destination locale hors perimetre : uniquement sous un depot suivi (jamais les donnees SituLab)")
			}
			return nil, d.svc.CloudDownload(a.Cloud, a.Path, a.Local)
		})
	case OpCloudRead:
		return d.withSession(func() (any, error) {
			if err := d.cloudGuard(a.Cloud, a.Path); err != nil {
				return nil, err
			}
			txt, err := d.svc.CloudRead(a.Cloud, a.Path)
			return map[string]string{"content": txt}, err
		})
	case OpCloudMkdir:
		if err := d.cloudGuard(a.Cloud, a.Path); err != nil {
			return nil, err
		}
		pin, err := d.gateCloud(op, a, a.Path, "Autoriser la création du dossier « "+a.Path+" »")
		if err != nil {
			return nil, err
		}
		return nil, d.svc.CloudMkdir(a.Cloud, a.Path, pin)
	case OpCloudUpload:
		// Fichier LOCAL source : fail-closed (jamais les secrets SituLab ni hors depot suivi).
		if !d.svc.AgentLocalPathAllowed(a.Local) {
			return nil, errors.New("fichier local hors perimetre : uniquement sous un depot suivi (jamais les donnees SituLab)")
		}
		// On garde le chemin cloud FINAL (dossier + nom de base), pas seulement le dossier : sinon un
		// nom de base pointant un fichier exclu (denied_dirs) contournerait l'allowlist en ecriture.
		finalRemote := path.Join("/"+strings.Trim(config.CloudCleanPath(a.Path), "/"), filepath.Base(a.Local))
		if err := d.cloudGuard(a.Cloud, finalRemote); err != nil {
			return nil, err
		}
		pin, err := d.gateCloud(op, a, finalRemote, "Autoriser l'envoi d'un fichier vers « "+finalRemote+" »")
		if err != nil {
			return nil, err
		}
		return nil, d.svc.CloudUpload(a.Cloud, a.Local, a.Path, pin)
	case OpCloudWrite:
		if err := d.cloudGuard(a.Cloud, a.Path); err != nil {
			return nil, err
		}
		pin, err := d.gateCloud(op, a, a.Path, "Autoriser l'écriture du fichier « "+a.Path+" »")
		if err != nil {
			return nil, err
		}
		return nil, d.svc.CloudWrite(a.Cloud, a.Path, a.Content, pin)
	case OpCloudDelete:
		if err := d.cloudGuard(a.Cloud, a.Path); err != nil {
			return nil, err
		}
		pin, err := d.gateCloud(op, a, a.Path, "Autoriser la suppression de « "+a.Path+" »")
		if err != nil {
			return nil, err
		}
		return nil, d.svc.CloudDelete(a.Cloud, a.Path, pin)

	// --- Sauvegardes git → cloud (v1.10) ---
	case OpBackupList:
		return d.svc.Backups(), nil
	case OpBackupAdd:
		// Definir un plan = definir une SORTIE de donnees. A la frontiere agent (fail-closed) : le depot
		// bundle doit etre un depot suivi et la destination cloud doit etre dans l'allowlist IA — sinon un
		// agent injecte creerait un canal d'exfiltration vers un dossier arbitraire. La fenetre (humain)
		// n'est pas bornee (App.SaveBackupPlan).
		if !d.svc.AgentLocalPathAllowed(a.Path) {
			return nil, errors.New("depot hors perimetre : seul un depot suivi peut etre sauvegarde depuis un agent")
		}
		dest := "/" + strings.Trim(config.CloudCleanPath(a.Dest), "/")
		if !d.svc.CloudPathAllowed(a.Cloud, dest) {
			return nil, core.ErrCloudForbidden
		}
		return nil, d.svc.AddBackupPlan(a.Name, a.Path, a.Cloud, a.Dest)
	case OpBackupRemove:
		return nil, d.svc.RemoveBackupPlan(a.Name)
	case OpBackupRun:
		// Pas de fenêtre PIN pour une sauvegarde : c'est une routine additive (copie vers le cloud de
		// l'utilisateur) qui doit se faire toute seule. RunBackup autorise via authorizeBackupCloud
		// (session déverrouillée + hors lecture seule, sans PIN).
		out, rerr := d.svc.RunBackup(a.Name)
		return mapStrErr("path", out, rerr)

	// --- KERNEL (v1.11) : la mémoire, lisible ---
	case OpKernelList:
		return d.svc.Kernels(), nil // config locale, pas de session requise
	case OpKernelStatus:
		return d.svc.KernelStatus(a.Name) // état git LOCAL, sans réseau
	case OpKernelContents:
		return d.svc.KernelContents(a.Name) // lecture disque
	case OpKernelPull:
		return d.withSession(func() (any, error) {
			out, err := d.svc.KernelPull(a.Name)
			return mapStrErr("output", out, err)
		})
	case OpKernelSave:
		// Résout le dépôt réel du KERNEL pour que la politique PIN et l'aperçu d'envoi portent sur LUI
		// (gate() lit a.Instance/a.Path). Le cœur re-résout ensuite par nom (source de vérité).
		for _, k := range d.svc.Kernels() {
			if (a.Name == "" && k.Active) || strings.EqualFold(k.Name, a.Name) {
				a.Instance, a.Path = k.Instance, k.LocalPath
				break
			}
		}
		pin, err := d.gate(op, a, "Autoriser l'enregistrement de vos notes dans le KERNEL (récupérer, enregistrer, envoyer)")
		if err != nil {
			return nil, err
		}
		return d.svc.KernelSave(a.Name, a.Message, pin)

	// --- SPOKES (v1.11) : projections de mission ---
	case OpSpokeList:
		return d.svc.Spokes(), nil
	case OpSpokeStatus:
		return d.svc.SpokeStatus(a.Name)
	case OpSpokeContents:
		return d.svc.SpokeContents(a.Name)
	case OpSpokePull:
		return d.withSession(func() (any, error) {
			out, err := d.svc.SpokePull(a.Name)
			return mapStrErr("output", out, err)
		})
	case OpSpokeSave:
		for _, sp := range d.svc.Spokes() {
			if strings.EqualFold(sp.Name, a.Name) {
				a.Instance, a.Path = sp.Instance, sp.LocalPath
				break
			}
		}
		pin, err := d.gate(op, a, "Autoriser l'enregistrement du spoke « "+a.Name+" » (récupérer, enregistrer, envoyer)")
		if err != nil {
			return nil, err
		}
		return d.svc.SpokeSave(a.Name, a.Message, pin)

	default:
		return nil, errors.New("opération inconnue : " + op)
	}
}

// withSession garantit la session puis exécute fn.
func (d *Daemon) withSession(fn func() (any, error)) (any, error) {
	if err := d.ensureUnlocked(); err != nil {
		return nil, err
	}
	return fn()
}

// helpers de mise en forme des retours (string, error) -> map JSON.
func mapStrErr(key, v string, err error) (any, error) {
	if err != nil {
		return nil, err
	}
	return map[string]string{key: v}, nil
}
