// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"situlab/internal/config"
)

// Effacement des données des AGENTS IA de ce poste (1.18.4, retour #14).
//
// Même esprit que Purge() pour SituLab, mais tourné vers les dossiers que les agents IA (Claude Code en
// tête) laissent sur la machine. Un agent a tendance à tout juger « important » et n'efface rien de
// lui-même ; SituLab propose une remise à blanc FRANCHE qui ne garde que le strict nécessaire : le LOGIN
// (pour ne pas devoir se ré-authentifier) et le minimum qui fait tourner le logiciel. Tout le reste part
// (sessions, historique, caches, projets). Irréversible — la confirmation textuelle « EFFACER » est
// exigée côté interface ET revérifiée ici (fail-safe). Ce n'est pas du git : ni PIN, ni réseau ; mais la
// lecture seule (assertWritable) la bloque, comme toute suppression.

// AgentPurgeConfirm : mot de confirmation textuel attendu pour une purge d'agent (parité avec
// l'effacement SituLab). Exporté : la séquence de fermeture (package main) le réutilise pour lancer
// les purges configurées sans redemander la saisie.
const AgentPurgeConfirm = "PURGER"

// agentProfile décrit un agent IA géré par SCRIPT (effacement direct par SituLab).
type agentProfile struct {
	key      string
	name     string
	dirOf    func() string // dossier de données de l'agent (absolu) ; "" si indéterminable
	preserve []string      // entrées à GARDER à la racine du dossier : login + minimum de fonctionnement
	note     string
}

// knownAgents : table des agents gérés. Claude Code d'abord (retour #14, feu vert gouvernance). On garde
// EXACTEMENT ce que la doctrine ne purge jamais : le login (.credentials.json) et le minimum de config/
// amorce (settings*.json, CLAUDE.md, plugins/). Le reste de ~/.claude repart à blanc.
func knownAgents() []agentProfile {
	return []agentProfile{
		{
			key:  "claude-code",
			name: "Claude Code",
			dirOf: func() string {
				return agentHomeDir(".claude")
			},
			preserve: []string{".credentials.json", "settings.json", "settings.local.json", "CLAUDE.md", "plugins"},
			note:     "Garde le login, les réglages, l'amorce et les extensions. Sessions, historique, caches et projets repartent à blanc.",
		},
		{
			key:  "codex",
			name: "Codex",
			dirOf: func() string {
				return agentHomeDir(".codex")
			},
			// Login + config + identité d'installation. Le reste (sessions, journaux, caches, bac à sable,
			// état) repart à blanc ; Codex re-provisionne au prochain lancement sans re-login.
			preserve: []string{"auth.json", "config.toml", "installation_id"},
			note:     "Garde le login, la configuration et l'identité d'installation. Sessions, journaux, caches et bac à sable repartent à blanc.",
		},
		{
			key:  "gemini",
			name: "Gemini CLI",
			dirOf: func() string {
				return agentHomeDir(".gemini")
			},
			preserve: []string{"oauth_creds.json", "google_accounts.json", "settings.json", "installation_id"},
			note:     "Garde le login Google et les réglages. Sessions, journaux et caches repartent à blanc.",
		},
		{
			key:  "copilot-cli",
			name: "GitHub Copilot CLI",
			dirOf: func() string {
				// ~/.config/github-copilot (Linux/mac, respecte XDG_CONFIG_HOME) ; %LocalAppData%\github-copilot (Windows).
				return agentConfigDir("github-copilot", "LOCALAPPDATA")
			},
			preserve: []string{"hosts.json", "apps.json", "versions.json"},
			note:     "Garde le jeton GitHub et la configuration. Caches et journaux repartent à blanc.",
		},
		{
			key:  "claude-desktop",
			name: "Claude Desktop",
			dirOf: func() string {
				return claudeDesktopDir()
			},
			// App Electron : on garde le login (stockage de session) et la config MCP ; on efface les
			// caches, journaux et données volumineuses. Périmètre prudent, à confirmer à l'usage.
			preserve: []string{"claude_desktop_config.json", "Local Storage", "Session Storage", "Cookies", "Cookies-journal", "IndexedDB", "Preferences"},
			note:     "Garde le login (session) et la configuration MCP. Caches, journaux et données temporaires repartent à blanc.",
		},
	}
}

