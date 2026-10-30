// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

// Export / import de la configuration SituLab, chiffré et lié à un PIN (P18, v1.18.2). FENÊTRE
// uniquement (aucun Op socket) : le PIN de sauvegarde transite en argument in-process, jamais par le
// socket. La crypto vit dans internal/keyring (réutilise Argon2id/KEK/DEK/AES-256-GCM) ; ici on
// orchestre le contenu exportable par catégorie et son application à l'import.

package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"situlab/internal/config"
	"situlab/internal/keyring"
)

// Catégories exportables.
const (
	CatSettings = "settings" // réglages (verrou, modules, tags, prompts)
	CatRepos    = "repos"    // projets, dépôts locaux, KERNEL, spokes
	CatGit      = "git"      // connexions git + leurs jetons
	CatCloud    = "cloud"    // connexions cloud + leurs secrets + plans de backup
)

func allCats() []string { return []string{CatSettings, CatRepos, CatGit, CatCloud} }

// normalizeCats : « total »/vide → toutes les catégories ; sinon le sous-ensemble demandé, dédupliqué.
func normalizeCats(cats []string) []string {
	if len(cats) == 0 {
		return allCats()
	}
	set := map[string]bool{}
	for _, c := range cats {
		c = strings.TrimSpace(strings.ToLower(c))
		if c == "total" || c == "all" {
			return allCats()
		}
		set[c] = true
	}
	out := []string{}
	for _, c := range allCats() {
		if set[c] {
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		return allCats()
	}
	return out
}

// buildConfigSubset copie dans un config NEUF les champs des catégories choisies, et renvoie les IDs
// de trousseau à exporter (jetons git + secrets cloud). Aucun secret ne vit dans le config lui-même.
func buildConfigSubset(cur *config.Config, cats []string) (*config.Config, map[string]bool) {
	out := &config.Config{}
	keep := map[string]bool{}
	for _, c := range cats {
		switch c {
		case CatSettings:
			out.LockAfterMin = cur.LockAfterMin
			out.LockAfterWrite = cur.LockAfterWrite
			out.ReadOnly = cur.ReadOnly
			out.Modules = cur.Modules
			out.TagDefs = cur.TagDefs
			out.AgentNotes = cur.AgentNotes
			out.PromptOverrides = cur.PromptOverrides
			out.HiddenPrompts = cur.HiddenPrompts
			out.UserPrompts = cur.UserPrompts
		case CatRepos:
			out.Projects = cur.Projects
			out.LocalRepos = cur.LocalRepos
			out.Kernels = cur.Kernels
			out.ActiveKernel = cur.ActiveKernel
			out.Spokes = cur.Spokes
			out.ArchivedPairings = cur.ArchivedPairings
			out.VitrineHints = cur.VitrineHints
		case CatGit:
			out.Instances = cur.Instances
			out.RevokedSerials = cur.RevokedSerials
			for _, in := range cur.Instances {
				keep[in.Name] = true
			}
		case CatCloud:
			out.FileClouds = cur.FileClouds
			out.Backups = cur.Backups
			out.BackupRuns = cur.BackupRuns
			for _, fc := range cur.FileClouds {
				keep["filecloud/"+fc.Name] = true
			}
		}
	}
	return out, keep
}

// applyConfigSubset applique (REMPLACE par catégorie) les champs importés à la config courante.
func applyConfigSubset(cur, imp *config.Config, cats []string) {
	for _, c := range normalizeCats(cats) {
		switch c {
		case CatSettings:
			cur.LockAfterMin = imp.LockAfterMin
			cur.LockAfterWrite = imp.LockAfterWrite
			cur.ReadOnly = imp.ReadOnly
			cur.Modules = imp.Modules
			cur.TagDefs = imp.TagDefs
			cur.AgentNotes = imp.AgentNotes
			cur.PromptOverrides = imp.PromptOverrides
			cur.HiddenPrompts = imp.HiddenPrompts
			cur.UserPrompts = imp.UserPrompts
		case CatRepos:
			cur.Projects = imp.Projects
			cur.LocalRepos = imp.LocalRepos
			cur.Kernels = imp.Kernels
			cur.ActiveKernel = imp.ActiveKernel
			cur.Spokes = imp.Spokes
			cur.ArchivedPairings = imp.ArchivedPairings
			cur.VitrineHints = imp.VitrineHints
		case CatGit:
			cur.Instances = imp.Instances
			cur.RevokedSerials = imp.RevokedSerials
		case CatCloud:
			cur.FileClouds = imp.FileClouds
			cur.Backups = imp.Backups
			cur.BackupRuns = imp.BackupRuns
		}
	}
}

func configBackupsDir() string { return filepath.Join(config.Dir(), "config-backups") }

// safeBackupName refuse toute traversée de chemin (uniquement un nom de fichier .slbak).
func safeBackupName(name string) (string, error) {
	base := filepath.Base(name)
	if base != name || !strings.HasSuffix(base, ".slbak") || strings.Contains(base, "..") {
		return "", errors.New("nom de sauvegarde invalide")
	}
	return base, nil
}

// ExportConfig produit un fichier .slbak chiffré sous exportPIN, dans le dossier des sauvegardes de
// config. Renvoie le nom du fichier. Nécessite une session déverrouillée (DEK).
func (s *Service) ExportConfig(exportPIN string, categories []string) (string, error) {
	if strings.TrimSpace(exportPIN) == "" {
		return "", errors.New("PIN de sauvegarde requis")
	}
	dek, err := s.sessionDEK()
	if err != nil {
		return "", err
	}
	defer zero(dek)
	cur, err := config.Load()
	if err != nil {
		return "", err
	}
	cats := normalizeCats(categories)
	subset, keepIDs := buildConfigSubset(cur, cats)
	cfgBytes, err := json.Marshal(subset)
	if err != nil {
		return "", err
	}
	kr, err := keyring.BuildExport(dek, exportPIN, keepIDs)
	if err != nil {
		return "", err
	}
	data, err := keyring.MakeBundle(cfgBytes, cats, kr)
	if err != nil {
		return "", err
	}
	dir := configBackupsDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	name := "situlab-" + time.Now().Format("20060102-150405") + ".slbak"
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
		return "", err
	}
	return name, nil
}

