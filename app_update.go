// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	wailsrt "github.com/wailsapp/wails/v2/pkg/runtime"
)

// Vérification de mise à jour (1.21.1, besoin #3). La fenêtre ne PEUT PAS interroger l'API GitHub
// elle-même : la CSP `connect-src 'self'` interdit tout fetch externe depuis le WebView. C'est donc
// le backend Go qui lit la dernière Release publique et compare à la version installée, puis — sur
// demande explicite — télécharge l'installeur de l'OS et le lance (mise à jour « clé en main »).
//
// Dépôt public de référence (vitrine) :
const updateRepo = "Apcom-Solutions-SA/situlab"

// UpdateInfo est le verdict rendu à la fenêtre. Error non vide = la vérification a échoué (réseau,
// API) ; dans ce cas les autres champs sont vides et l'UI reste passive (pas de fausse alerte).
type UpdateInfo struct {
	Current         string `json:"current"`          // version installée (ex. "1.21.0")
	Latest          string `json:"latest"`           // dernier tag publié, nettoyé (ex. "1.21.1")
	UpdateAvailable bool   `json:"update_available"` // true si Latest > Current
	HTMLURL         string `json:"html_url"`         // page de la release (pour « voir les notes »)
	AssetURL        string `json:"asset_url"`        // URL de l'installeur pour CET OS (vide si aucun)
	AssetName       string `json:"asset_name"`       // nom de fichier de l'installeur
	Error           string `json:"error"`            // message d'erreur lisible, ou vide
}

// ghReleaseRaw : sous-ensemble de la réponse de l'API GitHub Releases (tag + assets).
type ghReleaseRaw struct {
	TagName    string `json:"tag_name"`
	HTMLURL    string `json:"html_url"`
	Prerelease bool   `json:"prerelease"`
	Draft      bool   `json:"draft"`
	Assets     []struct {
		Name        string `json:"name"`
		DownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// CheckUpdate interroge la dernière Release publiée sur le dépôt vitrine et compare à la version
// installée. Lecture seule, sans PIN : aucun secret, aucune mutation. Ne lève jamais : toute erreur
// est renvoyée dans UpdateInfo.Error pour que l'UI reste sobre (passive en cas d'échec réseau).
func (a *App) CheckUpdate() UpdateInfo {
	info := UpdateInfo{Current: version}
	rel, err := fetchLatestRelease()
	if err != nil {
		info.Error = err.Error()
		return info
	}
	info.HTMLURL = rel.HTMLURL
	info.Latest = normalizeVersion(rel.TagName)
	info.UpdateAvailable = compareVersions(info.Latest, normalizeVersion(version)) > 0
	if info.UpdateAvailable {
		if name, url := pickAssetForOS(rel); url != "" {
			info.AssetName = name
			info.AssetURL = url
		}
	}
	return info
}

// fetchLatestRelease lit /releases/latest (dernière release NON brouillon et NON préversion).
func fetchLatestRelease() (ghReleaseRaw, error) {
	var rel ghReleaseRaw
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	url := "https://api.github.com/repos/" + updateRepo + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return rel, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "SituLab-updater")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return rel, errors.New("serveur de versions injoignable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return rel, fmt.Errorf("réponse inattendue du serveur de versions (%d)", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return rel, err
	}
	if err := json.Unmarshal(body, &rel); err != nil {
		return rel, errors.New("réponse du serveur de versions illisible")
	}
	if rel.TagName == "" {
		return rel, errors.New("aucune version publiée trouvée")
	}
	return rel, nil
}

// normalizeVersion nettoie un tag pour la comparaison : retire un « v » de tête et tout suffixe de
// préversion (« 1.21.0-dev » → « 1.21.0 »). Les espaces sont supprimés.
func normalizeVersion(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "v")
	s = strings.TrimPrefix(s, "V")
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	return s
}

