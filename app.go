// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"situlab/internal/config"
	"situlab/internal/core"
	"situlab/internal/daemon"
	"situlab/internal/forgejo"
	"situlab/internal/gitx"
	"situlab/internal/ipc"

	wailsrt "github.com/wailsapp/wails/v2/pkg/runtime"
)

// FocusWindow ramène la fenêtre au premier plan et lui donne le focus clavier — nécessaire
// pour que le champ PIN soit actif dès l'ouverture (WebView2 ne focalise pas un input tant
// que la fenêtre native n'a pas le focus).
func (a *App) FocusWindow() {
	if a.ctx != nil {
		wailsrt.WindowUnminimise(a.ctx)
		wailsrt.WindowShow(a.ctx)
	}
}

// domReady est appelé par Wails quand le DOM est prêt : on (ré)active la fenêtre pour que la
// saisie du PIN soit immédiate, sans clic préalable (hook standard Wails).
func (a *App) domReady(ctx context.Context) {
	wailsrt.WindowShow(ctx)
}

// PickFolder ouvre le sélecteur de dossier natif et renvoie le chemin choisi (vide si
// l'utilisateur annule). Sert à « lier un dépôt local » sans taper de chemin à la main.
func (a *App) PickFolder() (string, error) {
	if a.ctx == nil {
		return "", nil
	}
	return wailsrt.OpenDirectoryDialog(a.ctx, wailsrt.OpenDialogOptions{
		Title: "Choisir le dossier d'un dépôt git",
	})
}

// App expose le noyau à la fenêtre Wails. Chaque méthode appelle le même noyau que
// la CLI : la parité GUI <-> CLI est structurelle, pas dupliquée.
type App struct {
	ctx       context.Context
	svc       *core.Service
	initErr   error // non nil si le noyau n'a pas pu s'initialiser
	prompter  *guiPrompter
	daemonSrv *ipc.Server
}

// NewApp construit l'app GUI (le noyau est chargé au démarrage).
func NewApp() *App { return &App{} }

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.svc, a.initErr = core.New(version)
	if a.svc == nil {
		return
	}
	// Cette fenêtre est aussi le DÉMON résident : elle sert les commandes CLI et détient
	// la session. La modale PIN surgit ici quand une opération l'exige.
	a.prompter = newGUIPrompter(ctx)
	a.startDaemon()
}

// startDaemon démarre le serveur IPC, sauf si un démon tourne déjà (autre fenêtre ouverte).
func (a *App) startDaemon() {
	path := ipc.SocketPath()
	if c, err := ipc.Dial(path); err == nil {
		c.Close()
		return // un démon répond déjà : on ne vole pas son socket
	}
	if srv, err := daemon.New(a.svc, a.prompter).Serve(path); err == nil {
		a.daemonSrv = srv
	}
}

// SubmitPIN / CancelPIN sont appelées par le frontend pour répondre à la modale PIN.
func (a *App) SubmitPIN(pin string) {
	if a.prompter != nil {
		a.prompter.deliver(pin, true)
	}
}
func (a *App) CancelPIN() {
	if a.prompter != nil {
		a.prompter.deliver("", false)
	}
}

// ready renvoie une erreur explicite si le noyau n'est pas prêt (au lieu de paniquer).
func (a *App) ready() error {
	if a.svc == nil {
		if a.initErr != nil {
			return a.initErr
		}
		return errors.New("SituLab n'a pas pu démarrer (configuration ou trousseau indisponible)")
	}
	return nil
}

// AppVersion renvoie la version.
func (a *App) AppVersion() string { return version }

// Doc renvoie la documentation embarquée (Markdown) — utile aux humains comme aux agents.
func (a *App) Doc() string { return agentsDoc }

// License renvoie le texte complet de la licence (Apache 2.0).
func (a *App) License() string { return licenseText }

// Notice renvoie l'avis de droits et marques (fichier NOTICE).
func (a *App) Notice() string { return noticeText }

// Doctor renvoie l'état de l'environnement.
func (a *App) Doctor() (core.Doctor, error) {
	if err := a.ready(); err != nil {
		return core.Doctor{}, err
	}
	return a.svc.Doctor(), nil
}

// GetInstances renvoie les instances configurées.
func (a *App) GetInstances() ([]config.Instance, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.Instances(), nil
}

// AddInstance ajoute une instance avec un token déjà en main (teste avant de persister).
func (a *App) AddInstance(name, url, token string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.AddInstance(name, url, token)
}

