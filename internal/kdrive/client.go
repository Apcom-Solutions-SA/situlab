// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

// Package kdrive parle à l'API REST de kDrive (Infomaniak) avec un JETON API PERSONNEL de scope
// « drive » (généré par l'utilisateur dans son Manager ; ce n'est PAS un flux OAuth — le scope drive
// est fermé aux applications tierces en OAuth). Il expose la MÊME surface que internal/webdav
// (List/Get/Put/Mkcol/Delete/Move/TestConnection) pour être interchangeable derrière l'interface
// cloud du cœur. Différence clé face au WebDAV : l'API REST donne les fichiers « PARTAGÉS AVEC MOI »
// (GET /files/shared), que le WebDAV n'expose pas — c'est la raison d'être du mode REST.
//
// L'API adresse les fichiers par ID numérique, pas par chemin. Ce client maintient donc une
// résolution chemin↔ID (marche depuis la racine, avec cache) pour présenter au cœur des chemins
// POSIX comme le WebDAV. Endpoints vérifiés en direct le 2026-10-26 : GET /2/drive/{id},
// /2/drive/{id}/files, /files/{fid}/files, /files/{fid}, /files/{fid}/download, /files/shared.
package kdrive

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"situlab/internal/webdav"
)

const apiBase = "https://api.infomaniak.com"

// Client : un drive kDrive accédé par jeton API (Bearer).
type Client struct {
	drive string
	token string
	ua    string
	hc    *http.Client
	mu    sync.Mutex
	ids   map[string]int // chemin nettoyé → id de fichier/dossier (racine = 0)
}

// New construit un client REST kDrive. driveID peut contenir des non-chiffres (on ne garde que l'ID).
func New(driveID, token, ua string) *Client {
	return &Client{
		drive: digits(driveID),
		token: strings.TrimSpace(token),
		ua:    ua,
		hc:    &http.Client{Timeout: 90 * time.Second},
		ids:   map[string]int{"/": 0},
	}
}

func digits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// clean normalise un chemin POSIX (slash initial, sans slash final).
func clean(p string) string {
	p = strings.ReplaceAll(strings.TrimSpace(p), "\\", "/")
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	p = path.Clean(p)
	return p
}

type apiFile struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Type         string `json:"type"` // "dir" | autre (fichier)
	Size         int64  `json:"size"`
	LastModified int64  `json:"last_modified_at"`
}

type apiResp struct {
	Result string          `json:"result"`
	Data   json.RawMessage `json:"data"`
	Error  struct {
		Code        string `json:"code"`
		Description string `json:"description"`
	} `json:"error"`
}

func (c *Client) req(ctx context.Context, method, u string, body io.Reader, ctype string) ([]byte, int, error) {
	r, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, 0, err
	}
	r.Header.Set("Authorization", "Bearer "+c.token)
	r.Header.Set("Accept", "application/json")
	if c.ua != "" {
		r.Header.Set("User-Agent", "SituLab/"+c.ua)
	}
	if ctype != "" {
		r.Header.Set("Content-Type", ctype)
	}
	resp, err := c.hc.Do(r)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	return b, resp.StatusCode, nil
}

// apiData fait un GET et renvoie le champ data (erreur API → message lisible).
func (c *Client) apiData(ctx context.Context, u string) (json.RawMessage, error) {
	b, _, err := c.req(ctx, http.MethodGet, u, nil, "")
	if err != nil {
		return nil, err
	}
	var r apiResp
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("réponse kDrive illisible")
	}
	if r.Result != "success" {
		if r.Error.Description != "" {
			return nil, fmt.Errorf("kDrive : %s", r.Error.Description)
		}
		return nil, fmt.Errorf("kDrive : échec de la requête")
	}
	return r.Data, nil
}

func (c *Client) base() string { return apiBase + "/2/drive/" + c.drive }

