// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package forgejo

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	maxFileBytes    = 1 << 20 // 1 Mo : plafond d'affichage d'un fichier
	maxFileResponse = 3 << 20 // borne du corps de la réponse contents pour un fichier
	defaultCommits  = 20
)

// Branch est une branche distante.
type Branch struct {
	Name          string `json:"name"`
	CommitSHA     string `json:"commit_sha"`
	CommitMessage string `json:"commit_message"`
	CommitDate    string `json:"commit_date"`
}

type apiBranch struct {
	Name   string `json:"name"`
	Commit struct {
		ID        string `json:"id"`
		Message   string `json:"message"`
		Timestamp string `json:"timestamp"`
	} `json:"commit"`
}

// Tag est un tag/version distant.
type Tag struct {
	Name string `json:"name"`
	SHA  string `json:"sha"`
}

type apiTag struct {
	Name   string `json:"name"`
	Commit struct {
		SHA string `json:"sha"`
	} `json:"commit"`
}

// Commit est un commit distant (message commenté lisible).
type Commit struct {
	SHA     string `json:"sha"`
	Short   string `json:"short"`
	Message string `json:"message"`
	Author  string `json:"author"`
	Date    string `json:"date"`
}

type apiCommit struct {
	SHA    string `json:"sha"`
	Commit struct {
		Message string `json:"message"`
		Author  struct {
			Name string `json:"name"`
			Date string `json:"date"`
		} `json:"author"`
	} `json:"commit"`
}

// Entry est une entrée d'arborescence (fichier ou dossier).
type Entry struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Type string `json:"type"` // "dir" | "file"
	Size int64  `json:"size"`
}

// contentsPath construit le chemin d'API contents avec une éventuelle réf.
func contentsPath(owner, repo, path, ref string) string {
	p := fmt.Sprintf("/repos/%s/%s/contents/%s", esc(owner), esc(repo), pathEsc(path))
	if ref != "" {
		p += "?ref=" + url.QueryEscape(ref)
	}
	return p
}

// ListBranches renvoie toutes les branches d'un dépôt (pagination complète).
func (c *Client) ListBranches(ctx context.Context, owner, repo string) ([]Branch, error) {
	var out []Branch
	for page := 1; ; page++ {
		var raw []apiBranch
		p := fmt.Sprintf("/repos/%s/%s/branches?%s=%d&page=%d", esc(owner), esc(repo), c.limKey(), perPage, page)
		if err := c.do(ctx, p, &raw, maxAPIBody); err != nil {
			return nil, err
		}
		for _, b := range raw {
			out = append(out, Branch{Name: b.Name, CommitSHA: b.Commit.ID, CommitMessage: firstLine(b.Commit.Message), CommitDate: b.Commit.Timestamp})
		}
		if len(raw) < perPage || page >= maxPages {
			return out, nil
		}
	}
}

// ListTags renvoie tous les tags (versions) d'un dépôt (pagination complète).
func (c *Client) ListTags(ctx context.Context, owner, repo string) ([]Tag, error) {
	var out []Tag
	for page := 1; ; page++ {
		var raw []apiTag
		p := fmt.Sprintf("/repos/%s/%s/tags?%s=%d&page=%d", esc(owner), esc(repo), c.limKey(), perPage, page)
		if err := c.do(ctx, p, &raw, maxAPIBody); err != nil {
			return nil, err
		}
		for _, t := range raw {
			out = append(out, Tag{Name: t.Name, SHA: t.Commit.SHA})
		}
		if len(raw) < perPage || page >= maxPages {
			return out, nil
		}
	}
}

