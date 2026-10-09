// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package gitx

import (
	"strconv"
	"strings"
)

// Limites de journal pour la vue détail.
const (
	recentLogLimit   = 15
	unpushedLogLimit = 50
)

// LocalCommit est un commit local lisible.
type LocalCommit struct {
	SHA     string `json:"sha"` // SHA complet (nécessaire pour lire l'attestation du commit)
	Short   string `json:"short"`
	Message string `json:"message"`
	Date    string `json:"date"`
	Author  string `json:"author"`
}

// BranchInfo décrit une branche locale et sa version (tag le plus proche).
type BranchInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Current bool   `json:"current"`
}

// Detail enrichit Status avec la version décrite, les branches locales (avec leur
// version) et les commits (récents + non poussés). Embarquement simple de Status
// (pas d'imbrication anonyme : le générateur de modèles Wails ne la supporte pas).
type Detail struct {
	Status
	Describe      string        `json:"describe"`
	Branches      []BranchInfo  `json:"branches"`
	RecentCommits []LocalCommit `json:"recent_commits"`
	Unpushed      []LocalCommit `json:"unpushed"`
}

// DetailOf renvoie l'état détaillé d'un dépôt local.
func DetailOf(dir string) (*Detail, error) {
	st, err := StatusOf(dir)
	if err != nil {
		return &Detail{Status: *st}, err
	}
	d := &Detail{Status: *st}
	if !st.IsRepo {
		return d, nil
	}
	d.Describe = describe(dir)
	sum := st // alias pour la suite (upstream)
	if out, err := run(dir, LocalTimeout, "for-each-ref", "refs/heads", "--format=%(refname:short)%09%(HEAD)"); err == nil {
		for _, l := range strings.Split(out, "\n") {
			l = strings.TrimRight(l, "\r")
			if l == "" {
				continue
			}
			f := strings.SplitN(l, "\t", 2)
			bi := BranchInfo{Name: f[0], Current: len(f) > 1 && strings.TrimSpace(f[1]) == "*"}
			if v, err := run(dir, LocalTimeout, "describe", "--tags", "--always", bi.Name); err == nil {
				bi.Version = strings.TrimSpace(v)
			}
			d.Branches = append(d.Branches, bi)
		}
	}
	d.RecentCommits = parseLog(dir, "", recentLogLimit)
	if sum.Upstream != "" {
		d.Unpushed = parseLog(dir, sum.Upstream+"..HEAD", unpushedLogLimit)
	}
	return d, nil
}

// parseLog lit un journal git sous forme structurée. Le `--` final protège contre une
// révision commençant par `-`. Best-effort : renvoie nil en cas d'échec.
func parseLog(dir, rng string, limit int) []LocalCommit {
	args := []string{"log", "--pretty=format:%H\x1f%h\x1f%s\x1f%cI\x1f%an"}
	if limit > 0 {
		args = append(args, "-n", strconv.Itoa(limit))
	}
	if rng != "" {
		args = append(args, rng)
	}
	args = append(args, "--")
	out, err := run(dir, LocalTimeout, args...)
	if err != nil {
		return nil
	}
	var res []LocalCommit
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		f := strings.Split(line, "\x1f")
		if len(f) < 5 {
			continue
		}
		res = append(res, LocalCommit{SHA: f[0], Short: f[1], Message: f[2], Date: f[3], Author: f[4]})
	}
	return res
}