// agentHomeDir renvoie <home>/<name> (absolu), ou "" si le home est indéterminable.
func agentHomeDir(name string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, name)
}

// agentConfigDir résout un dossier de config applicatif selon l'OS : sur Windows, sous le dossier donné
// par la variable winEnv (ex. LOCALAPPDATA) ; ailleurs, sous ~/.config (ou XDG_CONFIG_HOME). "" si rien.
func agentConfigDir(name, winEnv string) string {
	if runtime.GOOS == "windows" {
		if base := os.Getenv(winEnv); base != "" {
			return filepath.Join(base, name)
		}
		return ""
	}
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, name)
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".config", name)
}

// claudeDesktopDir : dossier de données de l'app Claude Desktop selon l'OS.
func claudeDesktopDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	switch runtime.GOOS {
	case "windows":
		if ad := os.Getenv("APPDATA"); ad != "" {
			return filepath.Join(ad, "Claude")
		}
		return filepath.Join(home, "AppData", "Roaming", "Claude")
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Claude")
	default:
		return filepath.Join(home, ".config", "Claude")
	}
}

// AgentInfo : état d'un agent IA sur ce poste, pour l'interface (retour #14).
type AgentInfo struct {
	Key           string   `json:"key"`
	Name          string   `json:"name"`
	Dir           string   `json:"dir"`
	Present       bool     `json:"present"`
	SizeBytes     int64    `json:"size_bytes"`
	LastPurgeUnix int64    `json:"last_purge_unix,omitempty"`
	Note          string   `json:"note"`
	Preserve      []string `json:"preserve"`
	// Skipped : entrées qui n'ont PAS pu être retirées (fichier encore ouvert par un process — fréquent
	// sous Windows quand l'agent tourne — ou protégé). La purge n'échoue pas pour autant : on les signale.
	Skipped []string `json:"skipped,omitempty"`
}

// AgentPurgeProgress : avancement d'une purge d'agent, poussé vers l'UI (événement "agentpurge:progress")
// pour un suivi VISUEL entrée par entrée. Phase ∈ "start" (entrée en cours) | "removed" | "skipped" |
// "done" (rapport final). Les compteurs sont CUMULÉS au fil de la purge.
type AgentPurgeProgress struct {
	Key     string `json:"key"`
	Name    string `json:"name"`
	Total   int    `json:"total"`   // entrées à traiter (hors gardées)
	Index   int    `json:"index"`   // entrée courante, 1-based
	Entry   string `json:"entry"`   // nom de l'entrée courante
	IsDir   bool   `json:"is_dir"`  // dossier ?
	Size    int64  `json:"size"`    // poids de l'entrée courante
	Phase   string `json:"phase"`   // start | removed | skipped | done
	Freed   int64  `json:"freed"`   // cumul libéré (octets)
	Removed int    `json:"removed"` // cumul effacé
	Skipped int    `json:"skipped"` // cumul ignoré (verrouillé/protégé)
}

// SetAgentPurgeProgress enregistre le relais d'avancement des purges (l'App le câble sur un événement
// Wails). Posé une fois au démarrage, avant toute purge.
func (s *Service) SetAgentPurgeProgress(fn func(AgentPurgeProgress)) { s.agProg = fn }

func (s *Service) emitAgentPurge(ev AgentPurgeProgress) {
	if s.agProg != nil {
		s.agProg(ev)
	}
}

