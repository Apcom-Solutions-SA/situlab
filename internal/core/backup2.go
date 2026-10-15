// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"situlab/internal/config"
	"situlab/internal/gitx"
	"situlab/internal/webdav"
)

// Moteur de sauvegarde « tunnel clé-en-main » (v1.14). Un plan décrit QUOI sauvegarder (portée
// dynamique « générique » ou liste figée « ciblée »), SOUS QUELLE FORME (bundle git léger, ou miroir
// de fichiers lisibles), et OÙ (cloud + dossier). Chaque exécution écrit une entrée de JOURNAL.
//
// Compat : les plans v1.10 (mono-dépôt, Type=="") passent toujours par RunBackup (bundle d'un dépôt).

const maxMirrorFiles = 1500 // garde-fou : au-delà, le miroir fichier-par-fichier est refusé (préférer bundle)

type backupTarget struct {
	Path string
	Name string
}

// SaveBackupPlan crée ou met à jour un plan v1.14 (tunnel). Configuration (pas un geste de sécurité).
func (s *Service) SaveBackupPlan(in config.BackupPlan) error {
	if err := s.assertWritable(); err != nil {
		return err
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Cloud = strings.TrimSpace(in.Cloud)
	if in.Name == "" || in.Cloud == "" {
		return errors.New("nom et cloud sont requis")
	}
	if in.Type != "generic" && in.Type != "targeted" {
		return errors.New("type de plan invalide (generic|targeted)")
	}
	if in.Form == "" {
		in.Form = "bundle"
	}
	if in.Layer == "" {
		in.Layer = "local"
	}
	if in.Routine == "" {
		in.Routine = "manual"
	}
	if in.Type == "targeted" {
		clean := in.Repos[:0]
		for _, p := range in.Repos {
			if cp := config.CleanPath(p); cp != "" {
				clean = append(clean, cp)
			}
		}
		in.Repos = clean
		if len(in.Repos) == 0 {
			return errors.New("sélectionnez au moins un dépôt")
		}
	} else {
		in.Repos = nil
	}
	in.Dest = config.CloudCleanPath(in.Dest)
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.FindCloud(in.Cloud) == nil {
		return ErrNoCloud
	}
	if ex := cfg.FindBackup(in.Name); ex != nil {
		in.LastRun = ex.LastRun
		*ex = in
	} else {
		cfg.Backups = append(cfg.Backups, in)
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}

// BackupRuns renvoie le journal des sauvegardes (lecture, pas de PIN).
func (s *Service) BackupRuns() []config.BackupRun {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lockedReload()
	return append([]config.BackupRun(nil), s.cfg.BackupRuns...)
}

// resolveBackupTargets résout la liste des dépôts à sauvegarder pour un plan (dynamique ou figée).
func (s *Service) resolveBackupTargets(plan config.BackupPlan) []backupTarget {
	seen := map[string]bool{}
	var ts []backupTarget
	add := func(p string) {
		cp := config.CleanPath(p)
		if cp != "" && !seen[cp] {
			seen[cp] = true
			ts = append(ts, backupTarget{Path: cp, Name: baseName(cp)})
		}
	}
	if plan.Type == "targeted" {
		for _, p := range plan.Repos {
			add(p)
		}
		return ts
	}
	s.mu.Lock()
	s.lockedReload()
	kernels := append([]config.Kernel(nil), s.cfg.Kernels...)
	spokes := append([]config.Spoke(nil), s.cfg.Spokes...)
	locals := append([]string(nil), s.cfg.LocalRepos...)
	s.mu.Unlock()
	switch plan.Scope {
	case "kernels":
		for _, k := range kernels {
			add(k.LocalPath)
		}
	case "spokes":
		for _, sp := range spokes {
			add(sp.LocalPath)
		}
	case "kernels_spokes":
		for _, k := range kernels {
			add(k.LocalPath)
		}
		for _, sp := range spokes {
			add(sp.LocalPath)
		}
	default: // "repos" | "all" | vide : tous les dépôts locaux suivis
		for _, p := range locals {
			add(p)
		}
	}
	return ts
}

// RunBackupPlan exécute un plan v1.14. trigger ∈ {manual, on_open, on_close}. Renvoie un résumé.
func (s *Service) RunBackupPlan(name, pin, trigger string) (string, error) {
	s.mu.Lock()
	s.lockedReload()
	bp := s.cfg.FindBackup(name)
	s.mu.Unlock()
	if bp == nil {
		return "", ErrNoBackup
	}
	plan := *bp
	if plan.Type == "" { // plan hérité mono-dépôt → ancien chemin
		return s.RunBackup(name, pin)
	}
	if err := s.authorizeCloud(plan.Cloud, config.CloudWrite, pin); err != nil {
		return "", err
	}
	if !gitx.Available() {
		return "", errors.New("git indisponible sur ce poste")
	}
	targets := s.resolveBackupTargets(plan)
	if len(targets) == 0 {
		s.recordBackupRun(name, plan, trigger, 0, 0, "aucun dépôt dans la portée")
		return "", errors.New("aucun dépôt à sauvegarder dans la portée")
	}
	cl, _, err := s.fileCloudClientFor(plan.Cloud)
	if err != nil {
		return "", err
	}
	form := plan.Form
	if form == "" {
		form = "bundle"
	}
	root := "/" + strings.Trim(config.CloudCleanPath(plan.Dest), "/")
	var failed, done int
	firstErr := ""
	for _, t := range targets {
		if st, e := gitx.StatusOf(t.Path); e != nil || !st.IsRepo {
			failed++
			if firstErr == "" {
				firstErr = t.Name + " : pas un dépôt git"
			}
			continue
		}
		dest := path.Join(root, t.Name)
		var e error
		if form == "mirror" {
			e = s.backupMirror(cl, dest, t, plan.Layer)
		} else {
			e = s.backupBundle(cl, dest, t)
		}
		if e != nil {
			failed++
			if firstErr == "" {
				firstErr = t.Name + " : " + e.Error()
			}
		} else {
			done++
		}
	}
	s.recordBackupRun(name, plan, trigger, len(targets), failed, firstErr)
	if done == 0 {
		return "", fmt.Errorf("sauvegarde échouée : %s", firstErr)
	}
	msg := fmt.Sprintf("%d dépôt(s) sauvegardé(s)", done)
	if failed > 0 {
		msg += fmt.Sprintf(", %d échec(s)", failed)
	}
	return msg + " vers " + plan.Cloud, nil
}

// backupBundle : bundle git complet (--all) d'un dépôt → un fichier dans <dest>/.
func (s *Service) backupBundle(cl *webdav.Client, dest string, t backupTarget) error {
	fname := t.Name + "-" + time.Now().Format("20060102-150405") + ".bundle"
	tmp := filepath.Join(os.TempDir(), fname)
	defer os.Remove(tmp)
	if err := gitx.Bundle(t.Path, tmp); err != nil {
		return fmt.Errorf("bundle : %w", err)
	}
	content, err := os.ReadFile(tmp)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), backupUploadTimeout)
	defer cancel()
	s.cloudEnsureDir(ctx, cl, dest)
	return cl.Put(ctx, path.Join(dest, fname), content)
}

