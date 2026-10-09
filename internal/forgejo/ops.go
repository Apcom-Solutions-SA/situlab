// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package forgejo

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

// --- GET brut (texte) : pour le diff d'une demande de fusion ---

// getRaw récupère un corps texte (non-JSON) — ex. le diff unifié d'une PR. accept permet
// d'imposer un type (ex. le diff GitHub via vnd.github.v3.diff) ; vide = texte générique.
func (c *Client) getRaw(ctx context.Context, path, accept string, maxBytes int64) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiURL(path), nil)
	if err != nil {
		return "", err
	}
	c.setHeaders(req)
	if accept == "" {
		accept = "text/plain, */*"
	}
	req.Header.Set("Accept", accept)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", c.dialError(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return "", err
	}
	if int64(len(body)) > maxBytes {
		return "", ErrTooLarge
	}
	if err := httpError(resp.StatusCode, body, authAPIMsg); err != nil {
		return "", err
	}
	return string(repairUTF8(body)), nil
}

// --- Demandes de fusion (détail, diff, fichiers, revue, état, édition) ---

// PullInfo est le détail d'une demande de fusion.
type PullInfo struct {
	Number    int    `json:"number"`
	Title     string `json:"title"`
	State     string `json:"state"`
	Body      string `json:"body"`
	HTMLURL   string `json:"html_url"`
	Mergeable bool   `json:"mergeable"`
	Merged    bool   `json:"merged"`
	Head      struct {
		Ref string `json:"ref"`
		Sha string `json:"sha"`
	} `json:"head"`
	Base struct {
		Ref string `json:"ref"`
	} `json:"base"`
	User struct {
		Login string `json:"login"`
	} `json:"user"`
}

