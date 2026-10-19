// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"situlab/internal/config"
	"situlab/internal/osbox"
)

// agentsBoxDoc : la doc agent DÉPOSÉE DANS LA BOX à l'installation du pont (variante SafeRoom : piloter
// SituLab via `situlab-bridge`/le raccourci `situlab`, ouvrir un KERNEL par `kernel cat`). Distincte de
// docs/AGENTS.md (poste), qui parle du binaire `situlab` local.
//
//go:embed agents_box.md
var agentsBoxDoc string

// Orchestration de l'« OS de travail séparé » (v1.15), OPT-IN. Tant que config.OSBox.Enabled est faux,
// AUCUNE VM/image n'est créée. Le cœur de sécurité (démon, keyring, PIN, périmètres) est RÉUTILISÉ tel
// quel : ce module n'ajoute aucune surface critique, il orchestre `wsl.exe` et pose une box isolée.
//
// ⚠️ v1.15 : backend Windows/WSL2 uniquement. Les opérations lourdes (prepare/reset) ne s'exécutent que
// sur action EXPLICITE de l'utilisateur, après l'écran d'avertissement (taille, durée, connexion stable).

// URL de l'image rootfs Ubuntu pour WSL (téléchargée à l'activation, JAMAIS bundlée). Vérifiable par la
// gouvernance ; à terme, contrôle de somme/signature avant import (noté comme durcissement v1.16).
const osboxRootfsURL = "https://cloud-images.ubuntu.com/wsl/releases/noble/current/ubuntu-noble-wsl-amd64-wsl.rootfs.tar.gz"

// OSBoxStatus renvoie l'état consolidé (config + détection live). Lecture, pas de PIN.
func (s *Service) OSBoxStatus() osbox.Status {
	cfg, _ := config.Load()
	st := osbox.Status{Backend: "wsl2", Distro: osbox.BoxDistro, State: osbox.StateNone}
	if cfg != nil && cfg.OSBox != nil {
		st.Enabled = cfg.OSBox.Enabled
		if cfg.OSBox.State != "" {
			st.State = cfg.OSBox.State
		}
		st.Software = append([]string(nil), cfg.OSBox.Software...)
		st.LastErr = cfg.OSBox.LastErr
	}
	st.Available, st.AvailableHint = osbox.Available()
	if st.Available && st.Enabled {
		st.Installed = osbox.Installed()
		st.Running = st.Installed && osbox.Running()
		// Recale l'état lisible depuis la réalité — mais NE MASQUE PAS une erreur (sinon
		// l'utilisateur ne verrait jamais un échec d'installation ; bug corrigé en 1.15.1).
		if !st.Installed {
			if st.State != osbox.StatePreparing && st.State != osbox.StateError {
				st.State = osbox.StateNone
			}
		} else if st.Running {
			st.State = osbox.StateReady
		} else {
			st.State = osbox.StateStopped
		}
	}
	// Injecte la progression live (étapes + %) depuis la mémoire. Si une installation est en cours,
	// l'état d'affichage est forcé à "preparing" quoi qu'en dise la config relue.
	prog := s.osboxSnapshot()
	st.Phase, st.Pct, st.Steps, st.Busy = prog.Phase, prog.Pct, prog.Steps, prog.Active
	if prog.Active {
		st.State = osbox.StatePreparing
	}
	// Détection LIVE des logiciels réellement présents dans la box (ex. Claude installé hors-app, ou
	// liste persistée désynchronisée). Fusionnée avec la liste persistée. Ignorée pendant une opération
	// (box occupée) ou si la box ne tourne pas.
	if st.Running && !prog.Active {
		st.Software = unionSW(st.Software, s.osboxDetectCached())
	}
	return st
}

