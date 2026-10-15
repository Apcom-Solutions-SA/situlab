// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

// Terminal INTERACTIF (vrai TTY) de l'« OS de travail séparé » (v1.15.1). Contrairement à l'exécution
// « un coup à la fois » (OSBoxExecSession), ce terminal ouvre un VRAI pseudo-terminal (ConPTY sous
// Windows) attaché à `wsl.exe -d SituLab-Box`. La box voit alors un TTY (isatty() = vrai), ce qui
// permet de lancer des programmes plein écran interactifs — dont `claude` en mode agent.
//
// Flux : le PTY produit des octets (avec codes ANSI) → streamés vers l'UI via les événements Wails
// ("osterm:data:<id>", encodés base64 pour préserver l'UTF-8) ; les frappes clavier de xterm.js
// reviennent via OSBoxPtyWrite. Un PTY par onglet. La box reste SANS clé : toute action git/cloud
// passe toujours par SituLab (PIN sur l'hôte). ConPTY n'ouvre aucune fenêtre console visible.
package main

import (
	"context"
	"encoding/base64"
	"errors"
	"sync"

	"github.com/aymanbagabas/go-pty"
	wailsrt "github.com/wailsapp/wails/v2/pkg/runtime"

	"situlab/internal/osbox"
)

type ptySession struct {
	pty pty.Pty
	cmd *pty.Cmd
}

// ptyManager gère les sessions PTY actives (une par onglet de terminal), protégées par un mutex.
type ptyManager struct {
	mu   sync.Mutex
	sess map[string]*ptySession
}

func newPtyManager() *ptyManager { return &ptyManager{sess: map[string]*ptySession{}} }

// start ouvre un PTY pour l'onglet id et démarre `wsl.exe -d SituLab-Box` dedans. Idempotent : si la
// session existe déjà, on ne fait rien.
func (m *ptyManager) start(ctx context.Context, id string, cols, rows int) error {
	m.mu.Lock()
	if _, ok := m.sess[id]; ok {
		m.mu.Unlock()
		return nil
	}
	m.mu.Unlock()

	p, err := pty.New()
	if err != nil {
		return err
	}
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	_ = p.Resize(cols, rows)

	c := p.Command("wsl.exe", "-d", osbox.BoxDistro)
	if err := c.Start(); err != nil {
		_ = p.Close()
		return err
	}
	s := &ptySession{pty: p, cmd: c}
	m.mu.Lock()
	m.sess[id] = s
	m.mu.Unlock()

	// Streame la sortie du PTY vers l'UI (base64 pour un UTF-8 intact), puis signale la fin.
	go func() {
		buf := make([]byte, 8192)
		for {
			n, rerr := p.Read(buf)
			if n > 0 {
				wailsrt.EventsEmit(ctx, "osterm:data:"+id, base64.StdEncoding.EncodeToString(buf[:n]))
			}
			if rerr != nil {
				break
			}
		}
		_ = c.Wait()
		m.mu.Lock()
		delete(m.sess, id)
		m.mu.Unlock()
		wailsrt.EventsEmit(ctx, "osterm:exit:"+id, nil)
	}()
	return nil
}

func (m *ptyManager) write(id, data string) error {
	m.mu.Lock()
	s := m.sess[id]
	m.mu.Unlock()
	if s == nil {
		return errors.New("session de terminal inconnue")
	}
	_, err := s.pty.Write([]byte(data))
	return err
}

func (m *ptyManager) resize(id string, cols, rows int) error {
	m.mu.Lock()
	s := m.sess[id]
	m.mu.Unlock()
	if s == nil || cols <= 0 || rows <= 0 {
		return nil
	}
	return s.pty.Resize(cols, rows)
}

func (m *ptyManager) close(id string) {
	m.mu.Lock()
	s := m.sess[id]
	delete(m.sess, id)
	m.mu.Unlock()
	if s != nil {
		_ = s.pty.Close()
	}
}

func (m *ptyManager) closeAll() {
	m.mu.Lock()
	all := m.sess
	m.sess = map[string]*ptySession{}
	m.mu.Unlock()
	for _, s := range all {
		_ = s.pty.Close()
	}
}

// --- Bindings Wails ---------------------------------------------------------------------------

// OSBoxPtyStart ouvre un terminal interactif (vrai TTY) dans la box pour l'onglet id.
func (a *App) OSBoxPtyStart(id string, cols, rows int) error {
	if err := a.ready(); err != nil {
		return err
	}
	if !osbox.Installed() {
		return errors.New("l'OS de travail n'est pas installé")
	}
	if a.ptys == nil {
		a.ptys = newPtyManager()
	}
	return a.ptys.start(a.ctx, id, cols, rows)
}

// OSBoxPtyWrite envoie des frappes clavier (ou du texte collé) au terminal de l'onglet id.
func (a *App) OSBoxPtyWrite(id, data string) error {
	if a.ptys == nil {
		return errors.New("aucun terminal actif")
	}
	return a.ptys.write(id, data)
}

// OSBoxPtyResize ajuste la taille (colonnes x lignes) du terminal de l'onglet id.
func (a *App) OSBoxPtyResize(id string, cols, rows int) error {
	if a.ptys == nil {
		return nil
	}
	return a.ptys.resize(id, cols, rows)
}

// OSBoxPtyClose ferme le terminal de l'onglet id (fin de session).
func (a *App) OSBoxPtyClose(id string) error {
	if a.ptys != nil {
		a.ptys.close(id)
	}
	return nil
}
