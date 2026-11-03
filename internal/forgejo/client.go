// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

// Package forgejo est un client minimal de l'API REST Forgejo/Gitea.
// Il n'expose que les lectures nécessaires à SituLab, scopées aux droits du token.
//
// Garanties : TLS ≥ 1.2, timeout + annulation par requête (context), corps de réponse
// borné (anti-saturation mémoire), erreurs API traduites en texte lisible (jamais le
// corps JSON brut), et token transmis uniquement dans l'en-tête Authorization.
package forgejo

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

// cp1252hi : mappage des octets 0x80–0x9F de Windows-1252 (qui diffèrent de Latin-1).
var cp1252hi = map[byte]rune{
	0x80: '€', 0x82: '‚', 0x83: 'ƒ', 0x84: '„', 0x85: '…', 0x86: '†', 0x87: '‡', 0x88: 'ˆ', 0x89: '‰', 0x8A: 'Š', 0x8B: '‹', 0x8C: 'Œ', 0x8E: 'Ž',
	0x91: '‘', 0x92: '’', 0x93: '“', 0x94: '”', 0x95: '•', 0x96: '–', 0x97: '—', 0x98: '˜', 0x99: '™', 0x9A: 'š', 0x9B: '›', 0x9C: 'œ', 0x9E: 'ž', 0x9F: 'Ÿ',
}

// repairUTF8 corrige des octets Latin-1/Windows-1252 isolés dans un corps censé être UTF-8.
// Certaines descriptions Forgejo sont stockées en Latin-1 : l'API renvoie alors des octets
// invalides en UTF-8, que le décodeur JSON de Go remplacerait par « � ». On réinterprète ces
// octets invalides en CP1252 puis on les réencode en UTF-8. Les séquences UTF-8 VALIDES sont
// laissées strictement intactes (aucun risque pour le reste du corps).
func repairUTF8(b []byte) []byte {
	if utf8.Valid(b) {
		return b // cas normal : rien à réparer
	}
	out := make([]byte, 0, len(b)+16)
	for i := 0; i < len(b); {
		r, size := utf8.DecodeRune(b[i:])
		if r == utf8.RuneError && size == 1 { // octet invalide → interprété en CP1252
			c := b[i]
			rr, ok := cp1252hi[c]
			if !ok {
				rr = rune(c) // 0xA0–0xFF : Latin-1 = code point Unicode
			}
			out = utf8.AppendRune(out, rr)
			i++
			continue
		}
		out = append(out, b[i:i+size]...)
		i += size
	}
	return out
}

const (
	userAgent  = "SituLab"
	reqTimeout = 20 * time.Second
	maxAPIBody = 16 << 20 // plafond de corps générique (anti-DoS mémoire)
	perPage    = 50
	maxPages   = 40 // garde-fou : 2000 éléments avant de signaler une troncature
)

// ErrTooLarge signale une réponse (ou un fichier) dépassant la borne autorisée.
var ErrTooLarge = errors.New("réponse trop volumineuse")

// Kind distingue les fournisseurs supportés. L'API GitHub et l'API Forgejo/Gitea se
// recoupent largement pour les lectures ; les différences (préfixe d'URL, pagination,
// quelques noms de champs, fonctions d'administration) sont traitées ici.
type Kind int

const (
	KindForgejo Kind = iota // Forgejo/Gitea (API sous /api/v1)
	KindGitHub              // GitHub.com (API sur api.github.com)
)

const githubAPIBase = "https://api.github.com"

// ErrGitHubUnsupported signale une fonctionnalité propre à Forgejo indisponible sur GitHub
// (gestion des comptes, gestionnaire de jetons : GitHub ne les expose pas via l'API ici).
var ErrGitHubUnsupported = errors.New("fonction non disponible pour une instance GitHub")

// Client interroge une instance Forgejo/Gitea ou GitHub avec un token personnel.
// Immuable après construction (donc sûr en usage concurrent).
type Client struct {
	base  string
	token string
	http  *http.Client
	ver   string
	kind  Kind
}

// New crée un client Forgejo/Gitea pour l'URL de base donnée (ex. https://git.apcom.app).
// version sert à l'en-tête User-Agent (ex. "0.3").
func New(base, token, version string) *Client {
	return NewKind(base, token, version, KindForgejo)
}

// NewKind crée un client pour le fournisseur donné.
func NewKind(base, token, version string, kind Kind) *Client {
	// On part du transport par défaut (conserve Proxy=ProxyFromEnvironment, les
	// timeouts de connexion et HTTP/2) puis on impose TLS ≥ 1.2.
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	return &Client{
		base:  strings.TrimRight(base, "/"),
		token: token,
		ver:   version,
		kind:  kind,
		http:  &http.Client{Timeout: reqTimeout, Transport: tr},
	}
}

// IsGitHub indique si le client cible GitHub.
func (c *Client) IsGitHub() bool { return c.kind == KindGitHub }

// apiURL construit l'URL complète d'un chemin d'API selon le fournisseur.
func (c *Client) apiURL(path string) string {
	if c.kind == KindGitHub {
		return githubAPIBase + path
	}
	return c.base + "/api/v1" + path
}

