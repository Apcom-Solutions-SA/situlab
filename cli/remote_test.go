// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package cli

import (
	"errors"
	"path/filepath"
	"testing"

	"situlab/internal/core"
	"situlab/internal/daemon"
	"situlab/internal/ipc"
)

type stubPrompter struct {
	pin string
	ok  bool
}

func (s stubPrompter) PromptPIN(string, any) (string, bool) { return s.pin, s.ok }

// TestRemoteRoundtrip valide la chaîne complète client CLI -> IPC -> démon -> noyau,
// y compris le mapping d'erreur typée (sans GUI ni auto-démarrage).
func TestRemoteRoundtrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AppData", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)

	svc0, err := core.New("test")
	if err != nil {
		t.Fatalf("core.New : %v", err)
	}
	d := daemon.New(svc0, stubPrompter{pin: "123456", ok: true})
	sock := filepath.Join(dir, "d.sock")
	srv, err := d.Serve(sock)
	if err != nil {
		t.Fatalf("Serve : %v", err)
	}
	defer srv.Close()

	cl, err := ipc.Dial(sock)
	if err != nil {
		t.Fatalf("Dial : %v", err)
	}
	r := &remote{cl: cl} // client déjà connecté (pas d'auto-démarrage en test)

	if set, _ := r.PINSet(); set {
		t.Fatal("aucun PIN ne devrait être posé au départ")
	}
	// Le PIN se pose EN IN-PROCESS (chemin de la fenêtre), jamais par le socket.
	if err := svc0.SetPIN("", "123456"); err != nil {
		t.Fatalf("SetPIN (noyau) : %v", err)
	}
	if set, err := r.PINSet(); err != nil || !set {
		t.Fatalf("le PIN devrait être posé (set=%v, err=%v)", set, err)
	}
	// Invariant : l'op « pin.set » est refusée par le démon (le PIN ne se définit jamais par le
	// socket ; Args ne porte d'ailleurs plus aucun champ PIN).
	if err := r.call(daemon.OpPINSet, daemon.Args{}, nil); err == nil {
		t.Fatal("l'op pin.set doit être refusée par le démon")
	}
	if ins, err := r.Instances(); err != nil || len(ins) != 0 {
		t.Fatalf("instances attendues vides, obtenu %d (err=%v)", len(ins), err)
	}

	// Erreur typée : supprimer une instance absente -> ErrNoInstance (via Kind).
	err = r.RemoveInstance("inexistante")
	if !errors.Is(err, core.ErrNoInstance) {
		t.Fatalf("RemoveInstance absente = %v, attendu ErrNoInstance", err)
	}

	// Doctor passe par le démon et revient rempli.
	if doc, err := r.Doctor(); err != nil || doc.ConfigPath == "" {
		t.Fatalf("Doctor via démon : ConfigPath vide (err=%v)", err)
	}
}
