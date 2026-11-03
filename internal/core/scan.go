// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

// Découverte des dépôts git déjà présents sur la machine (v1.11) — confort de prise en main : plutôt
// que d'exiger un chemin tapé à la main pour désigner un KERNEL ou un spoke, SituLab PROPOSE une
// liste. On explore des racines usuelles (dossier personnel, Bureau, Documents, C:\tmp…) à profondeur
// bornée, en ignorant le bruit (node_modules, dossiers système), et on marque ce qui est déjà suivi
// ou déjà désigné. Lecture PURE : rien n'est modifié, git n'est pas invoqué (le distant est lu
// directement dans .git/config) — une recherche de confort ne doit ni écrire ni ralentir le poste.

package core

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"situlab/internal/config"
)

// errRepoFilesCap : sentinelle interne pour arrêter le parcours une fois le plafond atteint.
var errRepoFilesCap = errors.New("plafond de fichiers atteint")

// repoFilesMax borne le nombre de fichiers listés (évite d'arpenter un dépôt géant côté fenêtre).
const repoFilesMax = 8000

// RepoFiles renvoie la liste des fichiers d'un dépôt local (chemins RELATIFS, séparateur « / »), triés.
// Ignore .git et les dossiers de bruit (node_modules, build…), saute les dossiers cachés/système.
// Sert à choisir le « fichier d'ouverture » d'un KERNEL / spoke dans un arbre filtrable.
func (s *Service) RepoFiles(localPath string) ([]string, error) {
	root := strings.TrimSpace(localPath)
	if root == "" {
		return nil, errors.New("chemin vide")
	}
	if fi, err := os.Stat(root); err != nil || !fi.IsDir() {
		return nil, errors.New("dossier introuvable")
	}
	var out []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // on ignore les entrées illisibles plutôt que d'échouer tout le parcours
		}
		name := d.Name()
		if d.IsDir() {
			if p == root {
				return nil
			}
			if scanSkip[name] || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "$") {
				return filepath.SkipDir
			}
			return nil
		}
		rel, e := filepath.Rel(root, p)
		if e != nil {
			return nil
		}
		out = append(out, filepath.ToSlash(rel))
		if len(out) >= repoFilesMax {
			return errRepoFilesCap
		}
		return nil
	})
	if err != nil && !errors.Is(err, errRepoFilesCap) {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

// FoundRepo : un dépôt git détecté localement, prêt à être proposé à la désignation.
type FoundRepo struct {
	Path   string `json:"path"`
	Name   string `json:"name"`
	Remote string `json:"remote"`         // URL du distant « origin » si présent (lu dans .git/config)
	Known  bool   `json:"known"`          // déjà suivi par SituLab (dépôt local connu)
	Role   string `json:"role,omitempty"` // "kernel" | "spoke" | "" — déjà désigné ?
}

// scanSkip : dossiers à ne jamais traverser (bruit de dépendances, système, lourds).
var scanSkip = map[string]bool{
	"node_modules": true, "vendor": true, ".git": true, "AppData": true,
	"Application Data": true, "Windows": true, "Program Files": true,
	"Program Files (x86)": true, "ProgramData": true, "$Recycle.Bin": true,
	".cache": true, ".vscode": true, ".idea": true, "Library": true,
	"target": true, "dist": true, "build": true, "__pycache__": true,
}

// scanMaxDepth borne la descente depuis chaque racine (évite d'arpenter des arbres sans fin).
const scanMaxDepth = 5

// ScanGitRepos recherche les dépôts git du poste pour proposer une LISTE à la désignation d'un
// KERNEL ou d'un spoke. Combine les dépôts déjà suivis (toujours proposés, même hors racines) et
// ceux trouvés par exploration. Classe d'abord les connus/désignés, puis par nom.
func (s *Service) ScanGitRepos() ([]FoundRepo, error) {
	s.mu.Lock()
	s.lockedReload()
	known := map[string]bool{}
	for _, p := range s.cfg.LocalRepos {
		known[config.CleanPath(p)] = true
	}
	role := map[string]string{}
	for _, k := range s.cfg.Kernels {
		role[config.CleanPath(k.LocalPath)] = "kernel"
	}
	for _, sp := range s.cfg.Spokes {
		if role[config.CleanPath(sp.LocalPath)] == "" {
			role[config.CleanPath(sp.LocalPath)] = "spoke"
		}
	}
	s.mu.Unlock()

	seen := map[string]bool{}
	var out []FoundRepo
	add := func(dir string) {
		clean := config.CleanPath(dir)
		if seen[clean] {
			return
		}
		seen[clean] = true
		out = append(out, FoundRepo{
			Path:   clean,
			Name:   filepath.Base(clean),
			Remote: readGitRemote(clean),
			Known:  known[clean],
			Role:   role[clean],
		})
	}

	// Les dépôts déjà suivis dont le dossier existe encore : toujours proposés, même hors racines.
	for p := range known {
		if _, err := os.Stat(filepath.Join(p, ".git")); err == nil {
			add(p)
		}
	}
	for _, root := range scanRoots() {
		walkGitRoots(root, 0, add)
	}

	sort.Slice(out, func(i, j int) bool {
		ai := out[i].Known || out[i].Role != ""
		aj := out[j].Known || out[j].Role != ""
		if ai != aj {
			return ai // connus / déjà désignés en tête
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// ScanFolderRepos découvre les dépôts git sous UN dossier choisi par l'utilisateur (mise en route :
// « scanner un dossier » puis suivre en lot). Même exploration bornée que ScanGitRepos, mais sur la
// seule racine fournie. Marque Known (déjà suivi) et Role (déjà désigné). Lecture pure : rien n'est
// modifié — le suivi reste un geste explicite (AddLocalRepo). Les non suivis sont classés en tête.
func (s *Service) ScanFolderRepos(root string) ([]FoundRepo, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("dossier à scanner requis")
	}
	if fi, err := os.Stat(root); err != nil || !fi.IsDir() {
		return nil, errors.New("dossier introuvable")
	}
	s.mu.Lock()
	s.lockedReload()
	known := map[string]bool{}
	for _, p := range s.cfg.LocalRepos {
		known[config.CleanPath(p)] = true
	}
	role := map[string]string{}
	for _, k := range s.cfg.Kernels {
		role[config.CleanPath(k.LocalPath)] = "kernel"
	}
	for _, sp := range s.cfg.Spokes {
		if role[config.CleanPath(sp.LocalPath)] == "" {
			role[config.CleanPath(sp.LocalPath)] = "spoke"
		}
	}
	s.mu.Unlock()

	seen := map[string]bool{}
	var out []FoundRepo
	add := func(dir string) {
		clean := config.CleanPath(dir)
		if seen[clean] {
			return
		}
		seen[clean] = true
		out = append(out, FoundRepo{
			Path:   clean,
			Name:   filepath.Base(clean),
			Remote: readGitRemote(clean),
			Known:  known[clean],
			Role:   role[clean],
		})
	}
	walkGitRoots(config.CleanPath(root), 0, add)

	sort.Slice(out, func(i, j int) bool {
		if out[i].Known != out[j].Known {
			return !out[i].Known // non suivis d'abord : ce sont eux qu'on propose d'ajouter
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// scanRoots : les racines usuelles à explorer. Le dossier personnel couvre Bureau/Documents ; on y
// ajoute des racines de développement courantes et le répertoire de travail, si elles existent.
func scanRoots() []string {
	var roots []string
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		roots = append(roots, home)
	}
	for _, r := range []string{
		`C:\tmp`, `C:\dev`, `C:\src`, `C:\code`, `C:\Projects`, `C:\work`, `C:\git`,
		"/c/tmp", "/tmp",
	} {
		if fi, err := os.Stat(r); err == nil && fi.IsDir() {
			roots = append(roots, r)
		}
	}
	if wd, err := os.Getwd(); err == nil && wd != "" {
		roots = append(roots, wd)
	}
	return roots
}

// walkGitRoots descend dans dir (profondeur bornée). Dès qu'un dossier EST un dépôt (présence de
// .git), on le retient et on NE descend PAS dedans (les dépôts imbriqués sont rares ; cela borne le
// coût). Sinon on explore ses sous-dossiers en sautant le bruit et les dossiers cachés/système.
func walkGitRoots(dir string, depth int, add func(string)) {
	if depth > scanMaxDepth {
		return
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		add(dir)
		return
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if scanSkip[name] || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "$") {
			continue
		}
		walkGitRoots(filepath.Join(dir, name), depth+1, add)
	}
}

// readGitRemote lit l'URL du distant « origin » (ou, à défaut, du premier distant) directement dans
// .git/config — sans invoquer git. Renvoie "" si le dépôt n'a pas de distant, ou si .git est un
// fichier (worktree/sous-module) plutôt qu'un dossier.
func readGitRemote(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, ".git", "config"))
	if err != nil {
		return ""
	}
	var first, origin string
	inRemote, isOrigin := false, false
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			inRemote = strings.HasPrefix(line, "[remote ")
			isOrigin = inRemote && strings.Contains(line, `"origin"`)
			continue
		}
		if inRemote && strings.HasPrefix(line, "url") {
			if i := strings.Index(line, "="); i >= 0 {
				u := strings.TrimSpace(line[i+1:])
				if isOrigin {
					origin = u
				} else if first == "" {
					first = u
				}
			}
		}
	}
	if origin != "" {
		return origin
	}
	return first
}
