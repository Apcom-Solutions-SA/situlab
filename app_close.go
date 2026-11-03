// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package main

import (
	"context"
	"situlab/internal/core"
	"time"

	wailsrt "github.com/wailsapp/wails/v2/pkg/runtime"
)

// backupScheduler : sauvegarde AUTOMATIQUE de fond (P15, v1.18.2). Tant que la fenêtre est ouverte et
// la session déverrouillée (nécessaire pour déchiffrer les jetons cloud), relance les plans marqués
// Auto selon leur cadence, en incrémental (ne sauve que les dépôts changés). La trace va au Journal.
// S'arrête à la fermeture. Non bloquant, non perturbant.
func (a *App) backupScheduler() {
	t := time.NewTicker(60 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-t.C:
			if a.svc == nil || a.closing.Load() || !a.svc.Unlocked() || a.svc.ReadOnly() {
				continue
			}
			for _, p := range a.svc.Backups() {
				if !p.Auto {
					continue
				}
				every := p.EveryMin
				if every <= 0 {
					every = 30
				}
				if backupDue(p.LastRun, every) {
					name := p.Name
					// Journal (1.20) : la sauvegarde AUTO ne passe pas par le démon ; on la trace ici pour
					// qu'un échec silencieux (« pourquoi ma sauvegarde ne s'est pas lancée ? ») soit visible.
					go func() {
						_, err := a.svc.RunBackupPlan(name, "auto")
						a.svc.LogOp("auto", "backup.run", name, err == nil, err, 0, "")
					}()
				}
			}
		}
	}
}

// backupDue : le dernier run remonte-t-il à au moins everyMin minutes ? (jamais exécuté → dû).
func backupDue(lastRun string, everyMin int) bool {
	if lastRun == "" {
		return true
	}
	t, err := time.Parse(time.RFC3339, lastRun)
	if err != nil {
		return true
	}
	return time.Since(t) >= time.Duration(everyMin)*time.Minute
}

// BackupBusy : une sauvegarde est-elle en cours ? (pour l'indicateur, au montage de l'interface.)
func (a *App) BackupBusy() bool {
	if a.svc == nil {
		return false
	}
	return a.svc.BackupBusy()
}

// beforeClose (OnBeforeClose) : si des sauvegardes « à la fermeture » sont prévues, on NE ferme PAS
// tout de suite et on NE sauvegarde PLUS d'office (1.18.1). On empêche la fermeture et on laisse le
// frontend DEMANDER à l'utilisateur (modale « sauvegarder / fermer sans ») via « backup:close-prompt »,
// avec l'aperçu de ce qui serait sauvegardé. La suite est pilotée par RunCloseBackups / CloseWithoutBackup.
// Retourner true = empêcher la fermeture immédiate.
func (a *App) beforeClose(ctx context.Context) bool {
	if a.svc == nil || a.closing.Load() {
		return false // rien à faire, ou décision déjà prise (sauvegarde ou fermeture) → laisser fermer
	}
	hasBk := a.svc.HasDueBackups("on_close")
	agents := a.svc.AgentsForClose()
	if !hasBk && len(agents) == 0 {
		return false // ni sauvegarde due, ni purge proposée → fermer tout de suite
	}
	// Modale INTELLIGENTE de fermeture (1.18.5) : un seul prompt réunit sauvegardes et purges d'agent.
	wailsrt.EventsEmit(a.ctx, "close:prompt", map[string]any{
		"backups": a.svc.DueBackupPreview("on_close"),
		"agents":  agents,
	})
	return true
}

// ClosePlan : ce que la fermeture peut proposer (sauvegardes dues + agents à purger), pour la modale.
type ClosePlan struct {
	Backups []core.DueBackupItem `json:"backups"`
	Agents  []core.AgentInfo     `json:"agents"`
}

