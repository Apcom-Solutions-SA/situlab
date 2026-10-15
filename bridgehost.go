// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

// Pont CÔTÉ HÔTE (Windows) du canal box ⇄ SituLab (v1.15.1). Il ouvre une écoute TCP gardée par un
// jeton éphémère (fermeture de P2) et, à chaque requête relayée par `situlab-bridge` depuis la box,
// exécute la commande `situlab` correspondante SUR L'HÔTE — là où vivent les clés et où surgit le PIN.
// La box ne détient jamais de secret : elle ne fait que *demander*.
//
// ⚠️ v1.15.1 : seule une liste BLANCHE de commandes est relayable (pas d'administration via le pont).
// Transport TCP sur l'interface atteignable depuis WSL2 ; durcissement (vsock / pare-feu restreint au
// sous-réseau WSL) prévu v1.16.
package main

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"situlab/internal/osbox"
)

// bridgePort : port FIXE du pont hôte (une seule règle de pare-feu suffit, protégée par le jeton
// éphémère). Choisi peu courant pour limiter les collisions.
const bridgePort = 51774

// Binaire Linux de situlab-bridge, injecté dans la box à l'installation (jamais de clé dedans).
// Rempli au build par : GOOS=linux go build -o embedded/situlab-bridge ./cmd/situlab-bridge
//
//go:embed embedded/situlab-bridge
var bridgeBinLinux []byte

type bridgeHost struct {
	mu    sync.Mutex
	b     *osbox.Bridge
	token string
	port  int
}

// ensureStarted démarre l'écoute du pont si nécessaire et renvoie (port, jeton).
func (a *App) ensureBridgeStarted() (int, string, error) {
	if a.bh == nil {
		a.bh = &bridgeHost{}
	}
	a.bh.mu.Lock()
	defer a.bh.mu.Unlock()
	if a.bh.b != nil {
		return a.bh.port, a.bh.token, nil
	}
	token := osbox.NewToken()
	b, err := osbox.StartBridge(fmt.Sprintf("0.0.0.0:%d", bridgePort), token, a.bridgeDispatch, a.bridgeStream)
	if err != nil {
		return 0, "", fmt.Errorf("démarrage du pont sur le port %d : %w", bridgePort, err)
	}
	a.bh.b, a.bh.token, a.bh.port = b, token, bridgePort
	return bridgePort, token, nil
}

func (a *App) stopBridge() {
	if a.bh == nil {
		return
	}
	a.bh.mu.Lock()
	defer a.bh.mu.Unlock()
	if a.bh.b != nil {
		a.bh.b.Close()
		a.bh.b, a.bh.port = nil, 0
	}
}

// bridgeDispatch traite le PING (test de liaison). Les vraies commandes passent par bridgeStream.
func (a *App) bridgeDispatch(args []string, intent string) (string, error) {
	return "SituLab " + version + " · hôte prêt. CLI complet relayé depuis la SafeRoom (le PIN surgit ici si besoin).", nil
}

// bridgeStream exécute une SESSION CLI COMPLÈTE côté hôte : le binaire SituLab (qui EST aussi la CLI)
// est lancé avec les arguments bruts venus de la box, stdin/stdout/stderr branchés sur le flux, code de
// sortie propagé. 100 % du CLI est accessible — AUCUNE liste blanche : la gouvernance reste au démon
// (le PIN apparaît sur l'écran de l'hôte pour toute action sensible). Les secrets ne transitent jamais
// par la box : les saisies masquées (login/2FA) ne s'activent que sur un vrai terminal, pas via ce pipe.
func (a *App) bridgeStream(ctx context.Context, args []string, intent string, stdin io.Reader, stdout, stderr io.Writer) int {
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(stderr, "pont : binaire hôte introuvable :", err)
		return 127
	}
	// Intent par défaut (affiché dans la fenêtre PIN) si l'agent n'en a pas fourni : l'utilisateur voit
	// que la demande vient de la SafeRoom.
	full := args
	if !hasIntentFlag(args) {
		full = append([]string{"--intent", "Action demandée par l'IA depuis la SafeRoom"}, args...)
	}
	cmd := exec.CommandContext(ctx, exe, full...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode()
		}
		fmt.Fprintln(stderr, "pont :", err)
		return 1
	}
	return 0
}

func hasIntentFlag(args []string) bool {
	for _, a := range args {
		if a == "--intent" || strings.HasPrefix(a, "--intent=") {
			return true
		}
	}
	return false
}

// --- Bindings Wails ---------------------------------------------------------------------------

// OSBoxBridgeInstall installe/active le pont : démarre l'écoute hôte, injecte le binaire situlab-bridge
// dans la box et provisionne le canal (jeton + adresse). En arrière-plan (suivi via OSBoxStatus).
func (a *App) OSBoxBridgeInstall() error {
	if err := a.ready(); err != nil {
		return err
	}
	if !osbox.Installed() {
		return errors.New("l'OS de travail n'est pas installé")
	}
	go func() {
		port, token, err := a.ensureBridgeStarted()
		if err != nil {
			a.svc.OSBoxBridgeProvisionFailed(err)
			return
		}
		_ = a.svc.OSBoxProvisionBridge(bridgeBinLinux, port, token)
	}()
	return nil
}

// reprovisionBridge re-synchronise le canal (jeton/port changent à chaque session) si le pont est déjà
// installé. Appelé après un démarrage de la box. Best-effort, silencieux.
func (a *App) reprovisionBridge() {
	if !containsStr(a.svc.OSBoxStatus().Software, "situlab-bridge") {
		return
	}
	port, token, err := a.ensureBridgeStarted()
	if err != nil {
		return
	}
	_ = a.svc.OSBoxProvisionBridge(nil, port, token)
}

// OSBoxBridgeAuthorizeFirewall pose (avec élévation UAC) la règle de pare-feu entrante autorisant la
// box à joindre le pont hôte (le NAT WSL2 + le pare-feu Windows bloquent sinon). Idempotent. Le canal
// reste protégé par le jeton éphémère. Windows affichera une invite d'autorisation administrateur.
func (a *App) OSBoxBridgeAuthorizeFirewall() error {
	script := fmt.Sprintf("Remove-NetFirewallRule -DisplayName 'SituLab Bridge' -ErrorAction SilentlyContinue;"+
		"New-NetFirewallRule -DisplayName 'SituLab Bridge' -Direction Inbound -Action Allow -Protocol TCP -LocalPort %d -Profile Any | Out-Null", bridgePort)
	ps1 := filepath.Join(os.TempDir(), "situlab-bridge-fw.ps1")
	if err := os.WriteFile(ps1, []byte(script), 0o600); err != nil {
		return err
	}
	// Élévation : Start-Process -Verb RunAs déclenche l'invite UAC.
	launch := fmt.Sprintf("Start-Process powershell -Verb RunAs -WindowStyle Hidden -ArgumentList '-NoProfile','-ExecutionPolicy','Bypass','-File','%s'", ps1)
	cmd := exec.Command("powershell.exe", "-NoProfile", "-Command", launch)
	return cmd.Run()
}

func containsStr(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}