// osboxDetectCached renvoie la liste des logiciels détectés dans la box, avec un cache à TTL court pour
// éviter de sonder wsl à chaque appel (le polling appelle OSBoxStatus très souvent).
func (s *Service) osboxDetectCached() []string {
	s.osboxDetMu.Lock()
	if time.Since(s.osboxDetAt) < 6*time.Second && s.osboxDetSW != nil {
		sw := append([]string(nil), s.osboxDetSW...)
		s.osboxDetMu.Unlock()
		return sw
	}
	s.osboxDetMu.Unlock()

	sw := s.osboxDetectSoftware()

	s.osboxDetMu.Lock()
	s.osboxDetAt, s.osboxDetSW = time.Now(), sw
	s.osboxDetMu.Unlock()
	return sw
}

// osboxDetectSoftware sonde la box pour les logiciels installés (un seul appel wsl combiné).
func (s *Service) osboxDetectSoftware() []string {
	out, err := osbox.Exec(8*time.Second, "sh", "-lc",
		"command -v claude >/dev/null 2>&1 && echo claude-code; command -v node >/dev/null 2>&1 && echo node; [ -x /usr/local/bin/situlab-bridge ] && echo situlab-bridge")
	if err != nil {
		return nil
	}
	var sw []string
	for _, f := range strings.Fields(out) {
		sw = append(sw, f)
	}
	return sw
}

// unionSW fusionne deux listes de logiciels sans doublon (ordre : a puis les nouveaux de b).
func unionSW(a, b []string) []string {
	out := append([]string(nil), a...)
	for _, x := range b {
		if !contains(out, x) {
			out = append(out, x)
		}
	}
	return out
}

// --- Suivi de progression (en mémoire, éphémère) -----------------------------------------------

func (s *Service) osboxProgReset() {
	s.osboxMu.Lock()
	s.osboxProg = osbox.Progress{Active: true, Pct: -1, Steps: []osbox.Step{}}
	s.osboxMu.Unlock()
}
func (s *Service) osboxProgEnd() {
	s.osboxMu.Lock()
	s.osboxProg.Active = false
	s.osboxProg.Phase = ""
	s.osboxMu.Unlock()
}

// osboxBeginStep ouvre une étape (status "doing") et renvoie son index.
func (s *Service) osboxBeginStep(label string) int {
	s.osboxMu.Lock()
	defer s.osboxMu.Unlock()
	s.osboxProg.Phase = label
	s.osboxProg.Pct = -1
	s.osboxProg.Steps = append(s.osboxProg.Steps, osbox.Step{Label: label, Status: "doing"})
	return len(s.osboxProg.Steps) - 1
}

// osboxEndStep clôt une étape : status "ok" ou "fail", avec un éventuel détail.
func (s *Service) osboxEndStep(i int, status, detail string) {
	s.osboxMu.Lock()
	defer s.osboxMu.Unlock()
	if i >= 0 && i < len(s.osboxProg.Steps) {
		s.osboxProg.Steps[i].Status = status
		s.osboxProg.Steps[i].Detail = detail
	}
}

func (s *Service) osboxSetPct(p int) {
	s.osboxMu.Lock()
	s.osboxProg.Pct = p
	s.osboxMu.Unlock()
}

// osboxSnapshot renvoie une copie défensive de la progression courante.
func (s *Service) osboxSnapshot() osbox.Progress {
	s.osboxMu.Lock()
	defer s.osboxMu.Unlock()
	cp := s.osboxProg
	cp.Steps = append([]osbox.Step(nil), s.osboxProg.Steps...)
	return cp
}

// OSBoxSetEnabled active/désactive l'OPTION (toggle). Activer ne crée RIEN : la préparation est un geste
// distinct (OSBoxPrepare), déclenché après l'écran d'avertissement. Désactiver arrête la box (best-effort).
func (s *Service) OSBoxSetEnabled(on bool) error {
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.OSBox == nil {
		cfg.OSBox = &config.OSBox{}
	}
	cfg.OSBox.Enabled = on
	cfg.OSBox.Backend = "wsl2"
	if !on {
		cfg.OSBox.State = osbox.StateStopped
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	if !on {
		go func() { _ = osbox.Terminate() }() // best-effort, non bloquant
	}
	return nil
}

// osboxSetState met à jour l'état + l'erreur (best-effort).
func (s *Service) osboxSetState(state, lastErr string, software []string) {
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return
	}
	if cfg.OSBox == nil {
		cfg.OSBox = &config.OSBox{Enabled: true}
	}
	cfg.OSBox.State = state
	cfg.OSBox.LastErr = lastErr
	if software != nil {
		cfg.OSBox.Software = software
	}
	if cfg.Save() == nil {
		s.publish(cfg)
	}
}

