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
	"sort"
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
	// Rétention : 0..3 archives (bornes de l'interface) ; sans objet si on conserve tout (cumulatif).
	if in.Archives < 0 {
		in.Archives = 0
	} else if in.Archives > 3 {
		in.Archives = 3
	}
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
func (s *Service) RunBackupPlan(name, trigger string) (string, error) {
	s.mu.Lock()
	s.lockedReload()
	bp := s.cfg.FindBackup(name)
	s.mu.Unlock()
	if bp == nil {
		return "", ErrNoBackup
	}
	plan := *bp
	if plan.Type == "" { // plan hérité mono-dépôt → ancien chemin
		return s.RunBackup(name)
	}
	// Suivi VISUEL permanent : on signale le début, chaque dépôt, puis la fin — quel que soit le chemin
	// de sortie (le defer garantit un dernier événement « terminé » qui efface l'indicateur côté UI).
	s.bkEnter()
	fin := BackupProgress{Plan: name, Trigger: trigger, Phase: "done"}
	defer func() { fin.Active = false; s.emitBackup(fin); s.bkLeave() }()
	s.emitBackup(BackupProgress{Active: true, Plan: name, Trigger: trigger, Phase: "prepare"})
	// Les sauvegardes ne demandent JAMAIS le PIN (routine additive, non destructive) : elles se font
	// seules à l'ouverture/fermeture ou sur action manuelle (besoin utilisateur 1.17).
	if err := s.authorizeBackupCloud(plan.Cloud); err != nil {
		fin.Detail = cleanClientErr(err)
		return "", err
	}
	if !gitx.Available() {
		return "", errors.New("git indisponible sur ce poste")
	}
	targets := s.resolveBackupTargets(plan)
	fin.Total = len(targets)
	if len(targets) == 0 {
		fin.Detail = "aucun dépôt dans la portée"
		s.recordBackupRun(name, plan, trigger, 0, 0, fin.Detail)
		return "", errors.New("aucun dépôt à sauvegarder dans la portée")
	}
	cl, _, err := s.fileCloudClientFor(plan.Cloud)
	if err != nil {
		fin.Detail = cleanClientErr(err)
		return "", err
	}
	// PANNE cloud : on teste la joignabilité AVANT de bundler quoi que ce soit, pour remonter une
	// erreur nette (« cloud injoignable ») plutôt que d'échouer dépôt par dépôt sur chaque upload.
	{
		tctx, tcancel := context.WithTimeout(context.Background(), backupUploadTimeout)
		terr := cl.TestConnection(tctx)
		tcancel()
		if terr != nil {
			detail := "cloud « " + plan.Cloud + " » injoignable : " + terr.Error()
			fin.Detail = detail
			fin.Failed = len(targets)
			s.recordBackupRun(name, plan, trigger, len(targets), len(targets), detail)
			return "", fmt.Errorf("sauvegarde impossible — %s", detail)
		}
	}
	form := plan.Form
	if form == "" {
		form = "bundle"
	}
	root := "/" + strings.Trim(config.CloudCleanPath(plan.Dest), "/")
	var failed, done int
	var okNames, errLines []string
	for i, t := range targets {
		s.emitBackup(BackupProgress{Active: true, Plan: name, Trigger: trigger, Phase: "repo", Repo: t.Name, Index: i + 1, Total: len(targets)})
		if st, e := gitx.StatusOf(t.Path); e != nil || !st.IsRepo {
			failed++
			errLines = append(errLines, t.Name+" : pas un dépôt git")
			continue
		}
		dest := path.Join(root, t.Name)
		var e error
		var rot string
		if form == "mirror" {
			e = s.backupMirror(cl, dest, t, plan.Layer)
		} else {
			rot, e = s.backupBundle(cl, dest, t, plan.Overwrite, plan.Archives)
		}
		if e != nil {
			failed++
			errLines = append(errLines, t.Name+" : "+e.Error())
		} else {
			done++
			if rot != "" { // trace de rotation (archivage / purge) au journal, par dépôt
				okNames = append(okNames, t.Name+" ["+rot+"]")
			} else {
				okNames = append(okNames, t.Name)
			}
		}
	}
	// Détail du journal : où (dossier cloud) + ce qui a réussi + le(s) échec(s) — une trace complète,
	// lisible, de ce qui s'est réellement passé (R : succès ET échecs ET pannes remontent).
	detail := plan.Cloud + ":" + root
	if len(okNames) > 0 {
		detail += " · ✓ " + strings.Join(okNames, ", ")
	}
	if len(errLines) > 0 {
		detail += " · ✕ " + strings.Join(errLines, " ; ")
	}
	s.recordBackupRun(name, plan, trigger, len(targets), failed, detail)
	fin.OK = done > 0
	fin.Failed = failed
	fin.Detail = detail
	if done == 0 {
		return "", fmt.Errorf("sauvegarde échouée — %s", strings.Join(errLines, " ; "))
	}
	msg := fmt.Sprintf("%d dépôt(s) sauvegardé(s)", done)
	if failed > 0 {
		msg += fmt.Sprintf(", %d échec(s)", failed)
	}
	return msg + " → " + plan.Cloud + " : " + root, nil
}