// limKey renvoie le nom du paramètre de pagination (Forgejo: limit, GitHub: per_page).
func (c *Client) limKey() string {
	if c.kind == KindGitHub {
		return "per_page"
	}
	return "limit"
}

// requireForgejo renvoie ErrGitHubUnsupported si le client cible GitHub.
func (c *Client) requireForgejo() error {
	if c.kind == KindGitHub {
		return ErrGitHubUnsupported
	}
	return nil
}

// setHeaders pose l'authentification et les en-têtes communs, adaptés au fournisseur.
func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", "token "+c.token)
	req.Header.Set("User-Agent", userAgent+"/"+c.ver)
	if c.kind == KindGitHub {
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	} else {
		req.Header.Set("Accept", "application/json")
	}
}

// normalizeUser aligne les champs GitHub (name, site_admin) sur le modèle commun.
func (c *Client) normalizeUser(u *User) {
	if c.kind == KindGitHub {
		if u.FullName == "" {
			u.FullName = u.Name
		}
		u.IsAdmin = u.SiteAdmin
	}
}

// normalizeRepo aligne les champs GitHub (stargazers_count) sur le modèle commun.
func (c *Client) normalizeRepo(r *Repo) {
	if c.kind == KindGitHub && r.Stars == 0 {
		r.Stars = r.StarsGH
	}
}

// User est l'utilisateur courant. Les champs GitHub (name, site_admin) sont décodés en
// plus et normalisés vers FullName/IsAdmin par normalizeUser.
type User struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	FullName  string `json:"full_name"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
	IsAdmin   bool   `json:"is_admin"`
	// GitHub :
	Name      string `json:"name"`
	SiteAdmin bool   `json:"site_admin"`
}

// Repo est un dépôt.
type Repo struct {
	FullName      string    `json:"full_name"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	Private       bool      `json:"private"`
	Fork          bool      `json:"fork"`
	HTMLURL       string    `json:"html_url"`
	CloneURL      string    `json:"clone_url"`
	SSHURL        string    `json:"ssh_url"`
	DefaultBranch string    `json:"default_branch"`
	Stars         int       `json:"stars_count"`
	StarsGH       int       `json:"stargazers_count"` // GitHub
	Archived      bool      `json:"archived"`
	UpdatedAt     time.Time `json:"updated_at"`
	Permissions   struct {
		Admin bool `json:"admin"`
		Push  bool `json:"push"`
		Pull  bool `json:"pull"`
	} `json:"permissions"`
}

// Org est une organisation. GitHub expose le nom sous « login » (normalisé vers Username).
type Org struct {
	Username    string `json:"username"`
	Login       string `json:"login"` // GitHub
	FullName    string `json:"full_name"`
	Description string `json:"description"`
}

// Token est un token d'accès (valeur jamais renvoyée par l'API).
type Token struct {
	ID     int64    `json:"id"`
	Name   string   `json:"name"`
	Scopes []string `json:"scopes"`
}

// apiError est le corps d'erreur standard de Forgejo/Gitea.
type apiError struct {
	Message string `json:"message"`
}

// do effectue un GET authentifié et décode la réponse. maxBytes borne la lecture du
// corps ; si la borne est franchie, renvoie ErrTooLarge.
func (c *Client) do(ctx context.Context, path string, out any, maxBytes int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiURL(path), nil)
	if err != nil {
		return err
	}
	c.setHeaders(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return c.dialError(err)
	}
	defer resp.Body.Close()

	if resp.ContentLength > maxBytes {
		return ErrTooLarge
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return fmt.Errorf("lecture de la réponse interrompue : %w", err)
	}
	if int64(len(body)) > maxBytes {
		return ErrTooLarge
	}
	if err := httpError(resp.StatusCode, body, authAPIMsg); err != nil {
		return err
	}
	if out != nil {
		return json.Unmarshal(repairUTF8(body), out)
	}
	return nil
}

// reqJSON effectue une requête avec un corps JSON (POST/PATCH/PUT/DELETE) et décode
// la réponse. Utilisé pour les écritures (création de dépôt, d'issue…).
func (c *Client) reqJSON(ctx context.Context, method, path string, body, out any) error {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.apiURL(path), r)
	if err != nil {
		return err
	}
	c.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return c.dialError(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, maxAPIBody))
	if err := httpError(resp.StatusCode, data, authAPIMsg); err != nil {
		return err
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(repairUTF8(data), out)
	}
	return nil
}

// apiMessage extrait le champ message d'une erreur Gitea, sinon un libellé générique.
func apiMessage(body []byte, code int) string {
	var e apiError
	if json.Unmarshal(body, &e) == nil && strings.TrimSpace(e.Message) != "" {
		return strings.TrimSpace(e.Message)
	}
	return fmt.Sprintf("erreur du serveur (code %d)", code)
}

// authAPIMsg : message d'accès refusé pour une requête portée par un jeton d'API (do/reqJSON/getRaw).
// basicReq, qui authentifie par identifiants, utilise un message distinct (« identifiant… incorrect »).
const authAPIMsg = "accès refusé : token invalide ou droits insuffisants"

