// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"path/filepath"
	"situlab/internal/config"
	"strings"
)

// AgentLocalPathAllowed borne le chemin LOCAL d'un transfert cloud INITIE PAR UN AGENT (CORPUS
// upload/download). Fail-closed : seuls les fichiers situes SOUS un depot suivi (clone local) sont
// permis, et JAMAIS un chemin du dossier de configuration de SituLab (secrets : device.key, keyring,
// daemon.token, config.json). Empeche un agent injecte d'exfiltrer les clefs ou d'ecrire des octets
// arbitraires hors de l'espace de travail. La FENETRE (humain) n'est PAS soumise a cette borne.
func (s *Service) AgentLocalPathAllowed(local string) bool {
	if strings.TrimSpace(local) == "" {
		return false
	}
	abs, err := filepath.Abs(local)
	if err != nil {
		return false
	}
	// Resoudre les liens symboliques (du fichier s'il existe, sinon du parent) pour bloquer une evasion.
	if r, e := filepath.EvalSymlinks(abs); e == nil {
		abs = r
	} else if r, e := filepath.EvalSymlinks(filepath.Dir(abs)); e == nil {
		abs = filepath.Join(r, filepath.Base(abs))
	}
	abs = filepath.Clean(abs)
	// Jamais le dossier de configuration (secrets souverains).
	if cfgDir, e := filepath.Abs(config.Dir()); e == nil {
		if cr, e2 := filepath.EvalSymlinks(cfgDir); e2 == nil {
			cfgDir = cr
		}
		if pathUnder(abs, filepath.Clean(cfgDir)) {
			return false
		}
	}
	s.mu.Lock()
	s.lockedReload()
	roots := append([]string(nil), s.cfg.LocalRepos...)
	s.mu.Unlock()
	for _, root := range roots {
		ra, e := filepath.Abs(root)
		if e != nil {
			continue
		}
		if re, e2 := filepath.EvalSymlinks(ra); e2 == nil {
			ra = re
		}
		if pathUnder(abs, filepath.Clean(ra)) {
			return true
		}
	}
	return false
}

