// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package gitx

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// maxBranchDescribe borne le nombre d'appels `describe` (un par branche, local et
// distant). Au-delà, on montre les branches sans leur version pour rester léger.
const maxBranchDescribe = 60

// BranchRow est l'état d'une branche vu des deux côtés : ce poste et en ligne.
// Les versions sont le nom du tag le plus proche (ex. v1.2.0), vide si aucun.
type BranchRow struct {
	Name          string `json:"name"`
	Current       bool   `json:"current"`
	HasLocal      bool   `json:"has_local"`
	HasRemote     bool   `json:"has_remote"`
	LocalVersion  string `json:"local_version"`
	LocalDate     string `json:"local_date"`
	RemoteVersion string `json:"remote_version"`
	RemoteDate    string `json:"remote_date"`
	Ahead         int    `json:"ahead"`
	Behind        int    `json:"behind"`
	Author        string `json:"author"`   // auteur du dernier commit de la branche (« qui l'a ouverte »)
	Upstream      string `json:"upstream"` // branche de suivi (ex. origin/main) — « depuis quoi elle se met à jour »
	Merged        bool   `json:"merged"`   // pointe locale joignable depuis la branche d'intégration (main/master/courante) → sûre à supprimer
}

// BranchMatrix est l'union des branches locales et des branches distantes connues
// localement (refs origin/*, dans l'état du dernier rapatriement).
type BranchMatrix struct {
	IsRepo    bool        `json:"is_repo"`
	Branches  []BranchRow `json:"branches"`
	FetchedAt string      `json:"fetched_at"` // date du dernier « fetch » (FETCH_HEAD), si connue
}

// MatrixOf calcule la vue par branche d'un dépôt local, sans accès réseau. L'état
// « en ligne » reflète le dernier rapatriement (refs origin/*). Appel à la demande
// (ouverture de carte), jamais dans la boucle de rafraîchissement.
func MatrixOf(dir string) (*BranchMatrix, error) {
	m := &BranchMatrix{}
	if _, err := run(dir, LocalTimeout, "rev-parse", "--is-inside-work-tree"); err != nil {
		return m, nil // pas un dépôt : on renvoie vide sans erreur
	}
	m.IsRepo = true
	rows := map[string]*BranchRow{}
	order := []string{}
	get := func(name string) *BranchRow {
		if r, ok := rows[name]; ok {
			return r
		}
		r := &BranchRow{Name: name}
		rows[name] = r
		order = append(order, name)
		return r
	}

	// Branches locales : date du dernier commit + avance/retard sur leur distante.
	if out, err := run(dir, LocalTimeout, "for-each-ref", "refs/heads",
		"--format=%(refname:short)\x1f%(HEAD)\x1f%(committerdate:iso-strict)\x1f%(upstream:track)\x1f%(authorname)\x1f%(upstream:short)"); err == nil {
		for _, l := range splitLines(out) {
			f := strings.Split(l, "\x1f")
			if len(f) < 3 || f[0] == "" {
				continue
			}
			r := get(f[0])
			r.HasLocal = true
			r.Current = strings.TrimSpace(f[1]) == "*"
			r.LocalDate = strings.TrimSpace(f[2])
			if len(f) > 3 {
				r.Ahead, r.Behind = parseTrack(f[3])
			}
			if len(f) > 4 {
				r.Author = strings.TrimSpace(f[4])
			}
			if len(f) > 5 {
				r.Upstream = strings.TrimSpace(f[5])
			}
		}
	}

	// Branches distantes connues localement (origin/*), hors HEAD symbolique.
	if out, err := run(dir, LocalTimeout, "for-each-ref", "refs/remotes/origin",
		"--format=%(refname:short)\x1f%(committerdate:iso-strict)\x1f%(authorname)"); err == nil {
		for _, l := range splitLines(out) {
			f := strings.Split(l, "\x1f")
			if len(f) < 2 || f[0] == "" {
				continue
			}
			// f[0] vaut "origin/<branche>". Le HEAD symbolique du remote a pour nom
			// court "origin" (sans slash) : c'est un pointeur, pas une branche — on l'écarte.
			if !strings.Contains(f[0], "/") {
				continue
			}
			name := strings.TrimPrefix(f[0], "origin/")
			if name == "HEAD" || name == "" {
				continue
			}
			r := get(name)
			r.HasRemote = true
			r.RemoteDate = strings.TrimSpace(f[1])
			if r.Author == "" && len(f) > 2 { // auteur distant si la branche n'est pas locale
				r.Author = strings.TrimSpace(f[2])
			}
		}
	}

	// Versions (tag le plus proche) — bornées pour rester légères.
	n := 0
	for _, name := range order {
		r := rows[name]
		if n >= maxBranchDescribe {
			break
		}
		if r.HasLocal {
			r.LocalVersion = nearestTag(dir, name)
			n++
		}
		if r.HasRemote && n < maxBranchDescribe {
			r.RemoteVersion = nearestTag(dir, "origin/"+name)
			n++
		}
	}

	// Statut de fusion : une branche locale est « fusionnée » si sa pointe est joignable depuis la
	// branche d'intégration (main/master, sinon la branche courante). Sert à prévenir, AVANT une
	// suppression, qu'une branche jamais fusionnée ferait perdre du travail. UN SEUL `for-each-ref
	// --merged` (et non un `merge-base` par branche) : coût constant quel que soit le nombre de branches,
	// et pas de plafond (plus de faux « non fusionnée » au-delà du 60ᵉ). target est fusionnée dans
	// elle-même, donc présente dans l'ensemble.
	if target := integrationBranch(order, rows); target != "" && safeRevision(target) == nil {
		merged := map[string]bool{}
		if out, err := run(dir, LocalTimeout, "for-each-ref", "--format=%(refname:short)", "--merged="+target, "refs/heads"); err == nil {
			for _, name := range splitLines(out) {
				merged[name] = true
			}
		}
		for _, name := range order {
			if r := rows[name]; r.HasLocal {
				r.Merged = merged[name]
			}
		}
	}

	// Ordre : branche courante d'abord, puis alphabétique.
	sort.SliceStable(order, func(i, j int) bool {
		ri, rj := rows[order[i]], rows[order[j]]
		if ri.Current != rj.Current {
			return ri.Current
		}
		return order[i] < order[j]
	})
	for _, name := range order {
		m.Branches = append(m.Branches, *rows[name])
	}
	m.FetchedAt = fetchedAt(dir)
	return m, nil
}