// httpError traduit un code d'échec HTTP en erreur lisible (mapping unique pour tous les helpers).
// authMsg adapte le cas 401/403 au contexte. Renvoie nil si le code n'est pas un échec.
func httpError(code int, body []byte, authMsg string) error {
	switch {
	case code == http.StatusUnauthorized || code == http.StatusForbidden:
		return errors.New(authMsg)
	case code == http.StatusTooManyRequests:
		return errors.New("trop de requêtes : réessayez dans un instant")
	case code >= 400:
		return errors.New(apiMessage(body, code))
	}
	return nil
}

// dialError traduit une erreur de transport (aucune réponse reçue) : délai dépassé vs injoignable.
func (c *Client) dialError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return fmt.Errorf("délai dépassé en joignant %s", c.base)
	}
	return fmt.Errorf("connexion à %s impossible", c.base)
}

// CurrentUser renvoie l'utilisateur authentifié.
func (c *Client) CurrentUser(ctx context.Context) (*User, error) {
	u := &User{}
	if err := c.do(ctx, "/user", u, maxAPIBody); err != nil {
		return nil, err
	}
	c.normalizeUser(u)
	return u, nil
}

// ListRepos renvoie les dépôts accessibles (pagination complète). Le booléen vaut true
// si le garde-fou de pages a été atteint (liste potentiellement incomplète).
func (c *Client) ListRepos(ctx context.Context) ([]Repo, bool, error) {
	var repos []Repo
	for page := 1; ; page++ {
		var batch []Repo
		p := fmt.Sprintf("/user/repos?%s=%d&page=%d", c.limKey(), perPage, page)
		if err := c.do(ctx, p, &batch, maxAPIBody); err != nil {
			return nil, false, err
		}
		for i := range batch {
			c.normalizeRepo(&batch[i])
		}
		repos = append(repos, batch...)
		if len(batch) < perPage {
			return repos, false, nil
		}
		if page >= maxPages {
			return repos, true, nil
		}
	}
}

// ListMyRepos renvoie TOUS les dépôts accessibles à l'utilisateur, y compris ceux de ses
// organisations/équipes. C'est la source du tableau de bord (vue complète, sans surprise).
//
//   - GitHub : /user/repos inclut déjà, par défaut (affiliation owner+collaborator+org_member),
//     les dépôts d'organisation → on réutilise ListRepos.
//   - Forgejo/Gitea : /user/repos ne liste QUE le perso + collaborations directes. On passe
//     donc par /repos/search?uid=<id> qui remonte « possède OU contribue » (orgs comprises).
//     Repli sur /user/repos si l'identifiant est indisponible ou la recherche échoue.
func (c *Client) ListMyRepos(ctx context.Context) ([]Repo, bool, error) {
	if c.kind == KindGitHub {
		return c.ListRepos(ctx)
	}
	u, err := c.CurrentUser(ctx)
	if err != nil || u.ID == 0 {
		return c.ListRepos(ctx)
	}
	repos, trunc, err := c.searchReposByUID(ctx, u.ID)
	if err != nil {
		return c.ListRepos(ctx) // repli : jamais de tableau de bord vide à cause de la recherche
	}
	return repos, trunc, nil
}

// searchReposByUID pagine /repos/search?uid=<id> (dépôts possédés ou contribués).
func (c *Client) searchReposByUID(ctx context.Context, uid int64) ([]Repo, bool, error) {
	var out []Repo
	for page := 1; ; page++ {
		var wrap struct {
			OK   bool   `json:"ok"`
			Data []Repo `json:"data"`
		}
		p := fmt.Sprintf("/repos/search?uid=%d&%s=%d&page=%d", uid, c.limKey(), perPage, page)
		if err := c.do(ctx, p, &wrap, maxAPIBody); err != nil {
			return nil, false, err
		}
		for i := range wrap.Data {
			c.normalizeRepo(&wrap.Data[i])
		}
		out = append(out, wrap.Data...)
		if len(wrap.Data) < perPage {
			return out, false, nil
		}
		if page >= maxPages {
			return out, true, nil
		}
	}
}

// ViewRepo renvoie le détail d'un dépôt owner/repo.
func (c *Client) ViewRepo(ctx context.Context, owner, repo string) (*Repo, error) {
	r := &Repo{}
	p := fmt.Sprintf("/repos/%s/%s", esc(owner), esc(repo))
	if err := c.do(ctx, p, r, maxAPIBody); err != nil {
		return nil, err
	}
	c.normalizeRepo(r)
	return r, nil
}

// ListOrgs renvoie les organisations de l'utilisateur.
func (c *Client) ListOrgs(ctx context.Context) ([]Org, error) {
	var orgs []Org
	if err := c.do(ctx, fmt.Sprintf("/user/orgs?%s=100", c.limKey()), &orgs, maxAPIBody); err != nil {
		return nil, err
	}
	if c.kind == KindGitHub {
		for i := range orgs {
			if orgs[i].Username == "" {
				orgs[i].Username = orgs[i].Login
			}
		}
	}
	return orgs, nil
}