// compareVersions compare deux versions « a.b.c » numériquement. Renvoie 1 si x>y, -1 si x<y, 0 si
// égales. Un segment non numérique ou manquant vaut 0 (tolérant).
func compareVersions(x, y string) int {
	xs, ys := strings.Split(x, "."), strings.Split(y, ".")
	n := len(xs)
	if len(ys) > n {
		n = len(ys)
	}
	for i := 0; i < n; i++ {
		xi, yi := 0, 0
		if i < len(xs) {
			xi, _ = strconv.Atoi(strings.TrimSpace(xs[i]))
		}
		if i < len(ys) {
			yi, _ = strconv.Atoi(strings.TrimSpace(ys[i]))
		}
		if xi > yi {
			return 1
		}
		if xi < yi {
			return -1
		}
	}
	return 0
}

// pickAssetForOS choisit, parmi les assets de la release, l'installeur correspondant au système
// courant. Renvoie (nom, url), ou ("","") si aucun ne convient (alors l'UI renvoie vers la page web).
func pickAssetForOS(rel ghReleaseRaw) (string, string) {
	var exts []string
	var hints []string
	switch runtime.GOOS {
	case "windows":
		exts = []string{".exe", ".msi"}
		hints = []string{"windows", "win", "setup", "amd64"}
	case "darwin":
		exts = []string{".dmg", ".pkg"}
		hints = []string{"darwin", "macos", "mac", "osx"}
	default: // linux et autres
		exts = []string{".appimage", ".deb", ".rpm", ".tar.gz"}
		hints = []string{"linux"}
	}
	match := func(name string) bool {
		n := strings.ToLower(name)
		for _, e := range exts {
			if strings.HasSuffix(n, e) {
				return true
			}
		}
		for _, h := range hints {
			if strings.Contains(n, h) {
				return true
			}
		}
		return false
	}
	// Priorité à l'extension d'installeur la plus naturelle (premier ext listé), sinon premier indice.
	for _, want := range exts {
		for _, as := range rel.Assets {
			if strings.HasSuffix(strings.ToLower(as.Name), want) {
				return as.Name, as.DownloadURL
			}
		}
	}
	for _, as := range rel.Assets {
		if match(as.Name) {
			return as.Name, as.DownloadURL
		}
	}
	return "", ""
}

// DownloadAndRunUpdate télécharge l'installeur de la dernière version puis le lance et ferme SituLab,
// au plus près du « clé en main » (besoin #3). Le remplacement effectif de l'exécutable en cours est
// porté par l'installeur de l'OS (impossible d'écraser un .exe verrouillé pendant qu'il tourne sous
// Windows) : SituLab prépare, l'installeur applique. Pas de secret, pas de PIN.
func (a *App) DownloadAndRunUpdate() error {
	info := a.CheckUpdate()
	if info.Error != "" {
		return errors.New(info.Error)
	}
	if !info.UpdateAvailable {
		return errors.New("déjà à jour")
	}
	if info.AssetURL == "" {
		return errors.New("aucun installeur automatique pour ce système : utilisez la page des versions")
	}
	dir := filepath.Join(os.TempDir(), "situlab-update")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	dest := filepath.Join(dir, filepath.Base(info.AssetName))
	if err := downloadTo(info.AssetURL, dest); err != nil {
		return fmt.Errorf("téléchargement échoué : %w", err)
	}
	if err := launchInstaller(dest); err != nil {
		return fmt.Errorf("lancement de l'installeur échoué : %w", err)
	}
	// Laisse le temps à l'installeur de démarrer, puis ferme la fenêtre pour libérer le binaire.
	if a.ctx != nil {
		go func() {
			time.Sleep(1200 * time.Millisecond)
			wailsrt.Quit(a.ctx)
		}()
	}
	return nil
}

// downloadTo récupère une URL vers un fichier local (suivi des redirections, timeout généreux).
func downloadTo(url, dest string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "SituLab-updater")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return err
	}
	return nil
}

// launchInstaller démarre l'installeur téléchargé de façon détachée, selon l'OS.
func launchInstaller(path string) error {
	switch runtime.GOOS {
	case "windows":
		// cmd /c start "" "<installeur>" : détache le process de la fenêtre qui se ferme.
		cmd := exec.Command("cmd", "/c", "start", "", path)
		return cmd.Start()
	case "darwin":
		return exec.Command("open", path).Start()
	default:
		return exec.Command("xdg-open", path).Start()
	}
}