// AddInstanceToken ajoute une instance par JETON déjà en main. provider vaut "forgejo"
// (URL requise) ou "github" (l'utilisateur colle un Personal Access Token ; l'URL est
// fixée à github.com). Teste la connexion avant de persister.
func (a *App) AddInstanceToken(name, provider, url, token string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.AddInstanceToken(name, provider, url, token)
}

// AddInstanceLogin : onboarding « PIN-only ». Identifiant + mot de passe (+ code 2FA si le
// compte en a un) ; SituLab crée le token via l'API, le chiffre par le PIN, et ne stocke ni
// le mot de passe ni l'OTP. Si otp est vide et que le serveur exige la 2FA, l'appel échoue
// (le frontend révèle alors le champ code 2FA et réessaie).
func (a *App) AddInstanceLogin(name, url, user, password, otp string, adminScope, remember bool) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.AddInstanceLogin(name, url, user, password, otp, adminScope, remember)
}

// RemoveInstance supprime une instance.
func (a *App) RemoveInstance(name string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.RemoveInstance(name)
}

// SetDefaultInstance définit l'instance par défaut.
func (a *App) SetDefaultInstance(name string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.SetDefaultInstance(name)
}

// TestInstance teste une connexion.
func (a *App) TestInstance(name string) (*forgejo.User, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.TestInstance(name)
}

// Whoami renvoie l'utilisateur courant.
func (a *App) Whoami(instance string) (*forgejo.User, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.Whoami(instance)
}

// Dashboard renvoie la vue unifiée distant+local (objet central de SituLab).
func (a *App) Dashboard(instance string) (*core.DashboardData, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.Dashboard(instance)
}

// Repos liste les dépôts accessibles d'une connexion (utilisé par le sélecteur de la modale projet).
func (a *App) Repos(instance string) ([]forgejo.Repo, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.Repos(instance)
}

// RepoBranches / RepoTags / RepoCommits / RepoContents / RepoFile : détail distant.
func (a *App) RepoBranches(instance, owner, repo string) ([]forgejo.Branch, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.RepoBranches(instance, owner, repo)
}

func (a *App) RepoTags(instance, owner, repo string) ([]forgejo.Tag, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.RepoTags(instance, owner, repo)
}

func (a *App) RepoCommits(instance, owner, repo, ref string, limit int) ([]forgejo.Commit, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.RepoCommits(instance, owner, repo, ref, limit)
}

func (a *App) RepoContents(instance, owner, repo, path, ref string) ([]forgejo.Entry, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.RepoContents(instance, owner, repo, path, ref)
}

func (a *App) RepoFile(instance, owner, repo, path, ref string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.svc.RepoFile(instance, owner, repo, path, ref)
}

// ListTokens renvoie les tokens d'accès (noms et portées ; jamais les valeurs).
func (a *App) ListTokens(instance string) ([]forgejo.Token, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.Tokens(instance)
}

// LocalDetail renvoie l'état détaillé d'un dépôt local.
func (a *App) LocalDetail(path string) (*gitx.Detail, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.LocalDetail(path)
}

// BranchMatrix renvoie l'état par branche d'un dépôt local (ce poste vs en ligne).
func (a *App) BranchMatrix(path string) (*gitx.BranchMatrix, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.BranchMatrix(path)
}

// AddLocalRepo enregistre un dépôt local.
func (a *App) AddLocalRepo(path string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.AddLocalRepo(path)
}

// PinSet indique si un PIN est configuré.
func (a *App) PinSet() (bool, error) {
	if err := a.ready(); err != nil {
		return false, err
	}
	return a.svc.PINSet(), nil
}

// SetPin définit ou change le PIN (ancien requis s'il en existe un).
func (a *App) SetPin(oldCode, newCode string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.SetPIN(oldCode, newCode)
}

// Unlocked indique si la session de lecture est ouverte.
func (a *App) Unlocked() (bool, error) {
	if err := a.ready(); err != nil {
		return false, err
	}
	return a.svc.Unlocked(), nil
}

// Unlock ouvre la session de lecture avec le PIN (saisi dans la fenêtre). Indispensable au
// retour dans l'app : sans session, toutes les lectures échouent (ErrLocked).
func (a *App) Unlock(pin string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.Unlock(pin)
}

// Lock referme la session (oublie la clé en mémoire).
func (a *App) Lock() error {
	if err := a.ready(); err != nil {
		return err
	}
	a.svc.Lock()
	return nil
}

// ResetKeyring efface le trousseau (PIN oublié, irrécupérable) et ramène à l'onboarding.
func (a *App) ResetKeyring() error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.ResetKeyring()
}

// ActiveTokenName : nom exact du token utilisé par SituLab pour l'instance (à protéger).
func (a *App) ActiveTokenName(instance string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.svc.ActiveTokenName(instance), nil
}