// ListCommits renvoie les derniers commits d'une référence (branche/tag/sha). Pagine par
// pages de taille FIXE (perPage) jusqu'à atteindre limit, puis tronque — une taille de page
// variable provoquerait doublons et trous (la pagination par page suppose une page constante).
func (c *Client) ListCommits(ctx context.Context, owner, repo, ref string, limit int) ([]Commit, error) {
	if limit <= 0 {
		limit = defaultCommits
	}
	out := make([]Commit, 0, limit)
	for page := 1; len(out) < limit && page <= maxPages; page++ {
		var raw []apiCommit
		p := fmt.Sprintf("/repos/%s/%s/commits?%s=%d&page=%d", esc(owner), esc(repo), c.limKey(), perPage, page)
		if ref != "" {
			p += "&sha=" + url.QueryEscape(ref)
		}
		if err := c.do(ctx, p, &raw, maxAPIBody); err != nil {
			return nil, err
		}
		for _, cm := range raw {
			short := cm.SHA
			if len(short) > 8 {
				short = short[:8]
			}
			out = append(out, Commit{
				SHA: cm.SHA, Short: short,
				Message: firstLine(cm.Commit.Message),
				Author:  cm.Commit.Author.Name,
				Date:    cm.Commit.Author.Date,
			})
		}
		if len(raw) < perPage {
			break // fin de l'historique
		}
	}
	if len(out) > limit {
		out = out[:limit] // tronque au besoin (dernière page dépasse limit)
	}
	return out, nil
}

// CommitStat est un commit enrichi des volumes de lignes et de l'auteur (compte mappé si connu),
// pour l'analyse d'équipe (v1.5.0).
type CommitStat struct {
	SHA        string `json:"sha"`
	AuthorName string `json:"author_name"` // nom tel qu'inscrit dans le commit
	AuthorUser string `json:"author_user"` // login Forgejo mappé (si l'e-mail correspond à un compte)
	Date       string `json:"date"`        // date d'auteur (ISO 8601, avec décalage)
	Additions  int    `json:"additions"`
	Deletions  int    `json:"deletions"`
}

type apiCommitStat struct {
	SHA    string `json:"sha"`
	Commit struct {
		Author struct {
			Name string `json:"name"`
			Date string `json:"date"`
		} `json:"author"`
	} `json:"commit"`
	Author *struct {
		Login string `json:"login"`
	} `json:"author"`
	Stats *struct {
		Additions int `json:"additions"`
		Deletions int `json:"deletions"`
	} `json:"stats"`
}

// ListCommitsRange renvoie les commits d'un dépôt entre since et until (ISO 8601, bornes incluses
// côté serveur), avec les volumes de lignes (stat=true) et sans la liste des fichiers (files=false,
// pour alléger). ref vide = branche par défaut. Le second retour vaut true si la pagination s'est
// arrêtée sur la borne maxPages avant d'atteindre la fin des données (résultat partiel à signaler).
func (c *Client) ListCommitsRange(ctx context.Context, owner, repo, ref, since, until string) ([]CommitStat, bool, error) {
	// IMPORTANT : Forgejo/Gitea N'APPLIQUE PAS since/until sur /commits (contrairement à GitHub) —
	// l'API renverrait tout l'historique. On FILTRE donc par date côté client sur la date d'auteur,
	// et comme les commits arrivent du plus récent au plus ancien, on ARRÊTE la pagination dès qu'on
	// passe sous `since` (sinon les totaux explosent : milliers de commits / millions de lignes).
	sinceT, hasSince := parseRFC3339(since)
	untilT, hasUntil := parseRFC3339(until)

	var out []CommitStat
	truncated := true // passe à false dès qu'on atteint une fin de données « propre »
	for page := 1; page <= maxPages; page++ {
		var raw []apiCommitStat
		p := fmt.Sprintf("/repos/%s/%s/commits?%s=%d&page=%d&stat=true&files=false&verification=false",
			esc(owner), esc(repo), c.limKey(), perPage, page)
		if ref != "" {
			p += "&sha=" + url.QueryEscape(ref)
		}
		// since/until restent transmis (GitHub les respecte ; Forgejo les ignore, d'où le filtre client).
		if since != "" {
			p += "&since=" + url.QueryEscape(since)
		}
		if until != "" {
			p += "&until=" + url.QueryEscape(until)
		}
		if err := c.do(ctx, p, &raw, maxAPIBody); err != nil {
			return nil, false, err
		}
		reachedOlder := false
		for _, cm := range raw {
			cs := CommitStat{SHA: cm.SHA, AuthorName: cm.Commit.Author.Name, Date: cm.Commit.Author.Date}
			if cm.Author != nil {
				cs.AuthorUser = cm.Author.Login
			}
			if cm.Stats != nil {
				cs.Additions, cs.Deletions = cm.Stats.Additions, cm.Stats.Deletions
			}
			if t, ok := parseRFC3339(cs.Date); ok {
				if hasUntil && t.After(untilT) {
					continue // plus récent que la borne haute : on saute (mais on continue de paginer)
				}
				if hasSince && t.Before(sinceT) {
					reachedOlder = true // plus ancien que la borne basse : commits suivants encore plus vieux
					continue
				}
			}
			out = append(out, cs)
		}
		if reachedOlder || len(raw) < perPage {
			truncated = false
			break
		}
	}
	return out, truncated, nil
}

