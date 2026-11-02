// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package main

import (
	"context"
	"errors"
	"sync/atomic"

	"situlab/internal/core"
	"situlab/internal/daemon"
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
	go setWindowIcon() // Windows : pose l'icone de fenetre (barre de titre + taches), que Wails n'applique pas ; no-op ailleurs
	// Glisser-déposer de fichiers (CORPUS, v1.11) : Wails fournit les CHEMINS ABSOLUS des fichiers
	// déposés. On les relaie au frontend, qui décide (vue CORPUS active) de les envoyer au cloud, dans
	// le dossier courant, sous la politique PIN de la connexion.
	wailsrt.OnFileDrop(ctx, func(x, y int, paths []string) {
		if len(paths) > 0 {
			wailsrt.EventsEmit(ctx, "corpus:drop", paths)
		}
	})
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
	closing   atomic.Bool // vrai pendant la séquence « sauvegarde avant fermeture » (beforeClose)
}

// NewApp construit l'app GUI (le noyau est chargé au démarrage).
func NewApp() *App { return &App{} }

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.svc, a.initErr = core.New(version)
	if a.svc == nil {
		return
	}
	a.svc.ImportModulesSeed() // primo-paramètres déposés par l'installateur (1er lancement), best-effort
	// Suivi VISUEL des sauvegardes : on relaie l'avancement du cœur vers l'interface (événement Wails),
	// pour un indicateur permanent, quel que soit le déclencheur (manuel / ouverture / fermeture).
	a.svc.SetBackupProgress(func(ev core.BackupProgress) {
		if a.ctx != nil {
			wailsrt.EventsEmit(a.ctx, "backup:progress", ev)
		}
	})
	// Suivi VISUEL des purges d'agent IA (1.18.5) : on relaie l'avancement entrée par entrée vers l'UI,
	// pour l'écran de suivi (purge manuelle ET purge proposée à la fermeture).
	a.svc.SetAgentPurgeProgress(func(ev core.AgentPurgeProgress) {
		if a.ctx != nil {
			wailsrt.EventsEmit(a.ctx, "agentpurge:progress", ev)
		}
	})
	// Rafraîchissement FORCÉ de l'interface dès qu'une mutation git aboutit (optimisation 1.20) : plus
	// besoin d'attendre la récupération périodique ni de redémarrer. Le cœur signale à la libération du
	// verrou de dépôt (gestes locaux/distants) ; démon et fenêtre signalent en plus les gestes d'API
	// distante. On relaie vers l'événement Wails « git:changed » (quel que soit le déclencheur).
	a.svc.SetGitChanged(func() {
		if a.ctx != nil {
			wailsrt.EventsEmit(a.ctx, "git:changed", nil)
		}
	})
	// Cette fenêtre est aussi le DÉMON résident : elle sert les commandes CLI et détient
	// la session. La modale PIN surgit ici quand une opération l'exige.
	a.prompter = newGUIPrompter(ctx)
	a.startDaemon()
	go a.backupScheduler() // sauvegarde automatique de fond (v1.18.2), incrémentale, non perturbante
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

// VerifyPIN vérifie un PIN à blanc (throttlé), pour que la modale signale un PIN faux et reste ouverte
// pour une nouvelle saisie au lieu de se refermer en silence. N'ouvre PAS de session.
func (a *App) VerifyPIN(pin string) (bool, error) {
	if err := a.ready(); err != nil {
		return false, err
	}
	return a.svc.VerifyPIN(pin)
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

// shutdown : la fenêtre se ferme. Les sauvegardes « à la fermeture » sont exécutées EN AMONT par
// beforeClose (avec l'écran de suivi), pas ici — pour que l'utilisateur voie la procédure avant que la
// fenêtre disparaisse. Filet : si on arrive ici sans être passé par beforeClose (ex. arrêt système),
// on tente quand même les plans de fermeture, best-effort.
func (a *App) shutdown(ctx context.Context) {
	if a.svc != nil && !a.closing.Load() {
		a.svc.RunDueBackups("on_close")
	}
}

// afterWrite reverrouille la session si l'option « verrouiller après chaque écriture » est active.
// Le chemin démon (CLI/MCP) le fait dans handle() ; les écritures lancées depuis la fenêtre passent
// en in-process et ne traversent pas handle(), d'où ce rappel explicite pour la parité.
func (a *App) afterWrite() {
	// Force le rafraîchissement de l'UI après une mutation lancée depuis la FENÊTRE, y compris les
	// gestes d'API distante qui ne verrouillent aucun dépôt local (publication de version/tag, promotion,
	// réglages de dépôt…). Les gestes locaux signalent déjà via le verrou de dépôt (cœur).
	a.svc.SignalGitChange()
	if a.svc.LockAfterWrite() {
		a.svc.Lock()
	}
}

// lockedErr / lockedVal : parité du verrouillage-après-écriture pour les liaisons de la FENÊTRE, qui
// court-circuitent handle() (appel in-process du cœur). Toute mutation qui déchiffre un jeton ou touche
// un contenu distant doit, en cas de SUCCÈS, repasser par afterWrite() — exactement comme le démon le
// fait pour chaque isWriteOp. Sans ça, l'option « verrouiller après chaque écriture » ne protégeait que
// la moitié des surfaces (le jeton restait déchiffré en mémoire après un geste lancé depuis la fenêtre).
// lockedErr sert les liaisons qui ne renvoient qu'une erreur ; celles en « (valeur, erreur) » appellent
// afterWrite() en clair sur le chemin de succès (Go n'autorise pas d'étaler un résultat multi-valeurs
// comme argument quand un autre argument le précède).
func (a *App) lockedErr(err error) error {
	if err == nil {
		a.afterWrite()
	}
	return err
}