// DisconnectedInstances : instances sans token valide (à reconnecter).
func (a *App) DisconnectedInstances() ([]string, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.DisconnectedInstances(), nil
}

// LockDelay / SetLockDelay : délai d'auto-verrouillage en minutes (-1 = jamais, 0 = défaut 15).
func (a *App) LockDelay() (int, error) {
	if err := a.ready(); err != nil {
		return 0, err
	}
	return a.svc.LockDelay(), nil
}
func (a *App) SetLockDelay(min int) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.SetLockDelay(min)
}
func (a *App) LockAfterWrite() (bool, error) {
	if err := a.ready(); err != nil {
		return false, err
	}
	return a.svc.LockAfterWrite(), nil
}
func (a *App) SetLockAfterWrite(on bool) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.SetLockAfterWrite(on)
}

// ReadOnly / SetReadOnly : mode lecture seule (v1.4) — bloque toute mutation.
func (a *App) ReadOnly() (bool, error) {
	if err := a.ready(); err != nil {
		return false, err
	}
	return a.svc.ReadOnly(), nil
}
func (a *App) SetReadOnly(on bool) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.SetReadOnly(on)
}

// SetInstanceRole fixe le rôle d'une connexion (atelier|production). Fenêtre uniquement (v1.7).
func (a *App) SetInstanceRole(name, role string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.SetInstanceRole(name, role)
}

// SetInstancePINPolicy remplace la politique PIN d'une connexion (policy nil = défauts du rôle).
func (a *App) SetInstancePINPolicy(name string, policy *config.PINPolicy) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.SetInstancePINPolicy(name, policy)
}

// --- Projets atelier↔production (v1.7.1) ---

func (a *App) Projects() ([]config.Project, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.Projects(), nil
}

func (a *App) ProjectStatus(name string) (*core.ProjectView, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.ProjectStatus(name)
}

// ProjectBoards : état « 3 colonnes » (local · atelier · production) de tous les projets (v1.8).
func (a *App) ProjectBoards() ([]core.ProjectBoard, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.ProjectBoards()
}

// AddProject crée un projet (atelier obligatoire, vitrines facultatives). icon = emoji/picto choisi.
func (a *App) AddProject(name, icon string, atelier config.RepoRef, productions []config.RepoRef) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.SaveProject("", name, icon, atelier, productions)
}

// UpdateProject modifie un projet existant (retrouvé par oldName) : renommage, icône, atelier et
// vitrines. Permet notamment de passer d'un projet « local + atelier » à « local + atelier + vitrine ».
func (a *App) UpdateProject(oldName, name, icon string, atelier config.RepoRef, productions []config.RepoRef) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.SaveProject(oldName, name, icon, atelier, productions)
}

func (a *App) RemoveProject(name string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.RemoveProject(name)
}

// ProjectPromote publie la version de l'atelier vers une production du projet (PIN saisi dans la
// fenêtre). Geste de production : jamais déclenché par une IA sans confirmation humaine.
func (a *App) ProjectPromote(name string, prodIndex int, sourceBranch, version, targetBranch, pin string) (*core.PromoteResult, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.ProjectPromote(name, prodIndex, core.PromoteOptions{SourceBranch: sourceBranch, Version: version, TargetBranch: targetBranch}, pin)
}

// BranchTags liste les versions (tags) joignables depuis une branche d'un clone local, de la plus
// récente à la plus ancienne — pour que l'utilisateur choisisse QUELLE version promouvoir.
func (a *App) BranchTags(path, branch string) ([]string, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.BranchTags(path, branch)
}

// Purge efface toutes les données SituLab de ce poste (v1.4.1) : connexions, trousseau chiffré,
// réglages, jetons du coffre OS. IRRÉVERSIBLE — la fenêtre demande une confirmation explicite
// avant d'appeler cette méthode, puis se réinitialise (onboarding).
//
// On FERME d'abord le serveur IPC : son socket AF_UNIX vit DANS config.Dir(), or sous Windows un
// socket encore lié n'est pas supprimable → sans ça, os.RemoveAll échouerait sur daemon.sock APRÈS
// avoir déjà effacé config.json/keyring.json, et remonterait une fausse erreur. (Le socket sera
// recréé au prochain lancement ; la fenêtre, elle, continue via les liaisons in-process.)
func (a *App) Purge() error {
	if err := a.ready(); err != nil {
		return err
	}
	if a.daemonSrv != nil {
		a.daemonSrv.Close()
		a.daemonSrv = nil
	}
	return a.svc.Purge()
}