// listByID liste les enfants d'un dossier (id 0 = racine).
func (c *Client) listByID(ctx context.Context, id int) ([]apiFile, error) {
	u := c.base() + "/files?per_page=500"
	if id != 0 {
		u = c.base() + "/files/" + strconv.Itoa(id) + "/files?per_page=500"
	}
	data, err := c.apiData(ctx, u)
	if err != nil {
		return nil, err
	}
	var files []apiFile
	if err := json.Unmarshal(data, &files); err != nil {
		return nil, fmt.Errorf("liste kDrive illisible")
	}
	return files, nil
}

func toEntry(dir string, f apiFile) webdav.Entry {
	mod := ""
	if f.LastModified > 0 {
		mod = time.Unix(f.LastModified, 0).UTC().Format(time.RFC1123)
	}
	return webdav.Entry{
		Name:     f.Name,
		Path:     clean(path.Join(dir, f.Name)),
		IsDir:    f.Type == "dir",
		Size:     f.Size,
		Modified: mod,
	}
}

// resolveID marche depuis la racine pour trouver l'id d'un chemin (avec cache). Renvoie aussi si
// c'est un dossier. "/" → (0, true).
func (c *Client) resolveID(ctx context.Context, p string) (int, bool, error) {
	cp := clean(p)
	if cp == "/" {
		return 0, true, nil
	}
	c.mu.Lock()
	if id, ok := c.ids[cp]; ok {
		c.mu.Unlock()
		return id, true, nil // en cache (on ne mémorise que des dossiers traversés + feuilles listées)
	}
	c.mu.Unlock()
	// Marche segment par segment depuis le parent connu le plus proche.
	segs := strings.Split(strings.Trim(cp, "/"), "/")
	curID := 0
	curPath := ""
	for _, seg := range segs {
		files, err := c.listByID(ctx, curID)
		if err != nil {
			return 0, false, err
		}
		found := false
		for _, f := range files {
			childPath := clean(curPath + "/" + f.Name)
			c.mu.Lock()
			c.ids[childPath] = f.ID
			c.mu.Unlock()
			if f.Name == seg {
				curID = f.ID
				curPath = childPath
				found = true
				if f.Type != "dir" {
					return f.ID, false, nil // feuille atteinte
				}
				break
			}
		}
		if !found {
			return 0, false, fmt.Errorf("introuvable sur kDrive : %s", cp)
		}
	}
	return curID, true, nil
}

// --- Surface compatible internal/webdav --------------------------------------------------------

// List renvoie le contenu d'un dossier (chemins POSIX).
func (c *Client) List(ctx context.Context, dir string) ([]webdav.Entry, error) {
	id, _, err := c.resolveID(ctx, dir)
	if err != nil {
		return nil, err
	}
	files, err := c.listByID(ctx, id)
	if err != nil {
		return nil, err
	}
	out := make([]webdav.Entry, 0, len(files))
	for _, f := range files {
		e := toEntry(dir, f)
		c.mu.Lock()
		c.ids[e.Path] = f.ID
		c.mu.Unlock()
		out = append(out, e)
	}
	return out, nil
}

// ListShared renvoie les fichiers/dossiers PARTAGÉS AVEC MOI (absent du WebDAV). Chemins virtuels
// sous "/" (ce sont des racines de partage).
func (c *Client) ListShared(ctx context.Context) ([]webdav.Entry, error) {
	data, err := c.apiData(ctx, c.base()+"/files/shared?per_page=500")
	if err != nil {
		return nil, err
	}
	var files []apiFile
	if err := json.Unmarshal(data, &files); err != nil {
		return nil, fmt.Errorf("partages kDrive illisibles")
	}
	out := make([]webdav.Entry, 0, len(files))
	for _, f := range files {
		out = append(out, toEntry("/", f))
	}
	return out, nil
}

// Get télécharge le contenu d'un fichier.
func (c *Client) Get(ctx context.Context, p string) ([]byte, error) {
	id, isDir, err := c.resolveID(ctx, p)
	if err != nil {
		return nil, err
	}
	if isDir {
		return nil, fmt.Errorf("chemin kDrive est un dossier : %s", p)
	}
	b, code, err := c.req(ctx, http.MethodGet, c.base()+"/files/"+strconv.Itoa(id)+"/download", nil, "")
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("téléchargement kDrive : HTTP %d", code)
	}
	return b, nil
}