// PullFile est un fichier modifié par une PR.
type PullFile struct {
	Filename  string `json:"filename"`
	Status    string `json:"status"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Changes   int    `json:"changes"`
}

// PullDetail renvoie le détail d'une PR.
func (c *Client) PullDetail(ctx context.Context, owner, repo string, index int) (*PullInfo, error) {
	pi := &PullInfo{}
	p := fmt.Sprintf("/repos/%s/%s/pulls/%d", esc(owner), esc(repo), index)
	if err := c.do(ctx, p, pi, maxAPIBody); err != nil {
		return nil, err
	}
	return pi, nil
}

// PullDiff renvoie le diff unifié d'une PR (texte). Forgejo expose « …/pulls/{n}.diff » ;
// GitHub le sert via l'en-tête Accept sur « …/pulls/{n} ».
func (c *Client) PullDiff(ctx context.Context, owner, repo string, index int) (string, error) {
	if c.kind == KindGitHub {
		p := fmt.Sprintf("/repos/%s/%s/pulls/%d", esc(owner), esc(repo), index)
		return c.getRaw(ctx, p, "application/vnd.github.v3.diff", maxFileResponse)
	}
	p := fmt.Sprintf("/repos/%s/%s/pulls/%d.diff", esc(owner), esc(repo), index)
	return c.getRaw(ctx, p, "", maxFileResponse)
}

// PullFiles liste les fichiers modifiés par une PR.
func (c *Client) PullFiles(ctx context.Context, owner, repo string, index int) ([]PullFile, error) {
	var out []PullFile
	p := fmt.Sprintf("/repos/%s/%s/pulls/%d/files?%s=%d", esc(owner), esc(repo), index, c.limKey(), perPage)
	if err := c.do(ctx, p, &out, maxAPIBody); err != nil {
		return nil, err
	}
	return out, nil
}

// ReviewPull dépose une revue : event = APPROVED | REQUEST_CHANGES | COMMENT.
func (c *Client) ReviewPull(ctx context.Context, owner, repo string, index int, event, body string) error {
	p := fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews", esc(owner), esc(repo), index)
	return c.reqJSON(ctx, http.MethodPost, p, map[string]any{"event": event, "body": body}, nil)
}

// SetPullState ferme (« closed ») ou rouvre (« open ») une PR sans la fusionner.
func (c *Client) SetPullState(ctx context.Context, owner, repo string, index int, state string) error {
	p := fmt.Sprintf("/repos/%s/%s/pulls/%d", esc(owner), esc(repo), index)
	return c.reqJSON(ctx, http.MethodPatch, p, map[string]any{"state": state}, nil)
}

// EditPull modifie titre/corps d'une PR (champs vides ignorés).
func (c *Client) EditPull(ctx context.Context, owner, repo string, index int, title, body string) error {
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
	p := fmt.Sprintf("/repos/%s/%s/pulls/%d", esc(owner), esc(repo), index)
	return c.reqJSON(ctx, http.MethodPatch, p, payload, nil)
}

// --- Releases & tags ---

// CreateRelease publie une version attachée à un tag (créé si besoin via target).
func (c *Client) CreateRelease(ctx context.Context, owner, repo, tag, name, body, target string, draft, prerelease bool) (*Release, error) {
	payload := map[string]any{"tag_name": tag, "name": name, "body": body, "draft": draft, "prerelease": prerelease}
	if target != "" {
		payload["target_commitish"] = target
	}
	rel := &Release{}
	p := fmt.Sprintf("/repos/%s/%s/releases", esc(owner), esc(repo))
	if err := c.reqJSON(ctx, http.MethodPost, p, payload, rel); err != nil {
		return nil, err
	}
	return rel, nil
}

// DeleteRelease supprime une release par son id.
func (c *Client) DeleteRelease(ctx context.Context, owner, repo string, id int64) error {
	p := fmt.Sprintf("/repos/%s/%s/releases/%d", esc(owner), esc(repo), id)
	return c.reqJSON(ctx, http.MethodDelete, p, nil, nil)
}

// CreateTag crée un tag (sur target, ou la branche par défaut si vide).
func (c *Client) CreateTag(ctx context.Context, owner, repo, tag, target string) error {
	payload := map[string]any{"tag_name": tag}
	if target != "" {
		payload["target"] = target
	}
	p := fmt.Sprintf("/repos/%s/%s/tags", esc(owner), esc(repo))
	return c.reqJSON(ctx, http.MethodPost, p, payload, nil)
}

// DeleteTag supprime un tag par son nom.
func (c *Client) DeleteTag(ctx context.Context, owner, repo, tag string) error {
	p := fmt.Sprintf("/repos/%s/%s/tags/%s", esc(owner), esc(repo), esc(tag))
	return c.reqJSON(ctx, http.MethodDelete, p, nil, nil)
}

// --- Réglages de dépôt ---

// EditRepo applique une modification générique (PATCH /repos/{o}/{r}) : name (renommer),
// private, archived, default_branch, description…
func (c *Client) EditRepo(ctx context.Context, owner, repo string, fields map[string]any) (*Repo, error) {
	r := &Repo{}
	p := fmt.Sprintf("/repos/%s/%s", esc(owner), esc(repo))
	if err := c.reqJSON(ctx, http.MethodPatch, p, fields, r); err != nil {
		return nil, err
	}
	c.normalizeRepo(r)
	return r, nil
}

// --- Labels, assignés, milestones (organisation des issues) ---

// Label est une étiquette de dépôt.
type Label struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// Milestone est un jalon de dépôt.
type Milestone struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
	State string `json:"state"`
}

// ListLabels liste les étiquettes d'un dépôt.
func (c *Client) ListLabels(ctx context.Context, owner, repo string) ([]Label, error) {
	var out []Label
	p := fmt.Sprintf("/repos/%s/%s/labels?%s=%d", esc(owner), esc(repo), c.limKey(), perPage)
	if err := c.do(ctx, p, &out, maxAPIBody); err != nil {
		return nil, err
	}
	return out, nil
}

// SetIssueLabels remplace les étiquettes d'une issue (par identifiants).
func (c *Client) SetIssueLabels(ctx context.Context, owner, repo string, index int, labelIDs []int64) error {
	p := fmt.Sprintf("/repos/%s/%s/issues/%d/labels", esc(owner), esc(repo), index)
	return c.reqJSON(ctx, http.MethodPut, p, map[string]any{"labels": labelIDs}, nil)
}

// SetIssueAssignees remplace les assignés d'une issue (par identifiants de connexion).
func (c *Client) SetIssueAssignees(ctx context.Context, owner, repo string, index int, assignees []string) error {
	p := fmt.Sprintf("/repos/%s/%s/issues/%d", esc(owner), esc(repo), index)
	return c.reqJSON(ctx, http.MethodPatch, p, map[string]any{"assignees": assignees}, nil)
}

// ListMilestones liste les jalons d'un dépôt.
func (c *Client) ListMilestones(ctx context.Context, owner, repo string) ([]Milestone, error) {
	var out []Milestone
	p := fmt.Sprintf("/repos/%s/%s/milestones?%s=%d", esc(owner), esc(repo), c.limKey(), perPage)
	if err := c.do(ctx, p, &out, maxAPIBody); err != nil {
		return nil, err
	}
	return out, nil
}

// SetIssueMilestone fixe (ou retire avec 0) le jalon d'une issue.
func (c *Client) SetIssueMilestone(ctx context.Context, owner, repo string, index int, milestoneID int64) error {
	p := fmt.Sprintf("/repos/%s/%s/issues/%d", esc(owner), esc(repo), index)
	return c.reqJSON(ctx, http.MethodPatch, p, map[string]any{"milestone": milestoneID}, nil)
}
