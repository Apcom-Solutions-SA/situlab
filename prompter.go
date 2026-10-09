// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package main

import (
	"context"
	"sync"
	"time"

	wailsrt "github.com/wailsapp/wails/v2/pkg/runtime"
)

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
// un aperçu optionnel de l'action (ex. *core.PushPreview), transmis au frontend pour un consentement
// éclairé ; il est sérialisé dans la charge utile de l'évènement.
func (p *guiPrompter) PromptPIN(reason string, detail any) (string, bool) {
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
	wailsrt.EventsEmit(p.ctx, "situlab:pin-request", map[string]any{"reason": reason, "detail": detail})

	select {
	case r := <-ch:
		return r.pin, r.ok
	case <-time.After(2 * time.Minute):
		return "", false // garde-fou : pas de blocage éternel
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