// SetCloudAllowedDirs fixe la liste des dossiers que l'IA peut consulter sur cette connexion.
func (s *Service) SetCloudAllowedDirs(name string, dirs []string) error {
	if err := s.assertWritable(); err != nil {
		return err
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	fc := cfg.FindCloud(name)
	if fc == nil {
		return ErrNoCloud
	}
	seen := map[string]bool{}
	out := []string{}
	for _, d := range dirs {
		nd := config.CloudCleanPath(d)
		if nd == "" || seen[nd] {
			continue
		}
		seen[nd] = true
		out = append(out, nd)
	}
	fc.AllowedDirs = out
	s.resignPolicy(cfg) // re-signe la policy (fermeture P3)
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}

// SetCloudAccess fixe EN UN GESTE les autorisations (allow) et exclusions (deny) de l'IA pour une
// connexion — le modèle « arbre » de la fenêtre (v1.11). Les deux listes sont normalisées/dédupliquées.
// Réglage FENÊTRE UNIQUEMENT (non exposé socket/CLI/MCP).
func (s *Service) SetCloudAccess(name string, allowed, denied []string) error {
	if err := s.assertWritable(); err != nil {
		return err
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	fc := cfg.FindCloud(name)
	if fc == nil {
		return ErrNoCloud
	}
	fc.AllowedDirs = cleanCloudPaths(allowed)
	fc.DeniedDirs = cleanCloudPaths(denied)
	s.resignPolicy(cfg) // re-signe la policy (fermeture P3)
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}

// cleanCloudPaths normalise et déduplique une liste de chemins cloud (vides ignorés).
func cleanCloudPaths(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, d := range in {
		nd := config.CloudCleanPath(d)
		if nd == "" || seen[nd] {
			continue
		}
		seen[nd] = true
		out = append(out, nd)
	}
	return out
}

// SetCloudPINPolicy fixe la politique PIN d'une connexion cloud (nil = défauts).
func (s *Service) SetCloudPINPolicy(name string, pol *config.CloudPINPolicy) error {
	if err := s.assertWritable(); err != nil {
		return err
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	fc := cfg.FindCloud(name)
	if fc == nil {
		return ErrNoCloud
	}
	fc.PINPolicy = pol
	s.resignPolicy(cfg) // re-signe la policy (fermeture P3)
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}

// --- Autorisation ---------------------------------------------------------------------------

// RequiresPINCloud expose la politique PIN d'une connexion cloud. Fail-closed : cloud inconnu → PIN.
func (s *Service) RequiresPINCloud(name string, a config.CloudAction) bool {
	s.mu.Lock()
	s.lockedReload()
	cfg := s.cfg
	fc := cfg.ResolveCloud(name)
	ok := s.policyOK(cfg)
	s.mu.Unlock()
	if fc == nil || !ok {
		return true // fail-closed : cloud inconnu OU politique falsifiée → PIN exigé (fermeture P3)
	}
	return fc.RequiresPINCloud(a)
}

// authorizeCloud autorise une mutation cloud sur un CHEMIN précis (v1.18.2 : path-aware + tri-état).
// Ordre : lecture seule → PIN (si interdiction OU PIN sur ce chemin) → session (si sans PIN).
// L'« interdiction » est traitée comme « PIN » côté cœur : l'humain en fenêtre peut toujours agir
// avec son PIN (D4). Le refus pur-et-dur de l'interdiction vit à la frontière agent (démon gateCloud,
// via CloudOpForbidden), AVANT tout prompt — l'agent ne peut même pas tenter.
func (s *Service) authorizeCloud(name, p string, a config.CloudAction, pin string) error {
	if s.ReadOnly() {
		return ErrReadOnly
	}
	fc, ok := s.resolveCloud(name)
	if !ok {
		return s.gatePIN(pin) // fail-closed : cloud inconnu / politique falsifiée → PIN (ré-validation possible en fenêtre)
	}
	switch fc.EffectiveCloudLevel(p, a) {
	case config.CloudLevelPIN, config.CloudLevelForbid:
		return s.gatePIN(pin)
	default: // none
		if s.Unlocked() {
			return nil
		}
		return ErrLocked
	}
}

// CloudOpForbidden : l'opération est-elle formellement interdite à l'IA sur ce chemin ? Pour la
// frontière agent (démon). Fail-closed : cloud inconnu / politique falsifiée → interdit.
func (s *Service) CloudOpForbidden(name, p string, a config.CloudAction) bool {
	fc, ok := s.resolveCloud(name)
	if !ok {
		return true
	}
	return fc.CloudOpForbiddenAt(p, a)
}

// CloudRequiresPINAt : l'opération exige-t-elle le PIN sur ce chemin ? Fail-closed → PIN.
func (s *Service) CloudRequiresPINAt(name, p string, a config.CloudAction) bool {
	fc, ok := s.resolveCloud(name)
	if !ok {
		return true
	}
	return fc.RequiresPINCloudAt(p, a)
}

// CloudEffectivePolicy renvoie les niveaux résolus (write/delete/rights) pour un chemin — lecture,
// pour exposer à l'agent (CLI/MCP) et à la GUI « tes limites ». Fail-closed → interdiction partout.
func (s *Service) CloudEffectivePolicy(name, p string) (w, d, r config.CloudOpLevel) {
	fc, ok := s.resolveCloud(name)
	if !ok {
		return config.CloudLevelForbid, config.CloudLevelForbid, config.CloudLevelForbid
	}
	return fc.EffectiveCloudLevel(p, config.CloudWrite), fc.EffectiveCloudLevel(p, config.CloudDelete), fc.EffectiveCloudLevel(p, config.CloudRights)
}

// authorizeBackupCloud : autorisation DÉDIÉE aux sauvegardes. Un backup est une routine NON destructive
// (copie additive vers le cloud de l'utilisateur) qui doit « se faire toute seule » à l'ouverture, à la
// fermeture ou sur action manuelle — il ne demande JAMAIS le PIN, quelle que soit la politique du cloud.
// Il requiert seulement une session déverrouillée (nécessaire pour déchiffrer le jeton du cloud) et
// respecte le mode lecture seule (qui gèle tout envoi de façon délibérée).
func (s *Service) authorizeBackupCloud(name string) error {
	if s.ReadOnly() {
		return ErrReadOnly
	}
	if !s.Unlocked() {
		return ErrLocked
	}
	return nil
}

// CloudPathAllowed dit si un chemin est dans le périmètre autorisé à l'IA. Appelé à la frontière
// agent (démon). Fail-closed : cloud inconnu ou allowlist vide → refus.
func (s *Service) CloudPathAllowed(name, p string) bool {
	s.mu.Lock()
	s.lockedReload()
	cfg := s.cfg
	fc := cfg.ResolveCloud(name)
	ok := s.policyOK(cfg)
	s.mu.Unlock()
	if fc == nil || !ok {
		return false // fail-closed : cloud inconnu OU politique falsifiée → périmètre fermé (fermeture P3)
	}
	return fc.PathAllowed(p)
}

// CloudAllowedDirs renvoie les dossiers autorisés à l'IA pour une connexion (non secret). Permet à
// l'agent de connaître ses points d'entrée avant d'explorer.
func (s *Service) CloudAllowedDirs(name string) []string {
	s.mu.Lock()
	s.lockedReload()
	cfg := s.cfg
	fc := cfg.ResolveCloud(name)
	ok := s.policyOK(cfg)
	s.mu.Unlock()
	if fc == nil || !ok {
		return nil // fail-closed : politique falsifiée → aucun point d'entrée annoncé (fermeture P3)
	}
	return append([]string(nil), fc.AllowedDirs...)
}