// ClosePlan renvoie l'aperçu des actions proposables à la fermeture (lecture seule).
func (a *App) ClosePlan() ClosePlan {
	if err := a.ready(); err != nil {
		return ClosePlan{}
	}
	return ClosePlan{Backups: a.svc.DueBackupPreview("on_close"), Agents: a.svc.AgentsForClose()}
}

// RunCloseActions : l'utilisateur a choisi, dans la modale de fermeture, de SAUVEGARDER (doBackup) et/ou
// de PURGER des agents (purgeKeys), puis de fermer. On exécute dans l'ordre (sauvegardes → purges), avec
// le suivi visuel (events "backup:progress" et "agentpurge:progress"), on laisse voir les quittances un
// court instant, puis on quitte. closing passe à true pour verrouiller la décision (anti double-lancement).
func (a *App) RunCloseActions(doBackup bool, purgeKeys []string) {
	if a.svc == nil {
		return
	}
	if a.closing.Swap(true) {
		return // décision déjà prise
	}
	go func() {
		if doBackup && a.svc.HasDueBackups("on_close") {
			a.svc.RunDueBackups("on_close")
		}
		for _, k := range purgeKeys {
			_, _ = a.svc.PurgeAgent(k, core.AgentPurgeConfirm)
		}
		// Actions terminées : on NE ferme PLUS tout seul (1.19, #4). On signale la fin ; le frontend
		// affiche le bilan (statut par plan sauvegardé, par agent et par dossier purgé) et attend un
		// « OK — fermer » explicite (QuitNow). Fini le « tout se ferme trop vite ».
		if a.ctx != nil {
			wailsrt.EventsEmit(a.ctx, "close:done", nil)
		}
	}()
}

// QuitNow ferme la fenêtre immédiatement, après que l'utilisateur a validé le bilan de fermeture
// (bouton « OK — fermer »). closing est déjà posé (décision prise), donc beforeClose laisse passer.
func (a *App) QuitNow() {
	a.closing.Store(true)
	if a.ctx != nil {
		wailsrt.Quit(a.ctx)
	}
}

// AgentClosePurgeKeys : agents configurés pour la purge proposée à la fermeture (réglage). Lecture seule.
func (a *App) AgentClosePurgeKeys() []string {
	if a.svc == nil {
		return nil
	}
	return a.svc.AgentClosePurgeKeys()
}

// SetAgentClosePurge active/désactive la proposition de purge à la fermeture pour un agent (réglage local).
func (a *App) SetAgentClosePurge(key string, on bool) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.SetAgentClosePurge(key, on)
}

// DueBackupPreview : ce qui serait sauvegardé à la fermeture (pour la modale de choix et de suivi).
func (a *App) DueBackupPreview() []core.DueBackupItem {
	if err := a.ready(); err != nil {
		return nil
	}
	return a.svc.DueBackupPreview("on_close")
}

// CloseWithoutBackup : l'utilisateur choisit de fermer SANS sauvegarder. On marque la décision prise
// (closing) pour que beforeClose laisse passer, puis on quitte.
func (a *App) CloseWithoutBackup() {
	a.closing.Store(true)
	if a.ctx != nil {
		wailsrt.Quit(a.ctx)
	}
}

// RunCloseBackups : l'utilisateur choisit de sauvegarder avant de fermer. On exécute les plans dus
// (suivi visuel via « backup:progress »), on laisse l'écran afficher l'état final un court instant,
// puis on quitte. closing passe à true (via Swap) pour verrouiller la décision et éviter un double lancement.
func (a *App) RunCloseBackups() {
	if a.svc == nil {
		return
	}
	if a.closing.Swap(true) {
		return // décision déjà prise
	}
	go func() {
		a.svc.RunDueBackups("on_close")
		time.Sleep(1200 * time.Millisecond) // laisse voir les quittances ✓ avant la fermeture
		if a.ctx != nil {
			wailsrt.Quit(a.ctx)
		}
	}()
}