// AgentClosePurgeKeys : clés des agents que l'utilisateur veut voir PROPOSÉS en purge à la fermeture de
// SituLab (réglage, filtré aux agents connus). Lecture seule.
func (s *Service) AgentClosePurgeKeys() []string {
	cfg, err := config.Load()
	if err != nil {
		return nil
	}
	known := map[string]bool{}
	for _, p := range knownAgents() {
		known[p.key] = true
	}
	out := []string{}
	for _, k := range cfg.ClosePurgeAgents {
		if known[k] {
			out = append(out, k)
		}
	}
	return out
}

// SetAgentClosePurge active/désactive la proposition de purge à la fermeture pour un agent. Geste LOCAL
// (ni PIN ni réseau), bloqué en lecture seule comme toute mutation de réglage. L'ordre de stockage suit
// knownAgents() pour une liste stable.
func (s *Service) SetAgentClosePurge(key string, on bool) error {
	if err := s.assertWritable(); err != nil {
		return err
	}
	known := false
	for _, p := range knownAgents() {
		if p.key == key {
			known = true
			break
		}
	}
	if !known {
		return errors.New("agent inconnu")
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	set := map[string]bool{}
	for _, k := range cfg.ClosePurgeAgents {
		set[k] = true
	}
	if on {
		set[key] = true
	} else {
		delete(set, key)
	}
	cfg.ClosePurgeAgents = nil
	for _, p := range knownAgents() {
		if set[p.key] {
			cfg.ClosePurgeAgents = append(cfg.ClosePurgeAgents, p.key)
		}
	}
	return cfg.Save()
}

// AgentsForClose : les agents configurés pour la purge à la fermeture ET présents sur ce poste (avec
// leur poids), pour la modale intelligente de fermeture. Un agent absent n'est pas proposé.
func (s *Service) AgentsForClose() []AgentInfo {
	want := map[string]bool{}
	for _, k := range s.AgentClosePurgeKeys() {
		want[k] = true
	}
	purges := loadAgentPurges()
	out := []AgentInfo{}
	for _, p := range knownAgents() {
		if !want[p.key] {
			continue
		}
		if info := p.info(purges); info.Present {
			out = append(out, info)
		}
	}
	return out
}

// agentPurgesFile : sidecar des dates de dernière purge (map clé→unix). Volontairement HORS de
// config.json (pas touché par la signature de politique PolicySig) ; vit sous config.Dir(), donc le
// Purge() global de SituLab l'emporte aussi.
func agentPurgesFile() string { return filepath.Join(config.Dir(), "agent_purges.json") }

func loadAgentPurges() map[string]int64 {
	m := map[string]int64{}
	b, err := os.ReadFile(agentPurgesFile())
	if err != nil {
		return m
	}
	_ = json.Unmarshal(b, &m)
	return m
}

func saveAgentPurges(m map[string]int64) {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(config.Dir(), 0o700); err != nil {
		return
	}
	tmp := agentPurgesFile() + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		_ = os.Rename(tmp, agentPurgesFile())
	}
}

// dirSizeBytes somme la taille des fichiers sous root (best-effort : une entrée illisible est ignorée).
func dirSizeBytes(root string) int64 {
	var total int64
	_ = filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		if info, e := d.Info(); e == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}

func (p agentProfile) info(purges map[string]int64) AgentInfo {
	dir := p.dirOf()
	present := false
	var size int64
	if dir != "" {
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			present = true
			size = dirSizeBytes(dir)
		}
	}
	return AgentInfo{
		Key: p.key, Name: p.name, Dir: dir, Present: present, SizeBytes: size,
		LastPurgeUnix: purges[p.key], Note: p.note, Preserve: append([]string(nil), p.preserve...),
	}
}

// Agents liste les agents IA connus détectés sur ce poste, avec leur poids (toujours affiché, retour
// #14) et la date de dernière purge. Lecture seule ; ne touche à rien.
func (s *Service) Agents() []AgentInfo {
	purges := loadAgentPurges()
	profs := knownAgents()
	out := make([]AgentInfo, 0, len(profs))
	for _, p := range profs {
		out = append(out, p.info(purges))
	}
	return out
}

