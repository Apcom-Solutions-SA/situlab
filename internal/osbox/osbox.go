// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

// Package osbox orchestre l'« OS de travail séparé » (SituLab v1.15) : une VM invitée jetable où une IA
// (Claude Code…) travaille SANS jamais toucher la machine hôte ni ses clés. SituLab (hôte) reste la seule
// porte vers git/cloud. OPT-IN : tant que l'utilisateur n'a pas activé l'option, RIEN n'est créé.
//
// v1.15 : backend WINDOWS/WSL2 uniquement (macOS/Linux en v1.16+). SituLab ORCHESTRE `wsl.exe` ; il
// n'écrit pas d'hyperviseur. L'image Ubuntu est téléchargée à l'activation (jamais bundlée).
//
// ISOLATION (§10 de la note d'architecture, NON négociable) : la distribution dédiée est créée avec
// `wsl.conf` → automount=false + interop désactivé, de sorte qu'AUCUN fichier de l'hôte n'est visible
// depuis la box. Sans cela, l'« OS séparé » serait factice. Aucun dossier hôte n'est monté.
package osbox

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
	"unicode/utf16"
)

// BoxDistro : nom de la distribution WSL dédiée à SituLab (isolée de toute autre distro de l'utilisateur).
const BoxDistro = "SituLab-Box"

// États de la machine (miroir de config.OSBox.State).
const (
	StateNone      = "none"      // non activé
	StatePreparing = "preparing" // installation en cours
	StateReady     = "ready"     // prête, démarrée
	StateStopped   = "stopped"   // installée mais arrêtée
	StateError     = "error"
)

// Step : une étape du tunnel d'installation, pour un suivi lisible (ce qui marche / échoue).
type Step struct {
	Label  string `json:"label"`
	Status string `json:"status"`           // doing | ok | fail
	Detail string `json:"detail,omitempty"` // message (notamment en cas d'échec)
}

// Progress : progression LIVE de l'installation (tenue en mémoire, éphémère, non persistée).
type Progress struct {
	Active bool   `json:"active"`          // une installation est en cours
	Phase  string `json:"phase,omitempty"` // libellé de l'étape courante
	Pct    int    `json:"pct"`             // 0..100 pour le téléchargement ; -1 = indéterminé
	Steps  []Step `json:"steps,omitempty"` // journal des étapes (ce qui a réussi / échoué)
}

// Status : état consolidé pour l'interface.
type Status struct {
	Enabled       bool     `json:"enabled"`
	State         string   `json:"state"`
	Backend       string   `json:"backend"`
	Available     bool     `json:"available"`      // moteur de virtualisation présent et utilisable
	AvailableHint string   `json:"available_hint"` // message si indisponible (que faire)
	Installed     bool     `json:"installed"`      // la distribution SituLab-Box existe
	Running       bool     `json:"running"`        // la distribution est démarrée
	Distro        string   `json:"distro"`
	Software      []string `json:"software"`
	LastErr       string   `json:"last_err"`
	// Suivi live de l'installation (injecté par le cœur depuis la progression en mémoire).
	Phase string `json:"phase,omitempty"`
	Pct   int    `json:"pct"`
	Steps []Step `json:"steps,omitempty"`
	Busy  bool   `json:"busy"` // installation en cours (bloque un second lancement)
}

// --- Détection du backend ---------------------------------------------------------------------

// Available indique si le backend WSL2 est utilisable, avec un message d'aide sinon.
func Available() (bool, string) {
	if runtime.GOOS != "windows" {
		return false, "L'OS de travail (WSL2) n'est disponible que sous Windows en v1.15 (macOS/Linux : v1.16+)."
	}
	if _, err := exec.LookPath("wsl.exe"); err != nil {
		return false, "WSL n'est pas installé. Windows peut l'installer en une fois : « wsl --install » (droits administrateur, un redémarrage est parfois nécessaire)."
	}
	if _, err := runWSL(6*time.Second, nil, "--status"); err != nil {
		return false, "WSL est présent mais pas initialisé. Lancez une fois « wsl --install », puis redémarrez si demandé."
	}
	return true, ""
}

// Installed indique si la distribution dédiée SituLab-Box existe déjà.
func Installed() bool {
	out, err := runWSL(8*time.Second, nil, "-l", "-q")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.EqualFold(strings.TrimSpace(line), BoxDistro) {
			return true
		}
	}
	return false
}

// Running indique si la box est démarrée (état « Running » dans `wsl -l -v`).
func Running() bool {
	out, err := runWSL(8*time.Second, nil, "-l", "-v")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(strings.TrimPrefix(strings.TrimSpace(line), "*"))
		if len(f) >= 2 && strings.EqualFold(f[0], BoxDistro) && strings.EqualFold(f[1], "Running") {
			return true
		}
	}
	return false
}

// --- Opérations WSL (gated : appelées uniquement sur action explicite de l'utilisateur) --------

