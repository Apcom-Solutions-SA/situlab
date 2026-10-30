// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package gitx

import (
	"strconv"
	"strings"
)

// TreeEntry : une entrée de l'arbre d'un clone local (depuis HEAD), pour l'explorateur.
type TreeEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`           // chemin complet depuis la racine du dépôt
	Type string `json:"type"`           // "dir" | "file"
	Size int64  `json:"size"`           // octets (0 pour un dossier)
	Date string `json:"date,omitempty"` // ISO8601 du dernier commit (rempli à part, best-effort)
}

// LocalTreeAt liste le contenu DIRECT d'un sous-dossier du clone local, depuis HEAD, HORS-LIGNE et en
// un seul appel git : sert à peindre l'explorateur INSTANTANÉMENT avant toute réconciliation avec le
// dépôt en ligne. subpath "" = racine. Les dates sont remplies séparément (LocalTreeDates) pour ne
// pas retarder le premier affichage.
func LocalTreeAt(dir, subpath string) ([]TreeEntry, error) {
	sp := strings.Trim(strings.ReplaceAll(subpath, "\\", "/"), "/")
	args := []string{"ls-tree", "-l", "-z", "HEAD"}
	if sp != "" {
		args = append(args, sp+"/") // contenu direct du sous-dossier
	}
	out, err := run(dir, LocalTimeout, args...)
	if err != nil {
		return nil, err
	}
	var entries []TreeEntry
	for _, rec := range strings.Split(out, "\x00") {
		if rec == "" {
			continue
		}
		// Format ls-tree -l : "<mode> <type> <sha> <size>\t<path>"
		tab := strings.IndexByte(rec, '\t')
		if tab < 0 {
			continue
		}
		meta := strings.Fields(rec[:tab])
		path := rec[tab+1:]
		if len(meta) < 4 || path == "" {
			continue
		}
		typ := "file"
		if meta[1] == "tree" {
			typ = "dir"
		}
		var size int64
		if typ == "file" {
			size, _ = strconv.ParseInt(meta[3], 10, 64)
		}
		name := path
		if i := strings.LastIndexByte(path, '/'); i >= 0 {
			name = path[i+1:]
		}
		entries = append(entries, TreeEntry{Name: name, Path: path, Type: typ, Size: size})
	}
	return entries, nil
}

// TreeTotals : tailles agrégées par dossier enfant direct + total du sous-arbre (octets).
type TreeTotals struct {
	Sizes map[string]int64 `json:"sizes"`
	Total int64            `json:"total"`
}

// LocalTreeTotals calcule, EN UN SEUL appel git (ls-tree récursif), la taille agrégée de chaque
// dossier enfant direct de subpath et le total du sous-arbre. À la racine (subpath ""), Total est le
// poids total du dépôt. Hors-ligne. Sert à afficher la taille des dossiers + le total dans l'explorateur.
func LocalTreeTotals(dir, subpath string) (TreeTotals, error) {
	sp := strings.Trim(strings.ReplaceAll(subpath, "\\", "/"), "/")
	args := []string{"ls-tree", "-l", "-r", "-z", "HEAD"}
	if sp != "" {
		args = append(args, sp+"/")
	}
	out, err := run(dir, LocalTimeout, args...)
	res := TreeTotals{Sizes: map[string]int64{}}
	if err != nil {
		return res, err
	}
	for _, rec := range strings.Split(out, "\x00") {
		if rec == "" {
			continue
		}
		tab := strings.IndexByte(rec, '\t')
		if tab < 0 {
			continue
		}
		meta := strings.Fields(rec[:tab])
		path := rec[tab+1:]
		if len(meta) < 4 || meta[1] != "blob" {
			continue
		}
		sz, _ := strconv.ParseInt(meta[3], 10, 64)
		res.Total += sz
		rel := path
		if sp != "" {
			rel = strings.TrimPrefix(path, sp+"/")
		}
		if i := strings.IndexByte(rel, '/'); i >= 0 {
			// chemin complet du dossier enfant direct qui contient ce blob
			child := path[:len(path)-len(rel)] + rel[:i]
			res.Sizes[child] += sz
		}
	}
	return res, nil
}

// LocalTreeDates renvoie, pour chaque chemin, la date ISO8601 du dernier commit qui l'a touché
// (dernière « publication » locale). Best-effort, borné : appelé en arrière-plan par l'explorateur,
// pendant que la cellule date affiche un shimmer. Hors-ligne (log du clone).
func LocalTreeDates(dir string, paths []string) map[string]string {
	res := make(map[string]string, len(paths))
	for i, p := range paths {
		if i >= 200 || p == "" {
			break
		}
		out, err := run(dir, LocalTimeout, "log", "-1", "--format=%cI", "--", p)
		if err == nil {
			if d := strings.TrimSpace(out); d != "" {
				res[p] = d
			}
		}
	}
	return res
}