// AgentEntry : une entrée de premier niveau du dossier d'un agent, pour l'aperçu de purge (retour #14).
type AgentEntry struct {
	Name      string `json:"name"`
	IsDir     bool   `json:"is_dir"`
	SizeBytes int64  `json:"size_bytes"`
	Kept      bool   `json:"kept"`
}

// AgentPurgePreview liste les entrées de premier niveau du dossier de l'agent, chacune marquée « gardée »
// (login + fonctionnement) ou « effacée », avec son poids — pour montrer l'arbre avant de confirmer.
func (s *Service) AgentPurgePreview(key string) ([]AgentEntry, error) {
	var prof *agentProfile
	for i := range knownAgents() {
		if p := knownAgents()[i]; p.key == key {
			prof = &p
			break
		}
	}
	if prof == nil {
		return nil, errors.New("agent inconnu")
	}
	dir := prof.dirOf()
	if dir == "" {
		return nil, errors.New("dossier de l'agent indéterminé")
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	keep := map[string]bool{}
	for _, k := range prof.preserve {
		keep[k] = true
	}
	out := make([]AgentEntry, 0, len(ents))
	for _, e := range ents {
		full := filepath.Join(dir, e.Name())
		var size int64
		if e.IsDir() {
			size = dirSizeBytes(full)
		} else if info, ie := e.Info(); ie == nil {
			size = info.Size()
		}
		out = append(out, AgentEntry{Name: e.Name(), IsDir: e.IsDir(), SizeBytes: size, Kept: keep[e.Name()]})
	}
	// Tri : d'abord ce qui sera EFFACÉ (le gros, à comprendre), par poids décroissant ; gardés ensuite.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kept != out[j].Kept {
			return !out[i].Kept
		}
		return out[i].SizeBytes > out[j].SizeBytes
	})
	return out, nil
}