// Exec exécute une commande DANS la box et renvoie sa sortie combinée. La box n'a aucune clé ; ces
// commandes travaillent dans l'OS invité isolé.
func Exec(timeout time.Duration, args ...string) (string, error) {
	if !Installed() {
		return "", errors.New("l'OS de travail n'est pas installé")
	}
	full := append([]string{"-d", BoxDistro, "--"}, args...)
	return runWSL(timeout, nil, full...)
}

// ExecStdin exécute une commande DANS la box en lui fournissant stdin (ex. écrire un binaire/fichier
// via `cat > …`). Renvoie la sortie combinée.
func ExecStdin(timeout time.Duration, stdin []byte, args ...string) (string, error) {
	if !Installed() {
		return "", errors.New("l'OS de travail n'est pas installé")
	}
	full := append([]string{"-d", BoxDistro, "--"}, args...)
	return runWSL(timeout, stdin, full...)
}

// Import crée la distribution DÉDIÉE à partir d'un rootfs (tar.gz), en WSL2. installDir = dossier qui
// portera le disque virtuel de la box. Long (dépend de la taille de l'image).
func Import(distro, installDir, rootfs string) (string, error) {
	return runWSL(20*time.Minute, nil, "--import", distro, installDir, rootfs, "--version", "2")
}

// Terminate arrête la box (sans la supprimer).
func Terminate() error {
	_, err := runWSL(15*time.Second, nil, "--terminate", BoxDistro)
	return err
}

// Unregister SUPPRIME la box (reset complet → box propre au prochain prepare). Irréversible côté box.
func Unregister() error {
	_, err := runWSL(60*time.Second, nil, "--unregister", BoxDistro)
	return err
}

// EnsureIsolation écrit /etc/wsl.conf dans la box pour couper automount + interop (isolation §10).
// À appeler juste après la création de la box, AVANT de l'utiliser. Idempotent.
func EnsureIsolation() error {
	// Écrit un wsl.conf minimal : pas de montage du disque hôte, pas d'interop Windows↔Linux.
	const conf = "[automount]\nenabled=false\n[interop]\nenabled=false\nappendWindowsPath=false\n"
	cmd := "printf '%s' " + shSingleQuote(conf) + " | tee /etc/wsl.conf >/dev/null"
	if _, err := Exec(20*time.Second, "sh", "-c", cmd); err != nil {
		return fmt.Errorf("écriture de wsl.conf (isolation) : %w", err)
	}
	// Redémarrage de la distro pour appliquer wsl.conf.
	_ = Terminate()
	return nil
}

// IsolationOK : test d'acceptation (§10) — depuis la box, le disque hôte ne doit PAS être lisible.
// On teste la VRAIE lisibilité (pas la simple existence de /mnt/c, qui peut subsister comme dossier
// vide même sans montage) : un disque C: monté exposerait /mnt/c/Windows et /mnt/c/Users, et
// apparaîtrait dans la table des montages (drvfs/9p). Isolation effective = aucun des trois.
func IsolationOK() bool {
	const probe = `
		if mount 2>/dev/null | grep -Eq " /mnt/c |drvfs"; then echo LEAK
		elif ls /mnt/c/Windows >/dev/null 2>&1 || ls /mnt/c/Users >/dev/null 2>&1; then echo LEAK
		else echo ISOLATED; fi`
	out, err := Exec(12*time.Second, "sh", "-c", probe)
	if err != nil {
		return false
	}
	return strings.Contains(out, "ISOLATED") && !strings.Contains(out, "LEAK")
}

// --- Bas niveau -------------------------------------------------------------------------------

// runWSL exécute wsl.exe et décode sa sortie (wsl.exe émet de l'UTF-16LE sur Windows). stdin optionnel.
func runWSL(timeout time.Duration, stdin []byte, args ...string) (string, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	cmd := exec.Command("wsl.exe", args...)
	hideConsole(cmd) // Windows : pas de fenêtre console parasite qui clignote (CREATE_NO_WINDOW).
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		return "", err
	}
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		res := decodeWSL(out.Bytes())
		if err != nil {
			return res, fmt.Errorf("%s : %s", err, strings.TrimSpace(decodeWSL(errb.Bytes())))
		}
		return res, nil
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		return "", errors.New("wsl : délai dépassé")
	}
}

// decodeWSL : wsl.exe produit souvent de l'UTF-16LE (avec BOM). On décode, sinon on renvoie tel quel.
func decodeWSL(b []byte) string {
	if len(b) >= 2 && b[0] == 0xFF && b[1] == 0xFE { // BOM UTF-16LE
		b = b[2:]
	}
	// Heuristique : beaucoup d'octets nuls → UTF-16LE.
	nul := 0
	for _, c := range b {
		if c == 0 {
			nul++
		}
	}
	if len(b) > 0 && nul*2 >= len(b) {
		u16 := make([]uint16, 0, len(b)/2)
		for i := 0; i+1 < len(b); i += 2 {
			u16 = append(u16, uint16(b[i])|uint16(b[i+1])<<8)
		}
		return string(utf16.Decode(u16))
	}
	return string(b)
}

// shSingleQuote protège une chaîne pour un argument shell POSIX entre quotes simples.
func shSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
