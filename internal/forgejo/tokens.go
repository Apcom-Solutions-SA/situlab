// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package forgejo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Gestion des jetons d'accès personnels.
//
// Forgejo impose une authentification BASIQUE (identifiant + mot de passe) pour
// lister, créer et supprimer des tokens — un token ne peut pas se gérer lui-même.
// Le mot de passe n'est utilisé que le temps de l'appel, jamais stocké. Les actions
// portent toujours sur le COMPTE fourni, avec ses propres droits.

// basicReq effectue une requête authentifiée en Basic (user+password). Si otp est non
// vide, le code 2FA est transmis dans l'en-tête X-Forgejo-OTP (compat X-Gitea-OTP).
func (c *Client) basicReq(ctx context.Context, method, path, user, password, otp string, body, out any) error {
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+"/api/v1"+path, r)
	if err != nil {
		return err
	}
	req.SetBasicAuth(user, password)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if otp != "" {
		req.Header.Set("X-Forgejo-OTP", otp)
		req.Header.Set("X-Gitea-OTP", otp)
	}
	req.Header.Set("User-Agent", userAgent+"/"+c.ver)

	resp, err := c.http.Do(req)
	if err != nil {
		return c.dialError(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, maxAPIBody))
	// Contexte d'authentification par identifiants : message 401/403 distinct du jeton d'API.
	if err := httpError(resp.StatusCode, data, "identifiant, mot de passe ou code 2FA incorrect"); err != nil {
		return err
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}

// TokenList liste les jetons du compte (otp = code 2FA si le compte en a un).
func (c *Client) TokenList(ctx context.Context, user, password, otp string) ([]Token, error) {
	if err := c.requireForgejo(); err != nil {
		return nil, err
	}
	var out []Token
	if err := c.basicReq(ctx, http.MethodGet, fmt.Sprintf("/users/%s/tokens?limit=%d", esc(user), perPage), user, password, otp, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// repoTarget désigne un dépôt pour un token restreint (owner + name).
type repoTarget struct {
	Owner string `json:"owner"`
	Name  string `json:"name"`
}

// TokenCreate crée un jeton et renvoie sa VALEUR (affichée une seule fois). Si repos
// (liste de « owner/repo ») est non vide, le token est restreint à ces dépôts.
func (c *Client) TokenCreate(ctx context.Context, user, password, otp, name string, scopes, repos []string) (string, error) {
	if err := c.requireForgejo(); err != nil {
		return "", err
	}
	body := map[string]any{"name": name, "scopes": scopes}
	if targets := toTargets(repos); len(targets) > 0 {
		body["repositories"] = targets
	}
	var out struct {
		SHA1 string `json:"sha1"`
	}
	if err := c.basicReq(ctx, http.MethodPost, fmt.Sprintf("/users/%s/tokens", esc(user)), user, password, otp, body, &out); err != nil {
		return "", err
	}
	return out.SHA1, nil
}

func toTargets(repos []string) []repoTarget {
	var out []repoTarget
	for _, r := range repos {
		if i := strings.IndexByte(r, '/'); i > 0 && i < len(r)-1 {
			out = append(out, repoTarget{Owner: r[:i], Name: r[i+1:]})
		}
	}
	return out
}

// TokenDelete supprime un jeton par son identifiant.
func (c *Client) TokenDelete(ctx context.Context, user, password, otp string, id int64) error {
	if err := c.requireForgejo(); err != nil {
		return err
	}
	return c.basicReq(ctx, http.MethodDelete, fmt.Sprintf("/users/%s/tokens/%d", esc(user), id), user, password, otp, nil, nil)
}

// TokenDeleteByName supprime un jeton par son NOM (Forgejo accepte nom ou id sur cet
// endpoint). Sert au nettoyage d'un token orphelin juste après sa création, quand on n'a
// pas son id. otp transmet le code 2FA si le compte en a un.
func (c *Client) TokenDeleteByName(ctx context.Context, user, password, otp, name string) error {
	return c.basicReq(ctx, http.MethodDelete, fmt.Sprintf("/users/%s/tokens/%s", esc(user), esc(name)), user, password, otp, nil, nil)
}
