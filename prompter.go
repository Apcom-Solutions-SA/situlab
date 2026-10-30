// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package main

import (
	"context"
	"sync"
	"time"

	"situlab/internal/core"
	"situlab/internal/daemon"

	wailsrt "github.com/wailsapp/wails/v2/pkg/runtime"
)

// pinPromptTTL borne l'attente d'une saisie de PIN : au-delà, la demande expire (PINTimedOut) pour ne
// jamais bloquer éternellement un appel d'agent. Source UNIQUE : core.PINPromptTTL (même durée que le
// compte à rebours affiché et le message d'expiration — besoin #6).
const pinPromptTTL = core.PINPromptTTL

// guiPrompter réalise la demande de PIN « à la git » : quand le démon a besoin du PIN
// (déverrouiller pour lire, confirmer pour écrire), il fait surgir une fenêtre de saisie
// sur le bureau. Le PIN est saisi par l'humain DANS l'app et ne transite jamais par le
// socket : le client (souvent un agent) n'obtient que « fait » ou « refusé ».
//
// Déroulé : PromptPIN lève la fenêtre et émet l'événement "situlab:pin-request" ; le
// frontend affiche la modale et renvoie la saisie via les méthodes liées SubmitPIN /
// CancelPIN (App), qui la déposent sur le canal attendu par PromptPIN.
type guiPrompter struct {
	ctx  context.Context
	gate sync.Mutex // un seul PIN demandé à la fois
	rmu  sync.Mutex // protège resp (accédé depuis une autre goroutine)
	resp chan pinResp
}

type pinResp struct {
	pin string
	ok  bool
}

func newGUIPrompter(ctx context.Context) *guiPrompter { return &guiPrompter{ctx: ctx} }

// PromptPIN bloque jusqu'à la saisie (ou l'annulation, ou un délai de garde de 2 min). detail est
// un aperçu optionnel de l'action (ex. *core.PushPreview) et action une clé de classe (push|delete|…) ;
// tous deux transmis au frontend pour un consentement éclairé et un visuel adapté, sérialisés dans
// la charge utile de l'évènement. Ni l'un ni l'autre n'autorise quoi que ce soit : pur affichage.
func (p *guiPrompter) PromptPIN(reason string, detail any, action, intent string) (string, daemon.PINOutcome) {
	p.gate.Lock()
	defer p.gate.Unlock()

	ch := make(chan pinResp, 1)
	p.rmu.Lock()
	p.resp = ch
	p.rmu.Unlock()
	defer func() {
		p.rmu.Lock()
		p.resp = nil
		p.rmu.Unlock()
	}()

	// Consentement humain : la demande de PIN doit S'IMPOSER à l'écran, au-dessus de tout
	// ce que fait l'utilisateur (une action hors-agent est nécessaire). On force la fenêtre
	// au premier plan, toujours-au-dessus et recentrée, puis on restaure l'empilement normal
	// une fois la réponse obtenue (pour ne pas laisser l'app collée au-dessus des autres).
	wailsrt.WindowUnminimise(p.ctx)
	wailsrt.WindowShow(p.ctx)
	wailsrt.WindowCenter(p.ctx)
	wailsrt.WindowSetAlwaysOnTop(p.ctx, true)
	defer wailsrt.WindowSetAlwaysOnTop(p.ctx, false)

	// Ré-émission jusqu'à réponse : au DÉMARRAGE À FROID, c'est l'appel d'agent lui-même qui réveille
	// l'app ; le frontend ne lie son écouteur "situlab:pin-request" qu'APRÈS avoir booté, donc une
	// émission unique serait perdue — et le frontend retomberait sur son écran de déverrouillage
	// STANDARD, sans le contexte de l'action. En répétant l'évènement, on garantit que la fenêtre de
	// requête d'agent (contextuelle) finit par s'afficher et PRENNE LA MAIN sur l'écran standard.
	// Idempotent côté frontend : une ré-émission est ignorée tant que la fenêtre est déjà ouverte.
	// Échéance calculée AVANT la charge utile pour la transmettre au frontend : il affiche un compte à
	// rebours visible et bascule proprement en « délai écoulé » au lieu de rester figé (besoin #6). La
	// même échéance borne le garde-fou ci-dessous : frontend et démon expirent ensemble.
	deadline := time.Now().Add(pinPromptTTL)
	payload := map[string]any{"reason": reason, "detail": detail, "action": action, "intent": intent,
		"deadlineMs": deadline.UnixMilli(), "ttlSec": core.PINPromptTTLSeconds()}
	wailsrt.EventsEmit(p.ctx, "situlab:pin-request", payload)
	// Attirer l'attention quand la demande vient d'un agent (CLI/MCP) et que la fenêtre n'est pas
	// forcément au premier plan : clignotement de l'icône dans la barre des tâches (retour #15).
	// Best-effort et auto-neutralisé si la fenêtre est déjà au premier plan ; no-op hors Windows.
	// Le « cling » sonore est joué côté frontend à la réception de l'évènement.
	flashTaskbarAttention()
	reping := time.NewTicker(500 * time.Millisecond)
	defer reping.Stop()

	for {
		select {
		case r := <-ch:
			if r.ok {
				return r.pin, daemon.PINConfirmed
			}
			return "", daemon.PINRefused // l'humain a annulé la fenêtre
		case <-reping.C:
			if !time.Now().Before(deadline) {
				return "", daemon.PINTimedOut // garde-fou : pas de blocage éternel ; l'agent saura que c'est un délai
			}
			wailsrt.WindowShow(p.ctx)
			wailsrt.EventsEmit(p.ctx, "situlab:pin-request", payload)
		}
	}
}

// deliver est appelée par App.SubmitPIN / App.CancelPIN (goroutine du pont Wails).
func (p *guiPrompter) deliver(pin string, ok bool) {
	p.rmu.Lock()
	ch := p.resp
	p.rmu.Unlock()
	if ch != nil {
		select {
		case ch <- pinResp{pin: pin, ok: ok}:
		default: // aucune demande en attente : on ignore
		}
	}
}
