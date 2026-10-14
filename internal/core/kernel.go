// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

// KERNEL (v1.11) : le dépôt-mémoire, présenté pour ce qu'il EST — le centre — et non comme un dépôt
// logiciel parmi d'autres. Ce fichier est une FINE couche au-dessus des gestes git existants (Pull,
// Synchronize) : il résout le dépôt désigné, puis délègue, et traduit l'état git en langage humain.
//
// Doctrine : la DÉSIGNATION d'un KERNEL (ajout/retrait/actif) est un geste de gouvernance réservé à
// la FENÊTRE (AddKernel/RemoveKernel/SetActiveKernel ne sont PAS exposés au socket/CLI/MCP) — un
// agent ne doit jamais pouvoir changer ce qui fait office de mémoire. La lecture (liste, état, carte
// du contenu) et les gestes mémoire (récupérer, enregistrer) restent, eux, sur les trois surfaces.

package core

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"situlab/internal/config"
	"situlab/internal/gitx"
)

// ErrNoKernel : aucun KERNEL ne correspond (ou aucun n'est désigné).
var ErrNoKernel = errors.New("aucun KERNEL désigné")

// kernelRemoteKey renvoie l'identité du DÉPÔT d'un clone (URL distante normalisée), pour qu'un KERNEL
// soit unique PAR DÉPÔT et non par dossier (deux clones du même dépôt = un seul KERNEL). "" si le
// remote est inconnu (git absent, pas un dépôt, pas de distant) — on retombe alors sur le chemin.
func kernelRemoteKey(path string) string {
	if !gitx.Available() {
		return ""
	}
	st, err := gitx.StatusOf(path)
	if err != nil || !st.IsRepo {
		return ""
	}
	return normURL(st.Remote)
}

// KernelInfo : un KERNEL configuré, avec le drapeau « actif ».
type KernelInfo struct {
	config.Kernel
	Active bool `json:"active"`
}

// KernelStatus décrit, en clair, l'état de la mémoire — pour un affichage non technique.
type KernelStatus struct {
	Name        string `json:"name"`
	LocalPath   string `json:"local_path"`
	Instance    string `json:"instance"`
	Branch      string `json:"branch"`
	IsRepo      bool   `json:"is_repo"`
	AheadNotes  int    `json:"ahead_notes"`  // notes enregistrées localement, pas encore envoyées
	BehindNotes int    `json:"behind_notes"` // notes présentes en ligne, pas encore récupérées
	DirtyFiles  int    `json:"dirty_files"`  // modifications en cours non enregistrées
	Clean       bool   `json:"clean"`
	LastNote    string `json:"last_note,omitempty"` // date du dernier enregistrement (HEAD)
	Summary     string `json:"summary"`             // phrase lisible résumant l'état
}

// KernelEntry : une entrée de la « carte du contenu » du KERNEL (langage simple).
type KernelEntry struct {
	Name string `json:"name"`
	Kind string `json:"kind"` // "dossier" | "dépôt" | "fichier"
}

// Kernels renvoie les KERNEL configurés, avec le drapeau actif (relecture disque).
func (s *Service) Kernels() []KernelInfo {
	s.mu.Lock()
	s.lockedReload()
	ks := append([]config.Kernel(nil), s.cfg.Kernels...)
	active := s.cfg.ActiveKernel
	s.mu.Unlock()
	// Déduplication par DÉPÔT (remote, à défaut chemin) : deux clones du même dépôt = un seul KERNEL.
	// La première entrée rencontrée gagne ; l'entrée active est préférée si elle existe.
	out := make([]KernelInfo, 0, len(ks))
	seen := map[string]bool{}
	keyOf := func(k config.Kernel) string {
		if rem := kernelRemoteKey(k.LocalPath); rem != "" {
			return "rem:" + rem
		}
		return "path:" + config.CleanPath(k.LocalPath)
	}
	// Priorité à l'actif pour qu'il reste visible en cas de doublon.
	order := make([]config.Kernel, 0, len(ks))
	for _, k := range ks {
		if config.EqName(k.Name, active) {
			order = append(order, k)
		}
	}
	for _, k := range ks {
		if !config.EqName(k.Name, active) {
			order = append(order, k)
		}
	}
	for _, k := range order {
		key := keyOf(k)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, KernelInfo{Kernel: k, Active: config.EqName(k.Name, active)})
	}
	return out
}

// resolveKernel renvoie le KERNEL désigné par nom (vide = actif), ou une erreur guidée.
func (s *Service) resolveKernel(name string) (config.Kernel, error) {
	s.mu.Lock()
	s.lockedReload()
	k := s.cfg.ResolveKernel(name)
	s.mu.Unlock()
	if k == nil {
		return config.Kernel{}, ErrNoKernel
	}
	return *k, nil
}