// OSBoxPrepare installe la box (gated, long). Pré-vol → téléchargement/import de l'image → isolation.
// À appeler UNIQUEMENT après l'écran d'avertissement, sur action de l'utilisateur. Idempotent.
func (s *Service) OSBoxPrepare() error {
	// Garde anti-double-lancement : si une installation tourne déjà, on refuse (et on ne relance
	// pas un second téléchargement concurrent qui corromprait le fichier .part).
	s.osboxMu.Lock()
	if s.osboxProg.Active {
		s.osboxMu.Unlock()
		return errors.New("une installation est déjà en cours")
	}
	s.osboxMu.Unlock()
	s.osboxProgReset()
	defer s.osboxProgEnd()

	// Étape 1 : vérification du poste.
	i := s.osboxBeginStep("Vérification du poste (WSL2)")
	if ok, hint := osbox.Available(); !ok {
		s.osboxEndStep(i, "fail", hint)
		s.osboxSetState(osbox.StateError, hint, nil)
		return errors.New(hint)
	}
	s.osboxEndStep(i, "ok", "")
	s.osboxSetState(osbox.StatePreparing, "", nil)

	// Cas « déjà installée » : on s'assure juste de l'isolation puis on marque prête.
	if osbox.Installed() {
		i = s.osboxBeginStep("Isolation de la box")
		if err := osbox.EnsureIsolation(); err != nil {
			s.osboxEndStep(i, "fail", err.Error())
			s.osboxSetState(osbox.StateError, err.Error(), nil)
			return err
		}
		s.osboxEndStep(i, "ok", "")
		s.osboxSetState(osbox.StateReady, "", nil)
		return nil
	}

	// Étape 2 : téléchargement de l'image rootfs (avec pourcentage).
	i = s.osboxBeginStep("Téléchargement de l'image Ubuntu")
	rootfs, err := s.osboxFetchRootfs()
	if err != nil {
		s.osboxEndStep(i, "fail", err.Error())
		s.osboxSetState(osbox.StateError, "téléchargement de l'image : "+err.Error(), nil)
		return err
	}
	s.osboxEndStep(i, "ok", "")

	// Étape 3 : import comme distribution DÉDIÉE.
	i = s.osboxBeginStep("Installation du système (import)")
	installDir := filepath.Join(osboxDataDir(), "box")
	_ = os.MkdirAll(installDir, 0o750)
	if _, err := osbox.Import(osbox.BoxDistro, installDir, rootfs); err != nil {
		s.osboxEndStep(i, "fail", err.Error())
		s.osboxSetState(osbox.StateError, "import de la box : "+err.Error(), nil)
		return err
	}
	s.osboxEndStep(i, "ok", "")

	// Étape 4 : isolation (§10) — automount=false + interop off. Sans cela, l'OS « séparé » serait factice.
	i = s.osboxBeginStep("Isolation (coupure de l'accès au disque hôte)")
	if err := osbox.EnsureIsolation(); err != nil {
		s.osboxEndStep(i, "fail", err.Error())
		s.osboxSetState(osbox.StateError, err.Error(), nil)
		return err
	}
	s.osboxEndStep(i, "ok", "")

	// Étape 5 : test d'acceptation — depuis la box, le disque hôte ne doit pas être lisible.
	i = s.osboxBeginStep("Test de sécurité (isolation effective)")
	if !osbox.IsolationOK() {
		const msg = "l'isolation n'est pas effective (le disque hôte reste visible depuis la box) — installation refusée par sécurité"
		s.osboxEndStep(i, "fail", msg)
		s.osboxSetState(osbox.StateError, msg, nil)
		return errors.New(msg)
	}
	s.osboxEndStep(i, "ok", "")
	s.osboxSetState(osbox.StateReady, "", []string{})
	return nil
}

