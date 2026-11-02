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
	"time"

	"situlab/internal/config"
	"situlab/internal/core"
	"situlab/internal/ipc"
)

const (
	PINConfirmed   PINOutcome = iota // l'humain a saisi et validé le PIN dans la fenêtre
	PINRefused                       // l'humain a annulé la fenêtre
	PINTimedOut                      // aucune réponse dans le délai de garde (la fenêtre a pu rester ouverte)
	PINUnavailable                   // aucune interface disponible pour demander le PIN
)

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
	start := time.Now()
	data, err := d.dispatch(req.Op, a)
	// Journal local (1.20) : trace chaque op servie par le démon (surface cli/mcp), succès ou échec,
	// durée et « pourquoi » d'agent. Best-effort, jamais bloquant ; le niveau (off/errors/all) filtre.
	actor := req.Actor
	if actor == "" {
		actor = "cli"
	}
	d.svc.LogOp(actor, req.Op, targetOf(a), err == nil, err, time.Since(start).Milliseconds(), a.Intent)
	if err != nil {
		return ipc.Response{OK: false, Err: err.Error(), Kind: kindOf(err)}
	}
	// Rafraîchissement forcé de l'UI (optimisation 1.20) : un geste CLI/agent d'API DISTANTE qui aboutit
	// (création de dépôt/version/tag côté serveur, suppression de branche distante, réglages…) ne passe
	// par aucun verrou de dépôt local ; on signale donc ici. Les gestes LOCAUX (commit, merge, branche…)
	// et le push/fetch signalent déjà à la libération du verrou de dépôt dans le cœur. OpClone crée un
	// dépôt local suivi : on le signale aussi.
	if signalsGitChange(req.Op) {
		d.svc.SignalGitChange()
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

	// --- Journal local des opérations (1.20) ---
	case OpJournalRead:
		return d.svc.JournalRead(a.Limit, a.State == "errors")
	case OpJournalLevel:
		if a.Name != "" {
			if err := d.svc.SetJournalLevel(a.Name); err != nil {
				return nil, err
			}
		}
		return map[string]string{"level": d.svc.JournalLevel()}, nil
	case OpJournalClear:
		return nil, d.svc.JournalClear()

	default:
		return nil, errors.New("opération inconnue : " + op)
	}
}

// targetOf résume la cible d'une opération pour le journal : dépôt distant, chemin local, ou nom.
func targetOf(a Args) string {
	switch {
	case a.Owner != "" && a.Repo != "":
		return a.Owner + "/" + a.Repo
	case a.Path != "":
		return a.Path
	case a.Cloud != "":
		return a.Cloud
	case a.Name != "":
		return a.Name
	default:
		return ""
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