// backupMirror : upload des fichiers SUIVIS (lisibles dans le cloud) sous <dest>/[local|atelier]/.
// Garde-fou maxMirrorFiles : au-delà, on refuse (préférer le bundle).
func (s *Service) backupMirror(cl *webdav.Client, dest string, t backupTarget, layer string) error {
	files, err := gitx.TrackedFiles(t.Path)
	if err != nil {
		return fmt.Errorf("liste des fichiers : %w", err)
	}
	if len(files) > maxMirrorFiles {
		return fmt.Errorf("%d fichiers (> %d) : choisissez le format bundle pour ce dépôt", len(files), maxMirrorFiles)
	}
	ctx, cancel := context.WithTimeout(context.Background(), backupUploadTimeout)
	defer cancel()
	// Un seul palier « local » pour le miroir (fichiers du poste) ; « atelier »/« both » retombent sur
	// les mêmes fichiers suivis (le contenu publié = le contenu suivi committé). Simplicité assumée.
	base := dest
	if layer == "atelier" {
		base = path.Join(dest, "atelier")
	}
	s.cloudEnsureDir(ctx, cl, base)
	made := map[string]bool{base: true}
	var firstErr error
	for _, rel := range files {
		local := filepath.Join(t.Path, filepath.FromSlash(rel))
		content, e := os.ReadFile(local)
		if e != nil {
			if firstErr == nil {
				firstErr = e
			}
			continue
		}
		remote := path.Join(base, rel)
		// créer les dossiers parents une fois chacun
		dir := path.Dir(remote)
		if !made[dir] {
			s.cloudEnsureDir(ctx, cl, dir)
			made[dir] = true
		}
		if e := cl.Put(ctx, remote, content); e != nil && firstErr == nil {
			firstErr = e
		}
	}
	return firstErr
}

// cloudEnsureDir crée un dossier cloud et ses parents (Mkcol progressif, « existe déjà » ignoré).
func (s *Service) cloudEnsureDir(ctx context.Context, cl *webdav.Client, dir string) {
	dir = "/" + strings.Trim(dir, "/")
	if dir == "/" {
		return
	}
	segs := strings.Split(strings.Trim(dir, "/"), "/")
	cur := ""
	for _, seg := range segs {
		cur += "/" + seg
		_ = cl.Mkcol(ctx, cur) // best-effort : un dossier déjà présent renvoie une erreur ignorée
	}
}

// recordBackupRun écrit une entrée de journal + horodate le dernier run du plan (best-effort).
func (s *Service) recordBackupRun(name string, plan config.BackupPlan, trigger string, repos, failed int, detail string) {
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return
	}
	now := time.Now().Format(time.RFC3339)
	cfg.AddBackupRun(config.BackupRun{
		Plan: name, When: now, OK: failed == 0 && repos > 0,
		Repos: repos, Failed: failed, Form: plan.Form, Trigger: trigger, Detail: detail,
	})
	if b := cfg.FindBackup(name); b != nil {
		b.LastRun = now
	}
	if cfg.Save() == nil {
		s.publish(cfg)
	}
}

// RunDueBackups exécute les plans dont la routine correspond au déclencheur (on_open / on_close).
// Best-effort : n'exécute que les plans dont le cloud n'exige PAS le PIN (autonomes) — un plan sous
// PIN reste manuel (on ne peut pas ouvrir la fenêtre de façon fiable à l'ouverture/fermeture).
func (s *Service) RunDueBackups(trigger string) {
	s.mu.Lock()
	s.lockedReload()
	plans := append([]config.BackupPlan(nil), s.cfg.Backups...)
	s.mu.Unlock()
	for _, p := range plans {
		if p.Routine != trigger || p.Type == "" {
			continue
		}
		if s.RequiresPINCloud(p.Cloud, config.CloudWrite) {
			continue // sous PIN → reste manuel
		}
		_, _ = s.RunBackupPlan(p.Name, "", trigger)
	}
}