// OSBoxStart / OSBoxStop / OSBoxReset : cycle de vie.
func (s *Service) OSBoxStart() error {
	if !osbox.Installed() {
		return errors.New("l'OS de travail n'est pas installé")
	}
	if _, err := osbox.Exec(15*time.Second, "true"); err != nil { // réveille la distro
		s.osboxSetState(osbox.StateError, err.Error(), nil)
		return err
	}
	s.osboxSetState(osbox.StateReady, "", nil)
	return nil
}
func (s *Service) OSBoxStop() error {
	err := osbox.Terminate()
	s.osboxSetState(osbox.StateStopped, errText(err), nil)
	return err
}
func (s *Service) OSBoxReset() error {
	if err := osbox.Unregister(); err != nil {
		s.osboxSetState(osbox.StateError, err.Error(), nil)
		return err
	}
	s.osboxSetState(osbox.StateNone, "", []string{})
	return nil
}

// OSBoxExec exécute une commande DANS la box (terminal de l'onglet OS). La box n'a aucune clé.
func (s *Service) OSBoxExec(line string) (string, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", nil
	}
	return osbox.Exec(60*time.Second, "sh", "-lc", line)
}

// OSBoxExecResult : sortie d'une commande de session + répertoire courant après exécution.
type OSBoxExecResult struct {
	Out string `json:"out"`
	Cwd string `json:"cwd"`
}

// osboxCwdMark : marqueur (caractères de contrôle, improbables dans une sortie) séparant la sortie de
// la commande du répertoire courant renvoyé.
const osboxCwdMark = "\x1e\x1eSLCWD\x1e\x1e"

// OSBoxExecSession exécute une commande dans un répertoire donné et renvoie la sortie + le NOUVEAU
// répertoire courant. Cela donne au terminal une vraie persistance de `cd` entre les commandes d'un
// même onglet (les process restent mono-coup — un shell interactif en flux continu est prévu v1.17).
func (s *Service) OSBoxExecSession(cwd, line string) (OSBoxExecResult, error) {
	line = strings.TrimSpace(line)
	if cwd == "" {
		cwd = "/root"
	}
	if line == "" {
		return OSBoxExecResult{Cwd: cwd}, nil
	}
	// On fusionne stderr dans stdout ({ … } 2>&1) : la plupart des outils (dont claude) écrivent leurs
	// messages sur stderr ; sans cela, la console resterait muette en cas de succès.
	script := "cd " + shQuoteCore(cwd) + " 2>/dev/null; { " + line + " ; } 2>&1; printf '%s%s' '" + osboxCwdMark + "' \"$(pwd)\""
	out, err := osbox.Exec(60*time.Second, "sh", "-lc", script)
	res := OSBoxExecResult{Cwd: cwd}
	if i := strings.LastIndex(out, osboxCwdMark); i >= 0 {
		res.Cwd = strings.TrimSpace(out[i+len(osboxCwdMark):])
		out = out[:i]
	}
	res.Out = out
	return res, err
}

