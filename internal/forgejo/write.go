// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package forgejo

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// Issue est une issue ou une demande de fusion (forme commune lisible).
type Issue struct {
	Number  int    `json:"number"`
	Title   string `json:"title"`
	State   string `json:"state"`
	Body    string `json:"body"`
	HTMLURL string `json:"html_url"`
	User    struct {
		Login string `json:"login"`
	} `json:"user"`
	PullRequest *struct{} `json:"pull_request"` // non nul si l'issue est une PR
}

// Release est une publication/version.
type Release struct {
	ID         int64  `json:"id"`
	TagName    string `json:"tag_name"`
	Name       string `json:"name"`
	Body       string `json:"body"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
	HTMLURL    string `json:"html_url"`
}

// CreateRepo crée un dépôt. owner vide → espace personnel (POST /user/repos) ; owner non
// vide → dans l'organisation indiquée (POST /orgs/{owner}/repos). Beaucoup d'instances
// interdisent l'espace personnel (max_repo_creation=0) : la création passe alors par une org.
func (c *Client) CreateRepo(ctx context.Context, owner, name, description string, private bool) (*Repo, error) {
	body := map[string]any{"name": name, "description": description, "private": private, "auto_init": true}
	path := "/user/repos"
	if owner = strings.TrimSpace(owner); owner != "" {
		path = fmt.Sprintf("/orgs/%s/repos", esc(owner))
	}
	r := &Repo{}
	if err := c.reqJSON(ctx, http.MethodPost, path, body, r); err != nil {
		return nil, err
	}
	c.normalizeRepo(r)
	return r, nil
}

// DeleteRepo supprime définitivement un dépôt distant (DELETE /repos/{owner}/{repo}).
// Nécessite un jeton avec write:repository (et les droits sur le dépôt).
func (c *Client) DeleteRepo(ctx context.Context, owner, repo string) error {
	p := fmt.Sprintf("/repos/%s/%s", esc(owner), esc(repo))
	return c.reqJSON(ctx, http.MethodDelete, p, nil, nil)
}

// ListIssues renvoie les issues (hors PR) d'un dépôt, selon l'état (open|closed|all).
func (c *Client) ListIssues(ctx context.Context, owner, repo, state string) ([]Issue, error) {
	var all []Issue
	p := fmt.Sprintf("/repos/%s/%s/issues?state=%s&type=issues&%s=%d", esc(owner), esc(repo), normState(state), c.limKey(), perPage)
	if err := c.do(ctx, p, &all, maxAPIBody); err != nil {
		return nil, err
	}
	out := make([]Issue, 0, len(all))
	for _, i := range all {
		if i.PullRequest == nil { // on écarte les PR (remontées par cet endpoint)
			out = append(out, i)
		}
	}
	return out, nil
}

// CreateIssue crée une issue dans un dépôt.
func (c *Client) CreateIssue(ctx context.Context, owner, repo, title, body string) (*Issue, error) {
	i := &Issue{}
	payload := map[string]any{"title": title, "body": body}
	p := fmt.Sprintf("/repos/%s/%s/issues", esc(owner), esc(repo))
	if err := c.reqJSON(ctx, http.MethodPost, p, payload, i); err != nil {
		return nil, err
	}
	return i, nil
}

// CommentIssue ajoute un commentaire à une issue (ou une PR) par son numéro.
func (c *Client) CommentIssue(ctx context.Context, owner, repo string, index int, body string) error {
	p := fmt.Sprintf("/repos/%s/%s/issues/%d/comments", esc(owner), esc(repo), index)
	return c.reqJSON(ctx, http.MethodPost, p, map[string]any{"body": body}, nil)
}

// SetIssueState ferme (« closed ») ou rouvre (« open ») une issue par son numéro.
func (c *Client) SetIssueState(ctx context.Context, owner, repo string, index int, state string) error {
	p := fmt.Sprintf("/repos/%s/%s/issues/%d", esc(owner), esc(repo), index)
	return c.reqJSON(ctx, http.MethodPatch, p, map[string]any{"state": state}, nil)
}

// EditIssue modifie le titre et/ou le corps d'une issue (champs vides ignorés).
func (c *Client) EditIssue(ctx context.Context, owner, repo string, index int, title, body string) error {
	payload := map[string]any{}
	if title != "" {
		payload["title"] = title
	}
	if body != "" {
		payload["body"] = body
	}
	if len(payload) == 0 {
		return fmt.Errorf("rien à modifier")
	}
	p := fmt.Sprintf("/repos/%s/%s/issues/%d", esc(owner), esc(repo), index)
	return c.reqJSON(ctx, http.MethodPatch, p, payload, nil)
}

// CreatePull ouvre une demande de fusion de head vers base (branches du dépôt).
func (c *Client) CreatePull(ctx context.Context, owner, repo, title, head, base, body string) (*Issue, error) {
	payload := map[string]any{"title": title, "head": head, "base": base, "body": body}
	pr := &Issue{}
	p := fmt.Sprintf("/repos/%s/%s/pulls", esc(owner), esc(repo))
	if err := c.reqJSON(ctx, http.MethodPost, p, payload, pr); err != nil {
		return nil, err
	}
	return pr, nil
}

// MergePull fusionne une PR. method = merge | rebase | squash (défaut merge). L'API diffère
// selon le fournisseur : Forgejo/Gitea POST {Do} ; GitHub PUT {merge_method}.
func (c *Client) MergePull(ctx context.Context, owner, repo string, index int, method string) error {
	if method == "" {
		method = "merge"
	}
	p := fmt.Sprintf("/repos/%s/%s/pulls/%d/merge", esc(owner), esc(repo), index)
	if c.kind == KindGitHub {
		return c.reqJSON(ctx, http.MethodPut, p, map[string]any{"merge_method": method}, nil)
	}
	return c.reqJSON(ctx, http.MethodPost, p, map[string]any{"Do": method}, nil)
}

// normState borne l'état à {open, closed, all} (défaut open). Évite d'injecter une valeur
// libre dans la query (un paramètre de query se valide, il ne s'échappe pas « au mieux »).
func normState(s string) string {
	switch s {
	case "closed", "all":
		return s
	default:
		return "open"
	}
}

// ListPulls renvoie les demandes de fusion d'un dépôt.
func (c *Client) ListPulls(ctx context.Context, owner, repo, state string) ([]Issue, error) {
	var out []Issue
	p := fmt.Sprintf("/repos/%s/%s/pulls?state=%s&%s=%d", esc(owner), esc(repo), normState(state), c.limKey(), perPage)
	if err := c.do(ctx, p, &out, maxAPIBody); err != nil {
		return nil, err
	}
	return out, nil
}

// ListReleases renvoie les publications d'un dépôt.
func (c *Client) ListReleases(ctx context.Context, owner, repo string) ([]Release, error) {
	var out []Release
	p := fmt.Sprintf("/repos/%s/%s/releases?%s=%d", esc(owner), esc(repo), c.limKey(), perPage)
	if err := c.do(ctx, p, &out, maxAPIBody); err != nil {
		return nil, err
	}
	return out, nil
}