// afterWrite reverrouille la session si l'option « verrouiller après chaque écriture » est active.
// Le chemin démon (CLI/MCP) le fait dans handle() ; les écritures lancées depuis la fenêtre passent
// en in-process et ne traversent pas handle(), d'où ce rappel explicite pour la parité.
func (a *App) afterWrite() {
	if a.svc.LockAfterWrite() {
		a.svc.Lock()
	}
}

// Push pousse un dépôt local (PIN obligatoire).
func (a *App) Push(instance, path, pin string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	out, err := a.svc.Push(instance, path, pin)
	if err == nil {
		a.afterWrite()
	}
	return out, err
}

// Synchronize est le geste unique « Synchroniser » : récupérer → enregistrer → envoyer, sous une
// seule confirmation PIN (v1.3).
func (a *App) Synchronize(instance, path, message, pin string) (*core.SyncResult, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	res, err := a.svc.Synchronize(instance, path, message, pin)
	if err == nil {
		a.afterWrite()
	}
	return res, err
}

// Fetch / Pull / Commit / Diff : opérations de la boucle de travail locale.
func (a *App) Fetch(instance, path string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.svc.Fetch(instance, path)
}
func (a *App) Pull(instance, path string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.svc.Pull(instance, path)
}

// FetchAllLocal récupère en arrière-plan les nouveautés distantes de tous les dépôts suivis
// (best-effort, sans PIN). La GUI l'appelle périodiquement pour garder l'état à jour.
func (a *App) FetchAllLocal() (int, error) {
	if err := a.ready(); err != nil {
		return 0, err
	}
	return a.svc.FetchAllLocal()
}
func (a *App) Commit(path, message string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.svc.Commit(path, message)
}
func (a *App) Diff(path string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.svc.Diff(path)
}
func (a *App) RemoveLocalRepo(path string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.RemoveLocalRepo(path)
}

// Clone copie un dépôt distant sur le poste (action « Copier sur mon poste »).
func (a *App) Clone(instance, owner, repo, dir string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.svc.Clone(instance, owner, repo, dir)
}

// git local (modale « Git local ») : créer/changer de branche, mettre de côté, annuler. Pas de
// PIN (local), mais refusés en lecture seule par le cœur (assertWritable).
func (a *App) GitCreateBranch(path, name string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.svc.GitCreateBranch(path, name)
}
func (a *App) GitSwitchBranch(path, name string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.svc.GitSwitchBranch(path, name)
}
func (a *App) GitStashSave(path, message string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.svc.GitStashSave(path, message)
}
func (a *App) GitStashPop(path string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.svc.GitStashPop(path)
}
func (a *App) GitDiscard(path, file string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.svc.GitDiscard(path, file)
}

// --- Écritures et lectures API étendues (v1) ---
func (a *App) CreateRepo(instance, owner, name, desc string, private bool, pin string) (*forgejo.Repo, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.CreateRepo(instance, owner, name, desc, private, pin)
}

// DeleteRepo supprime définitivement un dépôt distant (PIN requis).
func (a *App) DeleteRepo(instance, owner, repo, pin string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.DeleteRepo(instance, owner, repo, pin)
}
func (a *App) CreateIssue(instance, owner, repo, title, body, pin string) (*forgejo.Issue, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.CreateIssue(instance, owner, repo, title, body, pin)
}
func (a *App) CommentIssue(instance, owner, repo string, index int, body, pin string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.CommentIssue(instance, owner, repo, index, body, pin)
}
func (a *App) SetIssueState(instance, owner, repo string, index int, state, pin string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.SetIssueState(instance, owner, repo, index, state, pin)
}
func (a *App) EditIssue(instance, owner, repo string, index int, title, body, pin string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.EditIssue(instance, owner, repo, index, title, body, pin)
}
func (a *App) CreatePull(instance, owner, repo, title, head, base, body, pin string) (*forgejo.Issue, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.CreatePull(instance, owner, repo, title, head, base, body, pin)
}
func (a *App) MergePull(instance, owner, repo string, index int, method, pin string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.MergePull(instance, owner, repo, index, method, pin)
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

// AdminScopeFor indique si le jeton de l'instance porte déjà le droit d'administration (v1.4.2) :
// si non (et que le compte est admin), la fenêtre propose « Activer l'administration ».
func (a *App) AdminScopeFor(instance string) (bool, error) {
	if err := a.ready(); err != nil {
		return false, err
	}
	return a.svc.AdminScopeFor(instance), nil
}

// ExePath renvoie le chemin de l'exécutable SituLab (pour composer la config MCP de l'agent).
func (a *App) ExePath() (string, error) { return os.Executable() }

// InstallAgentsDoc écrit le guide des agents (AGENTS.md embarqué) dans le dossier choisi, pour
// que l'assistant IA de l'utilisateur le lise (convention AGENTS.md). Ne RÉÉCRASE jamais un
// AGENTS.md existant : dans ce cas il écrit « SituLab-AGENTS.md » à côté. Renvoie le chemin écrit.
func (a *App) InstallAgentsDoc(dir string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	if dir == "" {
		return "", errors.New("aucun dossier choisi")
	}
	p := filepath.Join(dir, "AGENTS.md")
	if _, err := os.Stat(p); err == nil {
		p = filepath.Join(dir, "SituLab-AGENTS.md") // ne pas écraser le AGENTS.md de l'utilisateur
	}
	// Le guide embarqué + la fiche de collaboration (consignes du user) : l'IA lit l'ensemble au
	// démarrage et peut enrichir la section « Notre collaboration » directement dans ce fichier.
	content := agentsDoc + "\n\n## Notre collaboration (maintenue avec mon IA)\n\n" +
		"_Cette section est la nôtre : notes de travail entre moi et mon assistant. Mon IA peut la compléter ; je peux l'éditer dans SituLab (Réglages → Assistant IA)._\n\n" +
		strings.TrimSpace(a.svc.AgentNotes()) + "\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		return "", err
	}
	return p, nil
}