// integrationBranch choisit la branche d'intégration de référence d'un dépôt pour juger si une branche
// de travail est « fusionnée » : main, sinon master, sinon la branche courante. "" si rien de local.
func integrationBranch(order []string, rows map[string]*BranchRow) string {
	has := func(n string) bool { r, ok := rows[n]; return ok && r.HasLocal }
	if has("main") {
		return "main"
	}
	if has("master") {
		return "master"
	}
	for _, n := range order {
		if r := rows[n]; r.HasLocal && r.Current {
			return n
		}
	}
	return ""
}

// BranchMergedInto indique si la pointe de `branch` est joignable depuis `into` (git merge-base
// --is-ancestor) — autrement dit si `branch` est déjà entièrement fusionnée dans `into`. Purement
// local. Une branche NON fusionnée supprimée perdrait son travail : l'appelant alerte avant.
func BranchMergedInto(dir, branch, into string) bool {
	if safeRevision(branch) != nil || safeRevision(into) != nil {
		return false
	}
	_, err := run(dir, LocalTimeout, "merge-base", "--is-ancestor", branch, into)
	return err == nil
}

// NearestTagOf expose nearestTag : le tag le plus proche joignable depuis une réf (ex. "origin/dev").
func NearestTagOf(dir, ref string) string { return nearestTag(dir, ref) }

// TagPublished indique si le commit d'un tag est déjà présent sur l'atelier (ancêtre de — ou égal à —
// remoteRef, p.ex. "origin/main"). Sert à n'autoriser la promotion que de contenu publié sur la
// source de vérité, jamais de travail encore local.
func TagPublished(dir, tag, remoteRef string) bool {
	_, err := run(dir, LocalTimeout, "merge-base", "--is-ancestor", tag+"^{commit}", remoteRef)
	return err == nil
}

// TagsFor renvoie les tags joignables depuis une réf (ex. "origin/dev"), du plus récent au plus
// ancien (tri de version). Sert à proposer les versions promouvables d'une branche d'atelier.
func TagsFor(dir, ref string) []string {
	out, err := run(dir, LocalTimeout, "tag", "--merged", ref, "--sort=-version:refname")
	if err != nil {
		return nil
	}
	return splitLines(out)
}

// nearestTag renvoie le nom du tag le plus proche d'une réf (ex. v1.2.0), ou "" si
// aucun tag n'est joignable. --abbrev=0 donne le nom pur, sans suffixe technique.
func nearestTag(dir, ref string) string {
	if out, err := run(dir, LocalTimeout, "describe", "--tags", "--abbrev=0", ref); err == nil {
		return strings.TrimSpace(out)
	}
	return ""
}

// parseTrack lit "[ahead 2, behind 3]" (format %(upstream:track)) en entiers.
func parseTrack(s string) (ahead, behind int) {
	s = strings.Trim(strings.TrimSpace(s), "[]")
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if v := strings.TrimPrefix(part, "ahead "); v != part {
			ahead, _ = strconv.Atoi(strings.TrimSpace(v))
		} else if v := strings.TrimPrefix(part, "behind "); v != part {
			behind, _ = strconv.Atoi(strings.TrimSpace(v))
		}
	}
	return
}

// fetchedAt renvoie la date de modification de FETCH_HEAD (dernier rapatriement).
func fetchedAt(dir string) string {
	gd, err := run(dir, LocalTimeout, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return ""
	}
	fi, err := os.Stat(filepath.Join(strings.TrimSpace(gd), "FETCH_HEAD"))
	if err != nil {
		return ""
	}
	return fi.ModTime().Format("2006-01-02T15:04:05Z07:00")
}

func splitLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		l = strings.TrimRight(l, "\r")
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}