// TestConnection vérifie le jeton sur le drive (GET /2/drive/{id}).
func (c *Client) TestConnection(ctx context.Context) error {
	_, err := c.apiData(ctx, c.base())
	return err
}

// Put téléverse un fichier dans le dossier parent du chemin (crée ou remplace par un nouveau dépôt).
// PORTÉ de la référence bat-i-webapp ; non vérifié en direct (écriture sur le drive réel).
func (c *Client) Put(ctx context.Context, p string, content []byte) error {
	dir := clean(path.Dir(clean(p)))
	name := path.Base(clean(p))
	parentID, _, err := c.resolveID(ctx, dir)
	if err != nil {
		return err
	}
	q := url.Values{}
	q.Set("directory_id", strconv.Itoa(parentID))
	q.Set("file_name", name)
	q.Set("conflict", "version") // ne jamais détruire : nouvelle version plutôt qu'écrasement
	u := c.base() + "/upload?" + q.Encode()
	b, code, err := c.req(ctx, http.MethodPost, u, bytes.NewReader(content), "application/octet-stream")
	if err != nil {
		return err
	}
	if code != http.StatusOK && code != http.StatusCreated {
		msg := strings.TrimSpace(string(b))
		if len(msg) > 120 {
			msg = msg[:120]
		}
		return fmt.Errorf("envoi kDrive : HTTP %d (%s)", code, msg)
	}
	c.invalidate(dir)
	return nil
}

// Mkcol crée un dossier. PORTÉ ; non vérifié en direct.
func (c *Client) Mkcol(ctx context.Context, p string) error {
	dir := clean(path.Dir(clean(p)))
	name := path.Base(clean(p))
	parentID, _, err := c.resolveID(ctx, dir)
	if err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]string{"name": name})
	u := c.base() + "/files/" + strconv.Itoa(parentID) + "/directory"
	b, code, err := c.req(ctx, http.MethodPost, u, bytes.NewReader(body), "application/json")
	if err != nil {
		return err
	}
	if code != http.StatusOK && code != http.StatusCreated {
		return fmt.Errorf("création dossier kDrive : HTTP %d (%s)", code, strings.TrimSpace(string(b)))
	}
	c.invalidate(dir)
	return nil
}

// Delete supprime un fichier ou un dossier. PORTÉ ; non vérifié en direct.
func (c *Client) Delete(ctx context.Context, p string) error {
	id, _, err := c.resolveID(ctx, p)
	if err != nil {
		return err
	}
	_, code, err := c.req(ctx, http.MethodDelete, c.base()+"/files/"+strconv.Itoa(id), nil, "")
	if err != nil {
		return err
	}
	if code != http.StatusOK && code != http.StatusNoContent && code != http.StatusAccepted {
		return fmt.Errorf("suppression kDrive : HTTP %d", code)
	}
	c.invalidate(clean(path.Dir(clean(p))))
	return nil
}

// Move déplace/renomme. PORTÉ ; non vérifié en direct.
func (c *Client) Move(ctx context.Context, from, to string) error {
	id, _, err := c.resolveID(ctx, from)
	if err != nil {
		return err
	}
	destDir := clean(path.Dir(clean(to)))
	destID, _, err := c.resolveID(ctx, destDir)
	if err != nil {
		return err
	}
	u := c.base() + "/files/" + strconv.Itoa(id) + "/move/" + strconv.Itoa(destID)
	_, code, err := c.req(ctx, http.MethodPost, u, nil, "")
	if err != nil {
		return err
	}
	if code != http.StatusOK && code != http.StatusCreated && code != http.StatusAccepted {
		return fmt.Errorf("déplacement kDrive : HTTP %d", code)
	}
	c.invalidate(clean(path.Dir(clean(from))))
	c.invalidate(destDir)
	return nil
}

func (c *Client) invalidate(dir string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	pfx := clean(dir)
	for k := range c.ids {
		if k == pfx || strings.HasPrefix(k, pfx+"/") {
			if k != "/" {
				delete(c.ids, k)
			}
		}
	}
}