// parseRFC3339 tente plusieurs formats ISO courants ; renvoie (t, true) si réussi.
func parseRFC3339(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	for _, f := range []string{time.RFC3339, "2006-01-02T15:04:05Z07:00", "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.Parse(f, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// ListContents renvoie le contenu d'un dossier (path vide = racine) pour une réf.
func (c *Client) ListContents(ctx context.Context, owner, repo, path, ref string) ([]Entry, error) {
	if err := safePath(path); err != nil {
		return nil, err
	}
	var raw json.RawMessage
	if err := c.do(ctx, contentsPath(owner, repo, path, ref), &raw, maxAPIBody); err != nil {
		return nil, err
	}
	var entries []Entry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, fmt.Errorf("« %s » est un fichier, pas un dossier", path)
	}
	return entries, nil
}

// fileContent est le retour brut de l'API contents pour un fichier.
type fileContent struct {
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	Encoding string `json:"encoding"`
	Content  string `json:"content"`
	Type     string `json:"type"`
}

// GetFile renvoie le contenu texte d'un fichier. Refuse au-delà de maxFileBytes.
func (c *Client) GetFile(ctx context.Context, owner, repo, path, ref string) (string, error) {
	if err := safePath(path); err != nil {
		return "", err
	}
	var raw json.RawMessage
	if err := c.do(ctx, contentsPath(owner, repo, path, ref), &raw, maxFileResponse); err != nil {
		if err == ErrTooLarge {
			return "", fmt.Errorf("fichier trop volumineux à afficher")
		}
		return "", err
	}
	if len(raw) > 0 && raw[0] == '[' {
		return "", fmt.Errorf("« %s » est un dossier, pas un fichier", path)
	}
	var fc fileContent
	if err := json.Unmarshal(raw, &fc); err != nil {
		return "", err
	}
	if fc.Size > maxFileBytes {
		return "", fmt.Errorf("fichier trop volumineux à afficher (%d Ko)", fc.Size/1024)
	}
	if fc.Encoding == "base64" {
		// Tolérant aux retours-ligne que certaines versions insèrent dans le champ.
		clean := strings.NewReplacer("\n", "", "\r", "").Replace(fc.Content)
		b, err := base64.StdEncoding.DecodeString(clean)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	return fc.Content, nil
}

// --- Helpers d'échappement ---

func esc(s string) string { return url.PathEscape(s) }

// safePath refuse tout chemin dont un segment vaut « . », « .. » ou vide : url.PathEscape
// ne neutralise pas les points, et net/http n'aplanit pas « .. » → sans ce garde-fou un chemin
// façon « ../../admin/users » pourrait sortir du préfixe /contents (confinement des lectures).
func safePath(p string) error {
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return nil // racine
	}
	for _, s := range strings.Split(p, "/") {
		if s == "" || s == "." || s == ".." {
			return fmt.Errorf("chemin invalide : segment « %s »", s)
		}
	}
	return nil
}

// pathEsc échappe chaque segment d'un chemin tout en gardant les "/".
func pathEsc(p string) string {
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return ""
	}
	parts := strings.Split(p, "/")
	for i, s := range parts {
		parts[i] = url.PathEscape(s)
	}
	return strings.Join(parts, "/")
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}