// ConfigBackupInfo décrit un fichier de sauvegarde (non sensible).
type ConfigBackupInfo struct {
	Name string `json:"name"`
	When string `json:"when"`
	Size int64  `json:"size"`
}

// ListConfigBackups liste les fichiers .slbak présents (triés du plus récent au plus ancien).
func (s *Service) ListConfigBackups() []ConfigBackupInfo {
	ents, err := os.ReadDir(configBackupsDir())
	if err != nil {
		return nil
	}
	out := []ConfigBackupInfo{}
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".slbak") {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, ConfigBackupInfo{Name: e.Name(), When: fi.ModTime().UTC().Format(time.RFC3339), Size: fi.Size()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].When > out[j].When })
	return out
}

// ReadConfigBackup renvoie le contenu d'un .slbak (pour le télécharger ailleurs).
func (s *Service) ReadConfigBackup(name string) ([]byte, error) {
	base, err := safeBackupName(name)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(filepath.Join(configBackupsDir(), base))
}

// DeleteConfigBackup supprime un fichier de sauvegarde.
func (s *Service) DeleteConfigBackup(name string) error {
	base, err := safeBackupName(name)
	if err != nil {
		return err
	}
	return os.Remove(filepath.Join(configBackupsDir(), base))
}

// InspectConfigBackup lit l'en-tête d'un .slbak (catégories + date) SANS le déchiffrer, pour l'aperçu.
func (s *Service) InspectConfigBackup(name string) (map[string]any, error) {
	data, err := s.ReadConfigBackup(name)
	if err != nil {
		return nil, err
	}
	b, err := keyring.ParseBundle(data)
	if err != nil {
		return nil, err
	}
	return map[string]any{"categories": normalizeCats(b.Categories), "when": b.When}, nil
}

// ImportConfigFile importe un .slbak déjà présent dans le dossier des sauvegardes.
func (s *Service) ImportConfigFile(importPIN, name string) error {
	data, err := s.ReadConfigBackup(name)
	if err != nil {
		return err
	}
	return s.ImportConfig(importPIN, data)
}

// ImportConfig applique un .slbak : installe/fusionne le trousseau sous importPIN, puis applique la
// config par catégorie, et RE-ANCRE la politique (sinon le poste resterait en position fermée).
// Deux chemins : poste VIERGE (importPIN devient le PIN local) ; poste CONFIGURÉ (fusion sous la DEK
// locale, le PIN local ne change pas — session déverrouillée requise).
func (s *Service) ImportConfig(importPIN string, data []byte) error {
	if strings.TrimSpace(importPIN) == "" {
		return errors.New("PIN de sauvegarde requis")
	}
	b, err := keyring.ParseBundle(data)
	if err != nil {
		return err
	}
	if !keyring.Initialized() {
		// Poste vierge : le trousseau de sauvegarde DEVIENT le trousseau local.
		if err := keyring.InstallKeyring(&b.Keyring); err != nil {
			return err
		}
		dek, err := b.Keyring.Unlock(importPIN)
		if err != nil {
			_ = os.Remove(keyring.Path()) // PIN faux : on ne laisse pas un trousseau non ouvrable
			return keyring.ErrBadPIN
		}
		s.openSession(dek)
		zero(dek)
	} else {
		srcDEK, err := b.Keyring.Unlock(importPIN)
		if err != nil {
			return keyring.ErrBadPIN
		}
		defer zero(srcDEK)
		localDEK, err := s.sessionDEK()
		if err != nil {
			return err
		}
		defer zero(localDEK)
		local, err := keyring.Load()
		if err != nil {
			return err
		}
		if err := keyring.MergeInto(local, localDEK, &b.Keyring, srcDEK); err != nil {
			return err
		}
	}
	// Appliquer la config importée (remplacement par catégorie).
	s.ioMu.Lock()
	if len(b.Config) > 0 {
		var imp config.Config
		if err := json.Unmarshal(b.Config, &imp); err != nil {
			s.ioMu.Unlock()
			return fmt.Errorf("config du fichier illisible : %w", err)
		}
		cur, err := config.Load()
		if err != nil {
			s.ioMu.Unlock()
			return err
		}
		applyConfigSubset(cur, &imp, b.Categories)
		if err := cur.Save(); err != nil {
			s.ioMu.Unlock()
			return err
		}
		s.publish(cur)
	}
	s.ioMu.Unlock()
	// Re-signer la politique importée sous la clé d'appareil locale (sinon lecture seule + bandeau).
	s.ReanchorPolicy()
	return nil
}
