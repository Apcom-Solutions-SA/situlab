// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package forgejo

import (
	"context"
	"fmt"
	"net/http"
)

// AdminUser est un utilisateur vu par l'administration. Les champs de droits
// (Restricted, ProhibitLogin, Visibility, MaxRepoCreation) servent à montrer, en
// clair, ce que la personne peut faire et voir.
type AdminUser struct {
	ID              int64  `json:"id"`
	Login           string `json:"login"`
	FullName        string `json:"full_name"`
	Email           string `json:"email"`
	AvatarURL       string `json:"avatar_url"`
	IsAdmin         bool   `json:"is_admin"`
	Active          bool   `json:"active"`
	Restricted      bool   `json:"restricted"`
	ProhibitLogin   bool   `json:"prohibit_login"`
	Visibility      string `json:"visibility"`
	MaxRepoCreation int    `json:"max_repo_creation"`
	Created         string `json:"created"`
	LastLogin       string `json:"last_login"`
}

// Activity est une entrée du fil d'activité d'un utilisateur (forme lisible).
type Activity struct {
	ID      int64  `json:"id"`
	OpType  string `json:"op_type"`
	Created string `json:"created"`
	RefName string `json:"ref_name"`
	Content string `json:"content"`
	Repo    struct {
		FullName string `json:"full_name"`
	} `json:"repo"`
}

// ListUsers renvoie les utilisateurs (nécessite un token administrateur). Pagination COMPLÈTE
// (comme ListRepos) : sans elle, seuls les 50 premiers comptes remontaient, faussant l'annuaire
// consolidé (comptes perdus, présence « absent » à tort). Plafonné par maxPages (garde-fou).
func (c *Client) ListUsers(ctx context.Context) ([]AdminUser, error) {
	if err := c.requireForgejo(); err != nil {
		return nil, err
	}
	var users []AdminUser
	for page := 1; ; page++ {
		var batch []AdminUser
		p := fmt.Sprintf("/admin/users?%s=%d&page=%d", c.limKey(), perPage, page)
		if err := c.do(ctx, p, &batch, maxAPIBody); err != nil {
			return nil, err
		}
		users = append(users, batch...)
		if len(batch) < perPage || page >= maxPages {
			return users, nil
		}
	}
}

// CreateUser crée un utilisateur (token administrateur requis). Le mot de passe devra
// être changé à la première connexion.
func (c *Client) CreateUser(ctx context.Context, username, email, password string, admin bool) (*AdminUser, error) {
	if err := c.requireForgejo(); err != nil {
		return nil, err
	}
	body := map[string]any{
		"username": username, "email": email, "password": password,
		"must_change_password": true, "admin": admin,
	}
	u := &AdminUser{}
	if err := c.reqJSON(ctx, http.MethodPost, "/admin/users", body, u); err != nil {
		return nil, err
	}
	return u, nil
}

// SetUserLogin suspend (prohibit=true) ou réactive (prohibit=false) la connexion
// d'un compte. C'est une révocation réversible : le compte et ses données restent.
func (c *Client) SetUserLogin(ctx context.Context, username string, prohibit bool) error {
	if err := c.requireForgejo(); err != nil {
		return err
	}
	body := map[string]any{
		"login_name":     username,
		"source_id":      0,
		"prohibit_login": prohibit,
	}
	return c.reqJSON(ctx, http.MethodPatch, "/admin/users/"+esc(username), body, nil)
}

// DeleteUser supprime définitivement un compte. purge=true efface aussi ses dépôts.
func (c *Client) DeleteUser(ctx context.Context, username string, purge bool) error {
	if err := c.requireForgejo(); err != nil {
		return err
	}
	p := "/admin/users/" + esc(username)
	if purge {
		p += "?purge=true"
	}
	return c.reqJSON(ctx, http.MethodDelete, p, nil, nil)
}

// UserAccessibleRepos renvoie les dépôts qu'un utilisateur possède OU auxquels il
// contribue (accès direct ou via une organisation/équipe). C'est l'image fidèle de
// « ce qu'il peut voir ». Recherche par identifiant numérique (uid).
func (c *Client) UserAccessibleRepos(ctx context.Context, uid int64, limit int) ([]Repo, error) {
	if err := c.requireForgejo(); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > perPage {
		limit = perPage
	}
	var wrap struct {
		OK   bool   `json:"ok"`
		Data []Repo `json:"data"`
	}
	p := fmt.Sprintf("/repos/search?uid=%d&limit=%d", uid, limit)
	if err := c.do(ctx, p, &wrap, maxAPIBody); err != nil {
		return nil, err
	}
	return wrap.Data, nil
}

// UserActivities renvoie les dernières activités d'un utilisateur (actions qu'il a
// lui-même réalisées). Léger : une seule page, limitée.
func (c *Client) UserActivities(ctx context.Context, username string, limit int) ([]Activity, error) {
	if err := c.requireForgejo(); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > perPage {
		limit = perPage
	}
	var out []Activity
	p := fmt.Sprintf("/users/%s/activities/feeds?only-performed-by=true&limit=%d", esc(username), limit)
	if err := c.do(ctx, p, &out, maxAPIBody); err != nil {
		return nil, err
	}
	return out, nil
}