// AddKernel désigne un dépôt local comme KERNEL (mémoire). FENÊTRE UNIQUEMENT. Valide que le chemin
// est bien un dépôt git. Le premier KERNEL ajouté devient l'actif.
func (s *Service) AddKernel(name, localPath, instance, backup string) error {
	if err := s.assertWritable(); err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	localPath = strings.TrimSpace(localPath)
	if name == "" || localPath == "" {
		return errors.New("nom et dépôt local sont requis")
	}
	if gitx.Available() {
		if st, err := gitx.StatusOf(localPath); err != nil || !st.IsRepo {
			return fmt.Errorf("« %s » n'est pas un dépôt git", localPath)
		}
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	k := config.Kernel{
		Name:      name,
		LocalPath: config.CleanPath(localPath),
		Instance:  strings.TrimSpace(instance),
		Backup:    strings.TrimSpace(backup),
	}
	// Une désignation par DÉPÔT (chemin) : redésigner le même dépôt (même sous un autre nom) MET À JOUR
	// au lieu de créer un doublon. On retire toute désignation existante du même chemin, en conservant
	// son éventuel plan de sauvegarde si le nouveau n'en précise pas.
	cleanPath := k.LocalPath
	newRem := kernelRemoteKey(k.LocalPath) // identité du DÉPÔT (remote) — un KERNEL par dépôt, pas par dossier
	removedActive := false
	kept := cfg.Kernels[:0]
	for _, ex := range cfg.Kernels {
		sameRepo := config.CleanPath(ex.LocalPath) == cleanPath ||
			(newRem != "" && kernelRemoteKey(ex.LocalPath) == newRem)
		if sameRepo {
			if k.Backup == "" && ex.Backup != "" {
				k.Backup = ex.Backup
			}
			if config.EqName(cfg.ActiveKernel, ex.Name) {
				removedActive = true
			}
			continue // on retire toute désignation existante du même DÉPÔT (autre dossier/nom inclus)
		}
		kept = append(kept, ex)
	}
	cfg.Kernels = append(kept, k)
	if cfg.ActiveKernel == "" || removedActive {
		cfg.ActiveKernel = name
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}

// SetKernelRunFile définit le fichier d'ouverture (pour l'IA locale) d'un KERNEL, désigné par son
// chemin local (robuste à la déduplication par dépôt). runFile = chemin relatif ; "" = aucun.
func (s *Service) SetKernelRunFile(localPath, runFile string) error {
	if err := s.assertWritable(); err != nil {
		return err
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cp := config.CleanPath(localPath)
	found := false
	for i := range cfg.Kernels {
		if config.CleanPath(cfg.Kernels[i].LocalPath) == cp {
			cfg.Kernels[i].RunFile = strings.TrimSpace(runFile)
			found = true
		}
	}
	if !found {
		return ErrNoKernel
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}

// RemoveKernel retire un KERNEL de la liste (ne touche pas au dépôt sur le disque). FENÊTRE UNIQUEMENT.
func (s *Service) RemoveKernel(name string) error {
	if err := s.assertWritable(); err != nil {
		return err
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	// Retrait COMPLET par dépôt : on retire la désignation nommée ET toute autre désignation du MÊME
	// chemin local (nettoie d'éventuels doublons créés par un renommage). Sinon la carte « réapparaîtrait »
	// car un autre nom marque encore ce dépôt comme KERNEL.
	target := cfg.FindKernel(name)
	if target == nil {
		return ErrNoKernel
	}
	cleanPath := config.CleanPath(target.LocalPath)
	targetRem := kernelRemoteKey(target.LocalPath)
	kept := cfg.Kernels[:0]
	for _, ex := range cfg.Kernels {
		sameRepo := config.EqName(ex.Name, name) || config.CleanPath(ex.LocalPath) == cleanPath ||
			(targetRem != "" && kernelRemoteKey(ex.LocalPath) == targetRem)
		if sameRepo {
			if config.EqName(cfg.ActiveKernel, ex.Name) {
				cfg.ActiveKernel = ""
			}
			continue
		}
		kept = append(kept, ex)
	}
	cfg.Kernels = kept
	if cfg.ActiveKernel == "" && len(cfg.Kernels) > 0 {
		cfg.ActiveKernel = cfg.Kernels[0].Name
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}

// SetActiveKernel désigne le KERNEL courant. FENÊTRE UNIQUEMENT.
func (s *Service) SetActiveKernel(name string) error {
	if err := s.assertWritable(); err != nil {
		return err
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if !cfg.SetActiveKernel(name) {
		return ErrNoKernel
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}

// KernelStatus lit l'état local du KERNEL (sans réseau) et le traduit en langage humain.
func (s *Service) KernelStatus(name string) (*KernelStatus, error) {
	k, err := s.resolveKernel(name)
	if err != nil {
		return nil, err
	}
	return s.repoMemStatus(k.Name, k.LocalPath, k.Instance, "Mémoire à jour.")
}

// repoMemStatus calcule l'état lisible d'un dépôt-mémoire (KERNEL ou SPOKE). upToDate = phrase quand
// tout est propre. Lecture locale, sans réseau.
func (s *Service) repoMemStatus(name, localPath, instance, upToDate string) (*KernelStatus, error) {
	if !gitx.Available() {
		return nil, ErrNoGit
	}
	st, err := gitx.StatusOf(localPath)
	if err != nil {
		return nil, err
	}
	ks := &KernelStatus{
		Name:        name,
		LocalPath:   localPath,
		Instance:    instance,
		Branch:      st.Branch,
		IsRepo:      st.IsRepo,
		AheadNotes:  st.Ahead,
		BehindNotes: st.Behind,
		DirtyFiles:  len(st.Changed),
		Clean:       st.Clean,
	}
	if !st.IsRepo {
		ks.Summary = "Ce dossier n'est plus un dépôt git."
		return ks, nil
	}
	if commits := gitx.Log(localPath, 1); len(commits) > 0 {
		ks.LastNote = commits[0].Date
	}
	ks.Summary = memSummary(ks, upToDate)
	return ks, nil
}

// memSummary compose une phrase lisible d'après l'état (ordre : modifs > avance > retard).
func memSummary(ks *KernelStatus, upToDate string) string {
	parts := []string{}
	if ks.DirtyFiles > 0 {
		parts = append(parts, fmt.Sprintf("%d modification(s) en cours à enregistrer", ks.DirtyFiles))
	}
	if ks.AheadNotes > 0 {
		parts = append(parts, fmt.Sprintf("%d note(s) enregistrée(s) à envoyer", ks.AheadNotes))
	}
	if ks.BehindNotes > 0 {
		parts = append(parts, fmt.Sprintf("%d note(s) à récupérer", ks.BehindNotes))
	}
	if len(parts) == 0 {
		return upToDate
	}
	return strings.Join(parts, " · ")
}

// KernelContents dresse la « carte du contenu » du KERNEL : les grands dossiers/dépôts qu'il
// englobe, en libellés simples. Lecture disque, sans réseau. Les entrées cachées (.*) sont ignorées.
func (s *Service) KernelContents(name string) ([]KernelEntry, error) {
	k, err := s.resolveKernel(name)
	if err != nil {
		return nil, err
	}
	return repoMemContents(k.LocalPath)
}

// repoMemContents dresse la carte du contenu d'un dépôt-mémoire (partagé KERNEL/SPOKE).
func repoMemContents(localPath string) ([]KernelEntry, error) {
	ents, err := os.ReadDir(localPath)
	if err != nil {
		return nil, err
	}
	out := []KernelEntry{}
	for _, e := range ents {
		nm := e.Name()
		if strings.HasPrefix(nm, ".") {
			continue // .git et consorts : hors de la carte humaine
		}
		if e.IsDir() {
			kind := "dossier"
			if _, serr := os.Stat(filepath.Join(localPath, nm, ".git")); serr == nil {
				kind = "dépôt"
			}
			out = append(out, KernelEntry{Name: nm, Kind: kind})
		} else {
			out = append(out, KernelEntry{Name: nm, Kind: "fichier"})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind // "dossier" < "dépôt" < "fichier"
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// KernelPull récupère les dernières notes du KERNEL (fast-forward). Lecture : pas de PIN.
func (s *Service) KernelPull(name string) (string, error) {
	k, err := s.resolveKernel(name)
	if err != nil {
		return "", err
	}
	return s.Pull(k.Instance, k.LocalPath)
}

// KernelSave est le geste « enregistrer mes notes » : récupérer → enregistrer → envoyer, sous une
// seule confirmation PIN. Délègue à Synchronize (même portillon que le push). Message vide => libellé
// daté côté Synchronize.
func (s *Service) KernelSave(name, message, pin string) (*SyncResult, error) {
	k, err := s.resolveKernel(name)
	if err != nil {
		return nil, err
	}
	return s.Synchronize(k.Instance, k.LocalPath, message, pin)
}