// PurgeAgent efface les données de l'agent `key` SAUF les entrées préservées (login + minimum de
// fonctionnement) : remise à blanc franche. `confirm` doit valoir "EFFACER" (parité avec l'effacement
// SituLab, revérifié côté cœur). Renvoie l'état de l'agent après coup (poids recalculé, date posée).
// Irréversible. Bloqué par la lecture seule (assertWritable). Ni PIN ni réseau.
func (s *Service) PurgeAgent(key, confirm string) (AgentInfo, error) {
	if err := s.assertWritable(); err != nil {
		return AgentInfo{}, err
	}
	if strings.TrimSpace(strings.ToUpper(confirm)) != AgentPurgeConfirm {
		return AgentInfo{}, errors.New("confirmation manquante : tapez PURGER")
	}
	var prof *agentProfile
	for i := range knownAgents() {
		if p := knownAgents()[i]; p.key == key {
			prof = &p
			break
		}
	}
	if prof == nil {
		return AgentInfo{}, errors.New("agent inconnu")
	}
	dir := prof.dirOf()
	// Garde-fous : jamais le home nu ni un chemin vide/racine ; le dossier doit exister et être SOUS le
	// home (on n'efface pas n'importe où, même si la table était mal renseignée).
	home, _ := os.UserHomeDir()
	clean := filepath.Clean(dir)
	if dir == "" || home == "" || clean == filepath.Clean(home) || clean == "." || clean == string(filepath.Separator) {
		return AgentInfo{}, errors.New("dossier de l'agent indéterminé ou non sûr")
	}
	if !strings.HasPrefix(clean+string(filepath.Separator), filepath.Clean(home)+string(filepath.Separator)) {
		return AgentInfo{}, errors.New("dossier de l'agent hors du profil utilisateur : refusé")
	}
	st, err := os.Stat(clean)
	if err != nil || !st.IsDir() {
		return AgentInfo{}, errors.New("rien à effacer : dossier absent")
	}

	keep := map[string]bool{}
	for _, k := range prof.preserve {
		keep[k] = true
	}

	// total (entrées à traiter, hors gardées) pour la barre de progression du suivi visuel.
	total := 0
	if ents, e := os.ReadDir(clean); e == nil {
		for _, en := range ents {
			if !keep[en.Name()] {
				total++
			}
		}
	}

	s.ioMu.Lock()
	// Suivi VISUEL entrée par entrée (événement "agentpurge:progress") : début → effacé | ignoré, avec
	// cumul du poids libéré et des compteurs. Rien d'artificiel : on émet au rythme réel des suppressions.
	var freed int64
	removed, idx, skN := 0, 0, 0
	emit := func(name string, isDir bool, size int64, started, ok bool) {
		if started {
			idx++
			s.emitAgentPurge(AgentPurgeProgress{Key: key, Name: prof.name, Total: total, Index: idx, Entry: name, IsDir: isDir, Size: size, Phase: "start", Freed: freed, Removed: removed, Skipped: skN})
			return
		}
		phase := "removed"
		if ok {
			removed++
			freed += size
		} else {
			phase = "skipped"
			skN++
		}
		s.emitAgentPurge(AgentPurgeProgress{Key: key, Name: prof.name, Total: total, Index: idx, Entry: name, IsDir: isDir, Size: size, Phase: phase, Freed: freed, Removed: removed, Skipped: skN})
	}
	skipped, rmErr := purgeDirKeeping(clean, keep, emit)
	// Date de dernière purge posée même en cas de purge partielle (la purge a bien EU lieu).
	purges := loadAgentPurges()
	purges[key] = time.Now().Unix()
	saveAgentPurges(purges)
	s.ioMu.Unlock()

	// rmErr ne survient que si on n'a même pas pu LIRE le dossier (rien n'a pu commencer) : vrai échec.
	// Un fichier verrouillé (agent ouvert) n'est PAS un échec : il est listé dans `skipped`, et la purge
	// est un succès honnête — l'interface invite à fermer l'agent puis à relancer.
	if rmErr != nil {
		return AgentInfo{}, rmErr
	}
	s.emitAgentPurge(AgentPurgeProgress{Key: key, Name: prof.name, Total: total, Index: total, Phase: "done", Freed: freed, Removed: removed, Skipped: skN})
	info := prof.info(loadAgentPurges())
	info.Skipped = skipped
	return info, nil
}

// purgeDirKeeping efface le CONTENU de dir sauf les entrées (à la racine) dont le nom figure dans keep.
// dir doit avoir été validé par l'appelant (non vide, sous le home, existant). Best-effort : une entrée
// qu'on ne peut pas retirer (fichier encore OUVERT par un process — fréquent sous Windows quand l'agent
// tourne — ou protégée) est IGNORÉE et son nom renvoyé dans `skipped`, sans interrompre le reste.
// L'erreur renvoyée ne concerne QUE l'impossibilité de lire dir (rien n'a pu commencer).
//
// onEntry (peut être nil) est appelé pour CHAQUE entrée traitée : d'abord started=true (avant la
// suppression, avec le poids calculé), puis started=false avec ok = suppression réussie — pour le
// suivi visuel. Le poids n'est calculé que si onEntry != nil (sinon on ne paie pas le parcours).
func purgeDirKeeping(dir string, keep map[string]bool, onEntry func(name string, isDir bool, size int64, started, ok bool)) (skipped []string, err error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, e := range ents {
		if keep[e.Name()] {
			continue
		}
		full := filepath.Join(dir, e.Name())
		var size int64
		if onEntry != nil {
			if e.IsDir() {
				size = dirSizeBytes(full)
			} else if info, ie := e.Info(); ie == nil {
				size = info.Size()
			}
			onEntry(e.Name(), e.IsDir(), size, true, false)
		}
		rmErr := os.RemoveAll(full)
		if rmErr != nil {
			skipped = append(skipped, e.Name())
		}
		if onEntry != nil {
			onEntry(e.Name(), e.IsDir(), size, false, rmErr == nil)
		}
	}
	return skipped, nil
}
