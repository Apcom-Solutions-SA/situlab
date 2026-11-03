// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"encoding/json"
	"os"
	"path/filepath"

	"situlab/internal/config"
)

// Modules d'interface (v1.13) : groupes de fonctions activables/désactivables selon le profil
// (utilisateur « porté » vs développeur). Réglage LOCAL, FENÊTRE/CLI (non sensible). Un groupe absent
// de la map = ACTIVÉ par défaut : on ne persiste que ce qui a été coupé/rallumé explicitement.

// ModuleKeys : la liste canonique des groupes de modules connus (ordre d'affichage).
var ModuleKeys = []string{"agents", "git_advanced", "spokes", "corpus", "backup"}

// Modules renvoie l'état de CHAQUE groupe connu (résolu : absent = activé).
func (s *Service) Modules() map[string]bool {
	cfg, _ := config.Load()
	out := make(map[string]bool, len(ModuleKeys))
	for _, k := range ModuleKeys {
		if cfg == nil {
			out[k] = true
		} else {
			out[k] = cfg.ModuleOn(k)
		}
	}
	return out
}

// SetModule active/désactive un groupe de modules (persisté en config).
func (s *Service) SetModule(key string, on bool) error {
	if !knownModule(key) {
		return nil // inconnu : on ignore silencieusement (robustesse)
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.Modules == nil {
		cfg.Modules = map[string]bool{}
	}
	cfg.Modules[key] = on
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}

// SetModules applique un lot d'états (ex. un préréglage) en une fois.
func (s *Service) SetModules(mods map[string]bool) error {
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.Modules == nil {
		cfg.Modules = map[string]bool{}
	}
	for k, v := range mods {
		if knownModule(k) {
			cfg.Modules[k] = v
		}
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}

// ImportModulesSeed importe, au PREMIER lancement, les primo-paramètres de modules déposés par
// l'installateur à côté de l'exécutable (situlab-modules.json). N'écrase rien si l'utilisateur a déjà
// réglé ses modules. Le fichier seed est supprimé après lecture (consommé). Best-effort, silencieux.
func (s *Service) ImportModulesSeed() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	seed := filepath.Join(filepath.Dir(exe), "situlab-modules.json")
	raw, err := os.ReadFile(seed)
	if err != nil {
		return // pas de seed = rien à faire
	}
	var mods map[string]bool
	if json.Unmarshal(raw, &mods) != nil || len(mods) == 0 {
		_ = os.Remove(seed)
		return
	}
	s.ioMu.Lock()
	cfg, err := config.Load()
	if err == nil && len(cfg.Modules) == 0 { // uniquement si l'utilisateur n'a encore rien choisi
		cfg.Modules = map[string]bool{}
		for k, v := range mods {
			if knownModule(k) {
				cfg.Modules[k] = v
			}
		}
		_ = cfg.Save()
	}
	s.ioMu.Unlock()
	_ = os.Remove(seed) // consommé, une seule fois
	if c, e := config.Load(); e == nil {
		s.publish(c)
	}
}

func knownModule(key string) bool {
	for _, k := range ModuleKeys {
		if k == key {
			return true
		}
	}
	return false
}
