// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

// Package webdav est un client WebDAV minimal, fait main sur net/http (aucune dépendance
// externe), pour brancher SituLab à des clouds de fichiers souverains (Nextcloud, kDrive
// Infomaniak) qui portent le CORPUS.
//
// Garanties : TLS ≥ 1.2, timeout + annulation par requête (context), corps de réponse borné
// (anti-saturation mémoire), authentification Basic (identifiant + mot de passe/app-password)
// transmise uniquement dans l'en-tête Authorization. Le mot de passe vit chiffré au trousseau
// de SituLab, jamais dans la configuration, jamais remis à l'agent.
package webdav

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
)

const (
	userAgent = "SituLab"
	maxBody   = 128 << 20 // plafond de corps (anti-DoS mémoire) : 128 Mo par fichier
)

// ErrTooLarge signale un fichier dépassant la borne autorisée.
var ErrTooLarge = errors.New("fichier trop volumineux")

// Entry décrit une entrée d'un dossier cloud (fichier ou dossier).
type Entry struct {
	Name     string `json:"name"`     // nom de base
	Path     string `json:"path"`     // chemin cloud normalisé (ex. /Documents/a.md)
	IsDir    bool   `json:"is_dir"`   // vrai si c'est un dossier (collection)
	Size     int64  `json:"size"`     // taille en octets (fichiers)
	Modified string `json:"modified"` // date de dernière modification (RFC1123, telle que renvoyée)
	// Shared : l'entrée est PARTAGÉE AVEC MOI (son propriétaire n'est pas moi). Nextcloud le dit via
	// oc:owner-id (comparé à l'utilisateur connecté) ; kDrive via la liste /files/shared. Pour distinguer
	// d'un coup d'œil « mes fichiers » de « partagés avec moi » dans tous les arbres (1.19, #24).
	Shared bool `json:"shared,omitempty"`
}

// Client interroge un cloud WebDAV avec un identifiant et un mot de passe. Immuable après
// construction (sûr en usage concurrent).
type Client struct {
	base     string // base WebDAV, ex. https://host/remote.php/dav/files/USER
	basePath string // chemin de la base (pour relativiser les href renvoyés)
	user     string
	pass     string
	http     *http.Client
	ver      string
}

// New crée un client WebDAV pour la base donnée.
func New(base, user, pass, version string) *Client {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	bp := ""
	if u, err := url.Parse(base); err == nil {
		bp = strings.TrimRight(u.Path, "/")
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	// PAS de Timeout global au niveau du client : il bornerait la requete ENTIERE (envoi du corps
	// compris) et ecraserait le contexte par appel. Les deadlines viennent du ctx de chaque operation
	// (court pour lister/lire, backupUploadTimeout pour un gros bundle). Le transport garde ses garde-fous
	// de connexion/TLS (DialContext, TLSHandshakeTimeout) pour ne pas se figer sur l'etablissement.
	return &Client{
		base:     base,
		basePath: bp,
		user:     user,
		pass:     pass,
		ver:      version,
		http:     &http.Client{Transport: tr},
	}
}

// urlFor construit l'URL absolue d'un chemin cloud (chaque segment échappé).
func (c *Client) urlFor(p string) string {
	p = cleanPath(p)
	if p == "/" {
		return c.base + "/"
	}
	parts := strings.Split(strings.Trim(p, "/"), "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return c.base + "/" + strings.Join(parts, "/")
}

// cleanPath normalise un chemin cloud (POSIX, slash initial, sans slash final, « .. » résolu).
func cleanPath(p string) string {
	p = strings.ReplaceAll(strings.TrimSpace(p), "\\", "/")
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	p = path.Clean(p)
	if p != "/" {
		p = strings.TrimRight(p, "/")
	}
	return p
}

func (c *Client) newReq(ctx context.Context, method, p string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.urlFor(p), body)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.user, c.pass)
	req.Header.Set("User-Agent", userAgent+"/"+c.ver)
	return req, nil
}

// httpErr traduit un statut HTTP WebDAV en erreur lisible (jamais le corps brut).
func httpErr(resp *http.Response) error {
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("accès refusé par le cloud (identifiant ou mot de passe invalide)")
	case http.StatusNotFound:
		return fmt.Errorf("introuvable sur le cloud")
	case http.StatusMethodNotAllowed:
		return fmt.Errorf("opération non permise (le dossier existe déjà ?)")
	case http.StatusConflict:
		return fmt.Errorf("conflit : le dossier parent n'existe pas")
	default:
		return fmt.Errorf("le cloud a répondu %s", resp.Status)
	}
}

// --- Multi-Status (PROPFIND) -----------------------------------------------------------------

type msProp struct {
	ResourceType struct {
		Collection *struct{} `xml:"collection"`
	} `xml:"resourcetype"`
	ContentLength string `xml:"getcontentlength"`
	LastModified  string `xml:"getlastmodified"`
	OwnerID       string `xml:"http://owncloud.org/ns owner-id"` // Nextcloud : uid du propriétaire (≠ moi = partagé avec moi)
}

type msPropstat struct {
	Prop   msProp `xml:"prop"`
	Status string `xml:"status"`
}

type msResponse struct {
	Href     string       `xml:"href"`
	Propstat []msPropstat `xml:"propstat"`
}

type multistatus struct {
	XMLName   xml.Name     `xml:"multistatus"`
	Responses []msResponse `xml:"response"`
}

const propfindBody = `<?xml version="1.0" encoding="utf-8"?>
<d:propfind xmlns:d="DAV:" xmlns:oc="http://owncloud.org/ns"><d:prop>
<d:resourcetype/><d:getcontentlength/><d:getlastmodified/><oc:owner-id/>
</d:prop></d:propfind>`