// backupBundle : bundle git complet (--all) d'un dépôt → un fichier horodaté dans <dest>/. Selon la
// rétention du plan, range ensuite les anciens bundles (overwrite + archive/). Renvoie une note de
// rotation pour le journal (ex. « 2 archivée(s), 1 purgée(s) »), vide si cumulatif ou rien à faire.
func (s *Service) backupBundle(cl *webdav.Client, dest string, t backupTarget, overwrite bool, archives int) (string, error) {
	fname := t.Name + "-" + time.Now().Format("20060102-150405") + ".bundle"
	tmp := filepath.Join(os.TempDir(), fname)
	defer os.Remove(tmp)
	if err := gitx.Bundle(t.Path, tmp); err != nil {
		return "", fmt.Errorf("bundle : %w", err)
	}
	content, err := os.ReadFile(tmp)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), backupUploadTimeout)
	defer cancel()
	if err := s.cloudEnsureDir(ctx, cl, dest); err != nil {
		return "", err
	}
	if err := cl.Put(ctx, path.Join(dest, fname), content); err != nil {
		return "", fmt.Errorf("envoi : %w", err)
	}
	// Vérification : on confirme que le fichier est RÉELLEMENT présent sur le cloud après l'envoi.
	// Certains serveurs peuvent répondre 2xx sans persister (quota, règle serveur) : sans ce contrôle,
	// un « succès » mensonger passerait inaperçu — exactement le cas « je lance mais rien ».
	if err := s.verifyUploaded(ctx, cl, dest, fname, int64(len(content))); err != nil {
		return "", err
	}
	if !overwrite {
		return "", nil // cumulatif : on garde tous les bundles horodatés, rien à ranger
	}
	return s.rotateBundles(ctx, cl, dest, t.Name, fname, archives), nil
}

// rotateBundles applique la rétention « écrase » après qu'un nouveau bundle (keepName) a été écrit dans
// dest. Les bundles plus anciens du même dépôt sont retirés de la racine : supprimés si archives==0,
// sinon déplacés dans dest/archive/ dont on ne garde que les `archives` plus récents (les trop anciens
// sont purgés). Best-effort (ne fait jamais échouer la sauvegarde elle-même) ; renvoie une note lisible
// pour le journal.
func (s *Service) rotateBundles(ctx context.Context, cl *webdav.Client, dest, repo, keepName string, archives int) string {
	entries, err := cl.List(ctx, dest)
	if err != nil {
		return ""
	}
	var olds []string
	for _, e := range entries {
		if e.IsDir || e.Name == keepName {
			continue
		}
		if strings.HasPrefix(e.Name, repo+"-") && strings.HasSuffix(e.Name, ".bundle") {
			olds = append(olds, e.Name)
		}
	}
	sort.Strings(olds) // horodatage trié lexicographiquement : du plus ancien au plus récent
	if archives <= 0 {
		del := 0
		for _, nm := range olds {
			if cl.Delete(ctx, path.Join(dest, nm)) == nil {
				del++
			}
		}
		if del > 0 {
			return fmt.Sprintf("écrasé, %d ancien(s) supprimé(s)", del)
		}
		return ""
	}
	arch := path.Join(dest, "archive")
	if err := s.cloudEnsureDir(ctx, cl, arch); err != nil {
		return ""
	}
	moved := 0
	for _, nm := range olds {
		if cl.Move(ctx, path.Join(dest, nm), path.Join(arch, nm)) == nil {
			moved++
		}
	}
	// Purge de archive/ au-delà de `archives` versions.
	purged := 0
	if aents, e := cl.List(ctx, arch); e == nil {
		var archf []string
		for _, ae := range aents {
			if !ae.IsDir && strings.HasPrefix(ae.Name, repo+"-") && strings.HasSuffix(ae.Name, ".bundle") {
				archf = append(archf, ae.Name)
			}
		}
		sort.Strings(archf)
		if len(archf) > archives {
			for _, nm := range archf[:len(archf)-archives] {
				if cl.Delete(ctx, path.Join(arch, nm)) == nil {
					purged++
				}
			}
		}
	}
	parts := []string{}
	if moved > 0 {
		parts = append(parts, fmt.Sprintf("%d archivée(s)", moved))
	}
	if purged > 0 {
		parts = append(parts, fmt.Sprintf("%d purgée(s)", purged))
	}
	return strings.Join(parts, ", ")
}