// shQuoteCore protège une chaîne pour un argument shell POSIX (quotes simples).
func shQuoteCore(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// Libellés lisibles des logiciels (pour le suivi de progression).
var osboxSoftLabel = map[string]string{"claude-code": "Claude Code", "node": "Node.js", "situlab-bridge": "SituLab Bridge"}

// OSBoxProvisionBridge installe/synchronise le pont CÔTÉ BOX : dépose le binaire situlab-bridge (si
// fourni), écrit la configuration du canal (hôte + port + jeton) et vérifie la liaison. L'écoute hôte
// est démarrée par l'app AVANT cet appel (port/token lui sont transmis). Suivi via le tunnel d'étapes.
func (s *Service) OSBoxProvisionBridge(bin []byte, port int, token string) error {
	s.osboxMu.Lock()
	if s.osboxProg.Active {
		s.osboxMu.Unlock()
		return errors.New("une opération est déjà en cours")
	}
	s.osboxMu.Unlock()
	s.osboxProgReset()
	defer s.osboxProgEnd()

	if !osbox.Installed() {
		return errors.New("l'OS de travail n'est pas installé")
	}

	// 1) Déposer le binaire (à l'installation ; ignoré lors d'une simple re-synchronisation).
	if len(bin) > 0 {
		i := s.osboxBeginStep("Installation de SituLab Bridge dans la box")
		if out, err := osbox.ExecStdin(2*time.Minute, bin, "sh", "-c", "cat > /usr/local/bin/situlab-bridge && chmod +x /usr/local/bin/situlab-bridge"); err != nil {
			s.osboxEndStep(i, "fail", osboxErrDetail(out, err))
			return err
		}
		s.osboxEndStep(i, "ok", "")
	}

	// 2) Déterminer l'IP de l'hôte VUE DEPUIS LA BOX (passerelle WSL) et écrire la configuration.
	i := s.osboxBeginStep("Configuration du canal sécurisé (jeton + hôte)")
	// Passerelle WSL = IP de l'hôte Windows vue depuis la box. On lit `ip route show default` en argv PUR
	// (pas de `sh -c` ni d'awk inline : un pipeline quoté passé en argument à wsl.exe est ré-encodé de
	// façon fragile côté Windows et peut arriver corrompu dans la box — cause d'un « adresse introuvable »
	// alors que la route existe) et on extrait la passerelle CÔTÉ GO.
	// (On n'utilise PAS le nameserver de resolv.conf comme repli : c'est le DNS NAT interne de WSL, pas
	// une adresse où le pont hôte écoute — cela produirait une config silencieusement cassée.)
	// Démarrage à froid : eth0 / la route par défaut peuvent n'apparaître qu'après un instant → on réessaie.
	var gw string
	deadline := time.Now().Add(40 * time.Second)
	for {
		out, _ := osbox.Exec(10*time.Second, "ip", "route", "show", "default")
		if gw = parseDefaultGateway(out); gw != "" {
			break
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(time.Second)
	}
	if gw == "" {
		s.osboxEndStep(i, "fail", "impossible de déterminer l'adresse de l'hôte (passerelle WSL) depuis la box — réseau de la box pas encore prêt ; réessayez via « Tester la liaison »")
		return errors.New("adresse hôte introuvable depuis la box")
	}
	cfg := fmt.Sprintf(`{"host":%q,"port":%d,"token":%q}`, gw, port, token)
	if out, err := osbox.ExecStdin(20*time.Second, []byte(cfg), "sh", "-c", "mkdir -p /etc/situlab && cat > /etc/situlab/bridge.json && chmod 600 /etc/situlab/bridge.json"); err != nil {
		s.osboxEndStep(i, "fail", osboxErrDetail(out, err))
		return err
	}
	s.osboxEndStep(i, "ok", "")

	// 2b) Déposer la doc agent, un raccourci `situlab` et une bannière. Sans cela un agent dans la box
	// ne trouve aucune doc et cherche un binaire `situlab` absent → il conclut « non paramétré ». Le
	// raccourci relaie `situlab <cmd>` au poste ; la doc explique l'ouverture KERNEL via `kernel cat`.
	// Rejoué à chaque provision (install ET re-synchronisation) pour rester présent.
	i = s.osboxBeginStep("Doc agent + raccourci situlab + bannière")
	wrapper := "#!/bin/sh\n# SafeRoom : relaie toute commande `situlab` au poste via le pont.\nexec /usr/local/bin/situlab-bridge \"$@\"\n"
	motd := "SituLab Bridge — SafeRoom. Pilote SituLab par le pont : `situlab <commande>`.\n" +
		"Doc agent : /root/AGENTS.md · Ouvrir un KERNEL : `situlab kernel list` puis `situlab kernel cat`.\n"
	for _, st := range []struct {
		data []byte
		sh   string
	}{
		{[]byte(wrapper), "cat > /usr/local/bin/situlab && chmod +x /usr/local/bin/situlab"},
		{[]byte(agentsBoxDoc), "mkdir -p /etc/situlab && cat > /etc/situlab/AGENTS.md && cp -f /etc/situlab/AGENTS.md /root/AGENTS.md"},
		{[]byte(motd), "cat > /etc/motd"},
	} {
		if out, err := osbox.ExecStdin(20*time.Second, st.data, "sh", "-c", st.sh); err != nil {
			s.osboxEndStep(i, "fail", osboxErrDetail(out, err))
			return err
		}
	}
	s.osboxEndStep(i, "ok", "")

	// 3) Tester la liaison depuis la box.
	i = s.osboxBeginStep("Test de liaison box → SituLab")
	out, err := osbox.Exec(30*time.Second, "sh", "-lc", "/usr/local/bin/situlab-bridge connect")
	if err != nil || !strings.Contains(out, "Connecté") {
		s.osboxEndStep(i, "fail", "la box ne joint pas SituLab — pare-feu Windows à autoriser pour le sous-réseau WSL. "+strings.TrimSpace(out))
		// On marque tout de même installé (binaire + config posés) ; l'erreur est explicite.
		cur := s.OSBoxStatus().Software
		if !contains(cur, "situlab-bridge") {
			cur = append(cur, "situlab-bridge")
		}
		s.osboxSetState(osbox.StateReady, "", cur)
		return fmt.Errorf("liaison box→hôte échouée : %s", strings.TrimSpace(out))
	}
	s.osboxEndStep(i, "ok", "")

	cur := s.OSBoxStatus().Software
	if !contains(cur, "situlab-bridge") {
		cur = append(cur, "situlab-bridge")
	}
	s.osboxSetState(osbox.StateReady, "", cur)
	return nil
}

// OSBoxBridgeProvisionFailed enregistre un échec de démarrage du pont hôte (pour affichage).
func (s *Service) OSBoxBridgeProvisionFailed(err error) {
	s.osboxProgReset()
	i := s.osboxBeginStep("Démarrage du pont hôte")
	s.osboxEndStep(i, "fail", errText(err))
	s.osboxProgEnd()
}

// OSBoxInstallSoftware installe un logiciel dans la box selon une recette déclarative (carte). Remonte
// sa progression via le même tunnel d'étapes que l'installation de l'OS (ce qui tourne / réussit / échoue).
func (s *Service) OSBoxInstallSoftware(id string) (string, error) {
	recipe, ok := osboxRecipes[id]
	if !ok {
		return "", fmt.Errorf("logiciel inconnu : %q", id)
	}
	// Garde : une seule opération longue à la fois (installation OS ou logiciel).
	s.osboxMu.Lock()
	if s.osboxProg.Active {
		s.osboxMu.Unlock()
		return "", errors.New("une opération est déjà en cours")
	}
	s.osboxMu.Unlock()
	s.osboxProgReset()
	defer s.osboxProgEnd()

	label := osboxSoftLabel[id]
	if label == "" {
		label = id
	}
	i := s.osboxBeginStep("Installation de " + label)
	out, err := osbox.Exec(10*time.Minute, "sh", "-lc", recipe)
	if err != nil {
		s.osboxEndStep(i, "fail", osboxErrDetail(out, err))
		return out, err
	}
	s.osboxEndStep(i, "ok", "")
	// Marque installé.
	cur := s.OSBoxStatus().Software
	if !contains(cur, id) {
		cur = append(cur, id)
	}
	s.osboxSetState(osbox.StateReady, "", cur)
	return out, nil
}

// osboxErrDetail compose un message d'échec lisible : l'erreur + la fin de la sortie (souvent la cause).
func osboxErrDetail(out string, err error) string {
	msg := err.Error()
	out = strings.TrimSpace(out)
	if out != "" {
		if len(out) > 300 {
			out = "…" + out[len(out)-300:]
		}
		msg += " — " + out
	}
	return msg
}

// parseDefaultGateway extrait l'IP de passerelle d'une sortie « ip route show default »
// (ex. « default via 192.168.32.1 dev eth0 proto kernel »). Renvoie "" si absente.
func parseDefaultGateway(out string) string {
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		for i := 0; i+1 < len(f); i++ {
			if f[i] == "via" {
				return f[i+1]
			}
		}
	}
	return ""
}

// Recettes déclaratives (extensibles sans retoucher l'app). Exécutées DANS la box.
var osboxRecipes = map[string]string{
	// Claude Code : via npm (nécessite Node). Chaînage STRICT (&&) pour ne pas enchaîner après un échec,
	// réparation préalable de dpkg (une install interrompue laisse la base de paquets bancale), et mode
	// non interactif. npm global → sudo (l'agent tourne en root dans la box, sudo est disponible).
	"claude-code": "export DEBIAN_FRONTEND=noninteractive; sudo dpkg --configure -a >/dev/null 2>&1 || true; sudo apt-get update >/dev/null 2>&1 || true; sudo apt-get install -y nodejs npm >/dev/null 2>&1 || true; command -v npm >/dev/null 2>&1 || { echo 'ERREUR : Node/npm introuvable après installation'; exit 1; }; sudo npm i -g @anthropic-ai/claude-code >/dev/null 2>&1 || { echo 'ERREUR : installation de Claude Code (npm) échouée'; exit 1; }; command -v claude >/dev/null 2>&1 && echo OK-claude-code || { echo 'ERREUR : Claude Code introuvable après installation'; exit 1; }",
	"node":        "export DEBIAN_FRONTEND=noninteractive; sudo dpkg --configure -a >/dev/null 2>&1 || true; sudo apt-get update >/dev/null 2>&1 || true; sudo apt-get install -y nodejs npm >/dev/null 2>&1 || true; command -v node >/dev/null 2>&1 && command -v npm >/dev/null 2>&1 && echo \"OK-node $(node --version)\" || { echo 'ERREUR : Node/npm introuvable après installation'; exit 1; }",
}

// osboxFetchRootfs télécharge l'image rootfs dans le cache (si absente) et renvoie son chemin.
func (s *Service) osboxFetchRootfs() (string, error) {
	dir := osboxDataDir()
	_ = os.MkdirAll(dir, 0o750)
	dst := filepath.Join(dir, "ubuntu-wsl.rootfs.tar.gz")
	if fi, err := os.Stat(dst); err == nil && fi.Size() > 10<<20 { // déjà en cache (>10 Mo)
		return dst, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, osboxRootfsURL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("téléchargement HTTP %d", resp.StatusCode)
	}
	tmp := dst + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	// Recopie avec suivi du pourcentage (Content-Length connu → barre de progression réelle).
	pw := &osboxProgWriter{total: resp.ContentLength, set: s.osboxSetPct, last: -1}
	if _, err := io.Copy(f, io.TeeReader(resp.Body, pw)); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return "", err
	}
	f.Close()
	s.osboxSetPct(100)
	if err := os.Rename(tmp, dst); err != nil {
		return "", err
	}
	return dst, nil
}

// osboxProgWriter compte les octets reçus et publie un pourcentage (si la taille totale est connue).
type osboxProgWriter struct {
	total int64
	n     int64
	last  int
	set   func(int)
}

func (w *osboxProgWriter) Write(p []byte) (int, error) {
	w.n += int64(len(p))
	if w.total > 0 && w.set != nil {
		pct := int(w.n * 100 / w.total)
		if pct != w.last {
			w.last = pct
			w.set(pct)
		}
	}
	return len(p), nil
}

func osboxDataDir() string { return filepath.Join(config.Dir(), "osbox") }

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}