// AgentNotes / SetAgentNotes : fiche de collaboration avec l'IA, éditable dans les Réglages (v1.5.1).
func (a *App) AgentNotes() (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.svc.AgentNotes(), nil
}
func (a *App) SetAgentNotes(notes string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.SetAgentNotes(notes)
}
func (a *App) AdminCreateUser(instance, username, email, password string, admin bool, pin string) (*forgejo.AdminUser, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.AdminCreateUser(instance, username, email, password, admin, pin)
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
	return a.svc.AdminSuspendUser(instance, username, suspend, pin)
}
func (a *App) AdminDeleteUser(instance, username string, purge bool, pin string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.AdminDeleteUser(instance, username, purge, pin)
}

// --- Gestionnaire de tokens ---
// ReconnectInstance recrée le token d'une instance (portée admin en option) — sans supprimer/
// ré-ajouter. Le mot de passe n'est mémorisé que si remember est vrai.
func (a *App) ReconnectInstance(name, user, password, otp string, adminScope, remember bool) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.ReconnectInstance(name, user, password, otp, adminScope, remember)
}

// ReconnectToken remet un jeton pour une instance existante (GitHub : nouveau PAT collé).
func (a *App) ReconnectToken(name, token string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.ReconnectToken(name, token)
}

// RememberPassword mémorise le mot de passe (chiffré par le PIN) après un 1er déverrouillage
// réussi du gestionnaire de jetons — pour ne plus le redemander ensuite.
func (a *App) RememberPassword(instance, password string, twoFA bool) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.RememberPassword(instance, password, twoFA)
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
	return a.svc.TokenCreate(instance, user, password, otp, name, scopes, repos, pin)
}
func (a *App) TokenDelete(instance, user, password, otp string, id int64, pin string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.TokenDelete(instance, user, password, otp, id, pin)
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
	return a.svc.ReviewPull(instance, owner, repo, index, event, body, pin)
}
func (a *App) SetPullState(instance, owner, repo string, index int, state, pin string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.SetPullState(instance, owner, repo, index, state, pin)
}
func (a *App) EditPull(instance, owner, repo string, index int, title, body, pin string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.EditPull(instance, owner, repo, index, title, body, pin)
}
func (a *App) CreateRelease(instance, owner, repo, tag, name, body, target string, draft, prerelease bool, pin string) (*forgejo.Release, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.CreateRelease(instance, owner, repo, tag, name, body, target, draft, prerelease, pin)
}
func (a *App) DeleteRelease(instance, owner, repo string, id int64, pin string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.DeleteRelease(instance, owner, repo, id, pin)
}
func (a *App) EditRepoSettings(instance, owner, repo string, fields map[string]any, pin string) (*forgejo.Repo, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.EditRepoSettings(instance, owner, repo, fields, pin)
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
	return a.svc.SetIssueLabels(instance, owner, repo, index, names, pin)
}
func (a *App) SetIssueAssignees(instance, owner, repo string, index int, assignees []string, pin string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.SetIssueAssignees(instance, owner, repo, index, assignees, pin)
}
func (a *App) SetIssueMilestone(instance, owner, repo string, index int, milestoneID int64, pin string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.SetIssueMilestone(instance, owner, repo, index, milestoneID, pin)
}