// verifyUploaded confirme qu'un fichier tout juste envoyé est bien listé sur le cloud (et non vide).
func (s *Service) verifyUploaded(ctx context.Context, cl *webdav.Client, dir, name string, size int64) error {
	entries, err := cl.List(ctx, dir)
	if err != nil {
		return fmt.Errorf("envoi non vérifiable (lecture du dossier cloud échouée) : %w", err)
	}
	for _, e := range entries {
		if e.Name == name {
			if size > 0 && e.Size == 0 {
				return fmt.Errorf("fichier présent mais VIDE sur le cloud après envoi")
			}
			return nil
		}
	}
	return fmt.Errorf("envoi NON confirmé : « %s » absent du cloud après l'upload", name)
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
	if err := s.cloudEnsureDir(ctx, cl, base); err != nil {
		return err
	}
	made := map[string]bool{base: true}
	var firstErr error
	sent := 0
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
			if e := s.cloudEnsureDir(ctx, cl, dir); e != nil {
				if firstErr == nil {
					firstErr = e
				}
				continue
			}
			made[dir] = true
		}
		if e := cl.Put(ctx, remote, content); e != nil {
			if firstErr == nil {
				firstErr = e
			}
			continue
		}
		sent++
	}
	// Rien n'est parti alors qu'il y avait des fichiers → c'est un échec franc, pas un succès silencieux.
	if sent == 0 && len(files) > 0 {
		if firstErr != nil {
			return firstErr
		}
		return fmt.Errorf("aucun fichier n'a pu être envoyé")
	}
	return firstErr
}

// cloudEnsureDir crée un dossier cloud et ses parents (Mkcol progressif). Un dossier déjà présent n'est
// PAS une erreur (Mkcol renvoie nil sur 405) ; en revanche une vraie panne (accès refusé, quota, serveur
// en erreur) remonte désormais au lieu d'être avalée — sinon l'upload échouait ensuite sans explication.
func (s *Service) cloudEnsureDir(ctx context.Context, cl *webdav.Client, dir string) error {
	dir = "/" + strings.Trim(dir, "/")
	if dir == "/" {
		return nil
	}
	segs := strings.Split(strings.Trim(dir, "/"), "/")
	cur := ""
	for _, seg := range segs {
		cur += "/" + seg
		if err := cl.Mkcol(ctx, cur); err != nil {
			return fmt.Errorf("création du dossier cloud « %s » : %w", cur, err)
		}
	}
	return nil
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

// BackupProgress : état d'avancement d'une sauvegarde, poussé vers l'UI pour un suivi VISUEL permanent.
type BackupProgress struct {
	Active  bool   `json:"active"`  // true = une sauvegarde est en cours
	Plan    string `json:"plan"`    // nom du plan
	Trigger string `json:"trigger"` // manual | on_open | on_close
	Phase   string `json:"phase"`   // prepare | repo | done
	Repo    string `json:"repo"`    // dépôt en cours (phase "repo")
	Index   int    `json:"index"`   // dépôt courant (1-based)
	Total   int    `json:"total"`   // nombre total de dépôts
	OK      bool   `json:"ok"`      // (phase "done") succès global
	Failed  int    `json:"failed"`  // (phase "done") nombre d'échecs
	Detail  string `json:"detail"`  // (phase "done") résumé / message
}

// SetBackupProgress enregistre le relais d'avancement (l'App le câble sur un événement Wails). Un seul
// relais ; appelé au démarrage.
func (s *Service) SetBackupProgress(fn func(BackupProgress)) { s.bkProg = fn }

func (s *Service) emitBackup(ev BackupProgress) {
	if s.bkProg != nil {
		s.bkProg(ev)
	}
}

func (s *Service) bkEnter() {
	s.bkMu.Lock()
	s.bkBusy++
	s.bkMu.Unlock()
}
func (s *Service) bkLeave() {
	s.bkMu.Lock()
	if s.bkBusy > 0 {
		s.bkBusy--
	}
	s.bkMu.Unlock()
}

// BackupBusy indique si au moins une sauvegarde est en cours (pour l'UI, au montage).
func (s *Service) BackupBusy() bool {
	s.bkMu.Lock()
	defer s.bkMu.Unlock()
	return s.bkBusy > 0
}

// HasDueBackups dit s'il existe au moins un plan v1.14 dont la routine correspond au déclencheur —
// pour décider, à la fermeture, s'il faut afficher l'écran « sauvegarde en cours » avant de quitter.
func (s *Service) HasDueBackups(trigger string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lockedReload()
	for _, p := range s.cfg.Backups {
		if p.Routine == trigger && p.Type != "" {
			return true
		}
	}
	return false
}

// RunDueBackups exécute les plans dont la routine correspond au déclencheur (on_open / on_close).
// Les sauvegardes ne demandent pas le PIN (routine additive) : TOUS les plans dus s'exécutent, sans
// exception. Best-effort — chaque plan s'inscrit au Journal avec son état (succès / échec / panne).
func (s *Service) RunDueBackups(trigger string) {
	s.mu.Lock()
	s.lockedReload()
	plans := append([]config.BackupPlan(nil), s.cfg.Backups...)
	s.mu.Unlock()
	for _, p := range plans {
		if p.Routine != trigger || p.Type == "" {
			continue
		}
		_, _ = s.RunBackupPlan(p.Name, trigger)
	}
}