// TestConnection vérifie que la base WebDAV répond et que les identifiants sont valides.
func (c *Client) TestConnection(ctx context.Context) error {
	req, err := c.newReq(ctx, "PROPFIND", "/", strings.NewReader(propfindBody))
	if err != nil {
		return err
	}
	req.Header.Set("Depth", "0")
	req.Header.Set("Content-Type", "application/xml; charset=utf-8")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, maxBody))
	if resp.StatusCode == http.StatusMultiStatus || resp.StatusCode == http.StatusOK {
		return nil
	}
	// En CONTEXTE CONNEXION, 401/403 = identifiants ; tout autre refus (405, 404, 3xx HTML…) signifie
	// « cette adresse ne parle pas WebDAV » — souvent une URL d'app web au lieu de l'endpoint.
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("identifiant ou mot de passe refusé par le cloud")
	}
	return fmt.Errorf("cette adresse ne répond pas en WebDAV (code %d), vérifiez l'URL et le type de cloud", resp.StatusCode)
}

// List énumère le contenu direct d'un dossier cloud (PROPFIND Depth 1). Le dossier lui-même est
// exclu du résultat.
func (c *Client) List(ctx context.Context, dir string) ([]Entry, error) {
	req, err := c.newReq(ctx, "PROPFIND", dir, strings.NewReader(propfindBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Depth", "1")
	req.Header.Set("Content-Type", "application/xml; charset=utf-8")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMultiStatus {
		io.Copy(io.Discard, io.LimitReader(resp.Body, maxBody))
		return nil, httpErr(resp)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, err
	}
	var ms multistatus
	if err := xml.Unmarshal(raw, &ms); err != nil {
		return nil, fmt.Errorf("réponse WebDAV illisible : %w", err)
	}
	self := cleanPath(dir)
	out := make([]Entry, 0, len(ms.Responses))
	for _, r := range ms.Responses {
		cp := c.hrefToPath(r.Href)
		if cp == "" || cp == self {
			continue // le dossier lui-même
		}
		var pr msProp
		for _, ps := range r.Propstat {
			if strings.Contains(ps.Status, "200") {
				pr = ps.Prop
				break
			}
		}
		e := Entry{
			Name:     path.Base(cp),
			Path:     cp,
			IsDir:    pr.ResourceType.Collection != nil,
			Modified: strings.TrimSpace(pr.LastModified),
		}
		fmt.Sscan(pr.ContentLength, &e.Size)
		// Nextcloud : propriétaire ≠ utilisateur connecté → partagé AVEC MOI (#24). Un cloud qui n'expose
		// pas oc:owner-id (ex. kDrive WebDAV) laisse OwnerID vide → non partagé, comportement inchangé.
		if oid := strings.TrimSpace(pr.OwnerID); oid != "" && !strings.EqualFold(oid, c.user) {
			e.Shared = true
		}
		out = append(out, e)
	}
	return out, nil
}

// hrefToPath convertit un href WebDAV (absolu ou relatif, échappé) en chemin cloud normalisé,
// relatif à la base de la connexion.
func (c *Client) hrefToPath(href string) string {
	h := href
	if u, err := url.Parse(href); err == nil {
		h = u.Path // retire schéma/hôte si href absolu ; décode l'échappement
	}
	if c.basePath != "" && strings.HasPrefix(h, c.basePath) {
		h = h[len(c.basePath):]
	}
	if h == "" {
		return "/"
	}
	return cleanPath(h)
}

// Get télécharge le contenu d'un fichier (borné).
func (c *Client) Get(ctx context.Context, p string) ([]byte, error) {
	req, err := c.newReq(ctx, http.MethodGet, p, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(resp.Body, maxBody))
		return nil, httpErr(resp)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBody {
		return nil, ErrTooLarge
	}
	return data, nil
}

// Put envoie (crée ou remplace) un fichier.
func (c *Client) Put(ctx context.Context, p string, content []byte) error {
	req, err := c.newReq(ctx, http.MethodPut, p, bytes.NewReader(content))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, maxBody))
	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusNoContent:
		return nil
	default:
		return httpErr(resp)
	}
}

// Mkcol crée un dossier (MKCOL).
func (c *Client) Mkcol(ctx context.Context, p string) error {
	req, err := c.newReq(ctx, "MKCOL", p, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, maxBody))
	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusNoContent:
		return nil
	case http.StatusMethodNotAllowed:
		return nil // le dossier existe déjà (MKCOL sur une collection existante) : pas une erreur
	default:
		return httpErr(resp)
	}
}

// Delete supprime un fichier ou un dossier.
func (c *Client) Delete(ctx context.Context, p string) error {
	req, err := c.newReq(ctx, http.MethodDelete, p, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, maxBody))
	switch resp.StatusCode {
	case http.StatusOK, http.StatusNoContent, http.StatusNotFound:
		return nil
	default:
		return httpErr(resp)
	}
}

// Move déplace (renomme) une ressource d'un chemin cloud vers un autre (WebDAV MOVE). Le dossier de
// destination doit exister. Overwrite=T : écrase une cible de même nom. Utilisé pour l'archivage des
// bundles de sauvegarde (rotation). from/to sont des chemins cloud (échappés par urlFor).
func (c *Client) Move(ctx context.Context, from, to string) error {
	req, err := c.newReq(ctx, "MOVE", from, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Destination", c.urlFor(to))
	req.Header.Set("Overwrite", "T")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, maxBody))
	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusNoContent:
		return nil
	default:
		return httpErr(resp)
	}
}
