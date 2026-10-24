// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"

	"situlab/internal/config"
	"situlab/internal/keyring"
	"situlab/internal/webdav"
)

// buildWebDAVBase reconstruit l'URL WebDAV RÉELLE d'un cloud à partir de ce que l'utilisateur colle
// (souvent l'adresse du navigateur : page de login, tableau de bord…), par type de cloud.
//
//   - Nextcloud : <origine>[/sous-dossier]/remote.php/dav/files/<user> — on retire les segments
//     d'application connus (/index.php, /apps/…, /login, /s/…) et on ajoute le chemin dav. Si l'URL
//     contient déjà /remote.php/dav, on la garde telle quelle.
//   - kDrive : l'endpoint est l'ORIGINE de l'ID (https://<ID>.connect.kdrive.infomaniak.com). On
//     retire tout chemin. Pour les hôtes génériques connect.kdrive/drive.infomaniak.com, on conserve
//     le 1er segment de chemin (l'identifiant du kDrive).
func buildWebDAVBase(kind, rawURL, user string) (string, error) {
	raw := strings.TrimSpace(rawURL)
	if raw == "" {
		return "", errors.New("adresse du cloud requise")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("adresse invalide : %q", rawURL)
	}
	origin := u.Scheme + "://" + u.Host

	if kind == config.CloudKDrive {
		host := strings.ToLower(u.Host)
		// Déjà la forme endpoint sous-domaine <ID>.connect.kdrive|drive.infomaniak.com → origine seule.
		if strings.HasSuffix(host, ".connect.kdrive.infomaniak.com") || strings.HasSuffix(host, ".connect.drive.infomaniak.com") {
			return origin, nil
		}
		// Hôte générique connect.* : l'ID est le 1er segment du chemin.
		if host == "connect.kdrive.infomaniak.com" || host == "connect.drive.infomaniak.com" {
			if seg := firstSegment(u.Path); seg != "" {
				return origin + "/" + seg, nil
			}
			return origin, nil
		}
		// Sinon l'utilisateur a collé l'URL de l'app web (kdrive.infomaniak.com/app/drive/<ID>/…) ou
		// juste l'ID : on reconstruit l'endpoint WebDAV à partir de l'identifiant numérique du kDrive.
		if id := kdriveID(u.Host, u.Path); id != "" {
			return u.Scheme + "://" + id + ".connect.kdrive.infomaniak.com", nil
		}
		return "", fmt.Errorf("adresse kDrive non reconnue : indiquez « https://<ID>.connect.kdrive.infomaniak.com » (l'ID est le numéro après /drive/ dans votre navigateur)")
	}

	// Nextcloud
	p := u.Path
	if i := strings.Index(p, "/remote.php/dav"); i >= 0 {
		return strings.TrimRight(origin+p, "/"), nil // déjà une URL dav : conservée
	}
	// Retire les segments d'application pour retrouver l'éventuel sous-dossier d'installation.
	for _, marker := range []string{"/index.php", "/apps", "/remote.php", "/login", "/s/", "/settings"} {
		if i := strings.Index(p, marker); i >= 0 {
			p = p[:i]
		}
	}
	p = strings.TrimRight(p, "/")
	return origin + p + "/remote.php/dav/files/" + url.PathEscape(user), nil
}

// firstSegment renvoie le premier segment non vide d'un chemin (sans slash), ou "".
func firstSegment(p string) string {
	for _, s := range strings.Split(p, "/") {
		if s != "" {
			return s
		}
	}
	return ""
}

// kdriveID extrait l'identifiant NUMÉRIQUE du kDrive d'une URL de l'app web Infomaniak. L'ID est le
// segment juste APRÈS « drive » (ex. .../drive/3869957/ → 3869957), à distinguer d'un éventuel
// identifiant de compte présent plus tôt (ex. ksuite.infomaniak.com/53843/kdrive/app/drive/<ID>).
// Repli : si l'hôte est purement numérique (l'utilisateur n'a saisi que l'ID) ; sinon le DERNIER
// segment numérique (l'ID du drive est le plus profond).
func kdriveID(host, pth string) string {
	if isDigits(host) {
		return host
	}
	segs := []string{}
	for _, s := range strings.Split(pth, "/") {
		if s != "" {
			segs = append(segs, s)
		}
	}
	for i, s := range segs {
		if strings.EqualFold(s, "drive") && i+1 < len(segs) && isDigits(segs[i+1]) {
			return segs[i+1]
		}
	}
	last := ""
	for _, s := range segs {
		if isDigits(s) {
			last = s
		}
	}
	return last
}

// isDigits dit si s est non vide et composé uniquement de chiffres.
func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// ErrNoCloud : aucune connexion cloud ne correspond.
var ErrNoCloud = errors.New("connexion cloud introuvable")

// ErrCloudForbidden : chemin hors du périmètre autorisé à l'IA (allowlist). Levé à la frontière
// agent (démon) ; la fenêtre (humain) n'est pas soumise à l'allowlist.
var ErrCloudForbidden = errors.New("dossier hors du périmètre autorisé pour l'IA")

const maxUploadBytes = 128 << 20 // plafond d'un fichier envoyé (aligné sur le client WebDAV)

// cloudKeyID : identifiant du secret cloud au trousseau, dans un espace de noms distinct des
// instances git pour éviter toute collision de nom.
func cloudKeyID(name string) string { return "filecloud/" + name }

// FileClouds renvoie une copie des connexions cloud configurées (relecture disque).
func (s *Service) FileClouds() []config.FileCloud {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lockedReload()
	return append([]config.FileCloud(nil), s.cfg.FileClouds...)
}

// CloudResolveURL valide et NORMALISE une URL de cloud selon son type, SANS se connecter — pour le
// tunnel d'ajout (étape « vérifier l'URL »). Renvoie l'endpoint WebDAV réel ou une erreur guidée.
func (s *Service) CloudResolveURL(kind, rawURL, user string) (string, error) {
	kind = strings.TrimSpace(strings.ToLower(kind))
	if kind != config.CloudNextcloud && kind != config.CloudKDrive {
		return "", fmt.Errorf("type de cloud non pris en charge : %q", kind)
	}
	built, err := buildWebDAVBase(kind, rawURL, user)
	if err != nil {
		return "", err
	}
	return normalizeInstanceURL(built)
}

// AddFileCloud ajoute une connexion à un cloud de fichiers souverain (Nextcloud/kDrive) par
// identifiant + mot de passe (app-password). Valide la connexion AVANT de persister.
// Transactionnel : si la sauvegarde échoue, le secret est retiré du trousseau.
func (s *Service) AddFileCloud(name, kind, rawURL, user, password string) error {
	if err := s.assertWritable(); err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	kind = strings.TrimSpace(strings.ToLower(kind))
	user = strings.TrimSpace(user)
	password = strings.TrimSpace(password)
	if name == "" || user == "" || password == "" {
		return errors.New("nom, identifiant et mot de passe sont requis")
	}
	if kind != config.CloudNextcloud && kind != config.CloudKDrive {
		return fmt.Errorf("type de cloud non pris en charge : %q (seuls « nextcloud » et « kdrive » sont souverains)", kind)
	}
	// Construit l'endpoint WebDAV RÉEL selon le type de cloud : les utilisateurs collent souvent
	// l'adresse de leur navigateur (page de login, tableau de bord…), pas l'URL WebDAV. On la
	// reconstruit (Nextcloud → /remote.php/dav/files/<user> ; kDrive → origine de l'ID).
	built, err := buildWebDAVBase(kind, rawURL, user)
	if err != nil {
		return err
	}
	clean, err := normalizeInstanceURL(built) // même garde-fou que git : https, http refusé hors local/privé
	if err != nil {
		return err
	}

	// Test de connexion AVANT de persister quoi que ce soit. On joint l'URL RÉELLEMENT testée au
	// message : la cause la plus fréquente est une adresse qui ne répond pas en WebDAV (mauvais type,
	// URL de l'app web au lieu de l'endpoint…).
	ctx, cancel := opCtx()
	defer cancel()
	if err := webdav.New(clean, user, password, s.version).TestConnection(ctx); err != nil {
		return fmt.Errorf("échec de connexion WebDAV à %s — %w", clean, err)
	}

	dek, err := s.sessionDEK()
	if err != nil {
		return err
	}
	defer zero(dek)

	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.FindCloud(name) != nil {
		return fmt.Errorf("une connexion cloud « %s » existe déjà", name)
	}
	kr, err := keyring.Load()
	if err != nil {
		return err
	}
	if err := kr.SetToken(dek, cloudKeyID(name), clean, nil, password); err != nil {
		return fmt.Errorf("écriture du secret dans le trousseau impossible : %w", err)
	}
	fc := config.FileCloud{Name: name, Kind: kind, URL: clean, User: user, Default: len(cfg.FileClouds) == 0}
	cfg.FileClouds = append(cfg.FileClouds, fc)
	s.resignPolicy(cfg) // la policy inclut cette connexion (fermeture P3)
	if err := cfg.Save(); err != nil {
		_ = kr.DeleteToken(cloudKeyID(name)) // rollback trousseau
		return err
	}
	s.publish(cfg)
	return nil
}

// RemoveFileCloud supprime une connexion cloud (et son secret du trousseau).
func (s *Service) RemoveFileCloud(name string) error {
	if err := s.assertWritable(); err != nil {
		return err
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	real, ok := cfg.RemoveCloud(name)
	if !ok {
		return ErrNoCloud
	}
	s.resignPolicy(cfg) // la policy ne référence plus cette connexion (fermeture P3)
	if err := cfg.Save(); err != nil {
		return err
	}
	if kr, err := keyring.Load(); err == nil {
		_ = kr.DeleteToken(cloudKeyID(real)) // idempotent, pas de secret
	}
	s.publish(cfg)
	return nil
}

// SetDefaultCloud marque une connexion cloud comme défaut.
func (s *Service) SetDefaultCloud(name string) error {
	if err := s.assertWritable(); err != nil {
		return err
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if !cfg.SetDefaultCloud(name) {
		return ErrNoCloud
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}

// --- Politique PIN & allowlist — RÉGLAGE EN FENÊTRE UNIQUEMENT (non exposé socket/CLI/MCP) ---
// Doctrine : un agent ne doit jamais pouvoir ÉLARGIR ce qu'il voit ni ABAISSER la politique PIN.
// Seul l'humain, dans la fenêtre, décide des dossiers visibles par l'IA et de l'exigence de PIN.

// SetCloudAllowedDirs fixe la liste des dossiers que l'IA peut consulter sur cette connexion.
func (s *Service) SetCloudAllowedDirs(name string, dirs []string) error {
	if err := s.assertWritable(); err != nil {
		return err
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	fc := cfg.FindCloud(name)
	if fc == nil {
		return ErrNoCloud
	}
	seen := map[string]bool{}
	out := []string{}
	for _, d := range dirs {
		nd := config.CloudCleanPath(d)
		if nd == "" || seen[nd] {
			continue
		}
		seen[nd] = true
		out = append(out, nd)
	}
	fc.AllowedDirs = out
	s.resignPolicy(cfg) // re-signe la policy (fermeture P3)
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}

// SetCloudAccess fixe EN UN GESTE les autorisations (allow) et exclusions (deny) de l'IA pour une
// connexion — le modèle « arbre » de la fenêtre (v1.11). Les deux listes sont normalisées/dédupliquées.
// Réglage FENÊTRE UNIQUEMENT (non exposé socket/CLI/MCP).
func (s *Service) SetCloudAccess(name string, allowed, denied []string) error {
	if err := s.assertWritable(); err != nil {
		return err
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	fc := cfg.FindCloud(name)
	if fc == nil {
		return ErrNoCloud
	}
	fc.AllowedDirs = cleanCloudPaths(allowed)
	fc.DeniedDirs = cleanCloudPaths(denied)
	s.resignPolicy(cfg) // re-signe la policy (fermeture P3)
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}

// cleanCloudPaths normalise et déduplique une liste de chemins cloud (vides ignorés).
func cleanCloudPaths(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, d := range in {
		nd := config.CloudCleanPath(d)
		if nd == "" || seen[nd] {
			continue
		}
		seen[nd] = true
		out = append(out, nd)
	}
	return out
}

// SetCloudPINPolicy fixe la politique PIN d'une connexion cloud (nil = défauts).
func (s *Service) SetCloudPINPolicy(name string, pol *config.CloudPINPolicy) error {
	if err := s.assertWritable(); err != nil {
		return err
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	fc := cfg.FindCloud(name)
	if fc == nil {
		return ErrNoCloud
	}
	fc.PINPolicy = pol
	s.resignPolicy(cfg) // re-signe la policy (fermeture P3)
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}

// --- Autorisation ---------------------------------------------------------------------------

// RequiresPINCloud expose la politique PIN d'une connexion cloud. Fail-closed : cloud inconnu → PIN.
func (s *Service) RequiresPINCloud(name string, a config.CloudAction) bool {
	s.mu.Lock()
	s.lockedReload()
	cfg := s.cfg
	fc := cfg.ResolveCloud(name)
	ok := s.policyOK(cfg)
	s.mu.Unlock()
	if fc == nil || !ok {
		return true // fail-closed : cloud inconnu OU politique falsifiée → PIN exigé (fermeture P3)
	}
	return fc.RequiresPINCloud(a)
}

// authorizeCloud autorise une mutation cloud selon la politique de la connexion (même modèle que
// authorize() pour git). Lecture seule → refus ; politique sans PIN + session ouverte → ok ; sinon PIN.
func (s *Service) authorizeCloud(name string, a config.CloudAction, pin string) error {
	if s.ReadOnly() {
		return ErrReadOnly
	}
	if !s.RequiresPINCloud(name, a) {
		if s.Unlocked() {
			return nil
		}
		return ErrLocked
	}
	return s.gatePIN(pin)
}

// authorizeBackupCloud : autorisation DÉDIÉE aux sauvegardes. Un backup est une routine NON destructive
// (copie additive vers le cloud de l'utilisateur) qui doit « se faire toute seule » à l'ouverture, à la
// fermeture ou sur action manuelle — il ne demande JAMAIS le PIN, quelle que soit la politique du cloud.
// Il requiert seulement une session déverrouillée (nécessaire pour déchiffrer le jeton du cloud) et
// respecte le mode lecture seule (qui gèle tout envoi de façon délibérée).
func (s *Service) authorizeBackupCloud(name string) error {
	if s.ReadOnly() {
		return ErrReadOnly
	}
	if !s.Unlocked() {
		return ErrLocked
	}
	return nil
}

// CloudPathAllowed dit si un chemin est dans le périmètre autorisé à l'IA. Appelé à la frontière
// agent (démon). Fail-closed : cloud inconnu ou allowlist vide → refus.
func (s *Service) CloudPathAllowed(name, p string) bool {
	s.mu.Lock()
	s.lockedReload()
	cfg := s.cfg
	fc := cfg.ResolveCloud(name)
	ok := s.policyOK(cfg)
	s.mu.Unlock()
	if fc == nil || !ok {
		return false // fail-closed : cloud inconnu OU politique falsifiée → périmètre fermé (fermeture P3)
	}
	return fc.PathAllowed(p)
}

// CloudAllowedDirs renvoie les dossiers autorisés à l'IA pour une connexion (non secret). Permet à
// l'agent de connaître ses points d'entrée avant d'explorer.
func (s *Service) CloudAllowedDirs(name string) []string {
	s.mu.Lock()
	s.lockedReload()
	cfg := s.cfg
	fc := cfg.ResolveCloud(name)
	ok := s.policyOK(cfg)
	s.mu.Unlock()
	if fc == nil || !ok {
		return nil // fail-closed : politique falsifiée → aucun point d'entrée annoncé (fermeture P3)
	}
	return append([]string(nil), fc.AllowedDirs...)
}

// fileCloudClientFor construit un client WebDAV authentifié pour une connexion nommée (session
// déverrouillée requise : le mot de passe est déchiffré du trousseau).
func (s *Service) fileCloudClientFor(name string) (*webdav.Client, config.FileCloud, error) {
	s.mu.Lock()
	s.lockedReload()
	fcp := s.cfg.ResolveCloud(name)
	s.mu.Unlock()
	if fcp == nil {
		return nil, config.FileCloud{}, ErrNoCloud
	}
	fc := *fcp
	dek, err := s.sessionDEK()
	if err != nil {
		return nil, fc, err
	}
	defer zero(dek)
	kr, err := keyring.Load()
	if err != nil {
		return nil, fc, err
	}
	pass, err := kr.GetToken(dek, cloudKeyID(fc.Name))
	if err != nil {
		return nil, fc, err
	}
	return webdav.New(fc.URL, fc.User, pass, s.version), fc, nil
}

// CloudHealthItem : santé d'une connexion cloud (pour la pastille de santé en fenêtre, v1.11).
type CloudHealthItem struct {
	Name string `json:"name"`
	OK   bool   `json:"ok"`
	Err  string `json:"err,omitempty"`
}

// CloudHealth teste chaque connexion cloud (PROPFIND racine, best-effort). Renvoie ErrLocked si la
// session est verrouillée (le secret est alors indisponible) ; sinon chaque item porte son état.
func (s *Service) CloudHealth() ([]CloudHealthItem, error) {
	clouds := s.FileClouds()
	// Perf (1.17, Point 2) : chaque test est un aller-retour réseau indépendant, avec un délai de garde
	// par cloud (opCtx). En file, un seul cloud lent (jusqu'à l'expiration) retardait TOUS les suivants —
	// les pastilles de santé mettaient alors des dizaines de secondes à s'afficher. On teste en parallèle ;
	// chaque goroutine écrit SA case (pas de course), même modèle que boardFor.
	out := make([]CloudHealthItem, len(clouds))
	var mu sync.Mutex
	locked := false
	var wg sync.WaitGroup
	for i, fc := range clouds {
		wg.Add(1)
		go func(i int, fc config.FileCloud) {
			defer wg.Done()
			cl, _, err := s.fileCloudClientFor(fc.Name)
			if err != nil {
				if errors.Is(err, ErrLocked) {
					mu.Lock()
					locked = true
					mu.Unlock()
				}
				out[i] = CloudHealthItem{Name: fc.Name, OK: false, Err: err.Error()}
				return
			}
			ctx, cancel := opCtx()
			terr := cl.TestConnection(ctx)
			cancel()
			it := CloudHealthItem{Name: fc.Name, OK: terr == nil}
			if terr != nil {
				it.Err = terr.Error()
			}
			out[i] = it
		}(i, fc)
	}
	wg.Wait()
	if locked {
		// Session verrouillée : condition globale (tous les clouds) — on remonte ErrLocked comme avant.
		return nil, ErrLocked
	}
	return out, nil
}

// --- Lectures (session déverrouillée, pas de PIN) -------------------------------------------

// CloudList énumère le contenu d'un dossier cloud.
func (s *Service) CloudList(name, dir string) ([]webdav.Entry, error) {
	cl, _, err := s.fileCloudClientFor(name)
	if err != nil {
		return nil, err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.List(ctx, dir)
}

// CloudGet télécharge le contenu d'un fichier (octets).
func (s *Service) CloudGet(name, p string) ([]byte, error) {
	cl, _, err := s.fileCloudClientFor(name)
	if err != nil {
		return nil, err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.Get(ctx, p)
}

// CloudRead renvoie le contenu TEXTE d'un fichier (pour l'IA qui consulte un document).
func (s *Service) CloudRead(name, p string) (string, error) {
	b, err := s.CloudGet(name, p)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// CloudDownload télécharge un fichier cloud vers un chemin local du poste.
func (s *Service) CloudDownload(name, remotePath, localDest string) error {
	b, err := s.CloudGet(name, remotePath)
	if err != nil {
		return err
	}
	return os.WriteFile(localDest, b, 0o600)
}

// --- Mutations (politique PIN de la connexion) ----------------------------------------------

// CloudMkdir crée un dossier sur le cloud.
func (s *Service) CloudMkdir(name, p, pin string) error {
	if err := s.authorizeCloud(name, config.CloudWrite, pin); err != nil {
		return err
	}
	cl, _, err := s.fileCloudClientFor(name)
	if err != nil {
		return err
	}
	ctx, cancel := opCtx()
	defer cancel()
	if err := cl.Mkcol(ctx, p); err != nil {
		return err
	}
	return nil
}

// CloudUpload envoie un fichier LOCAL du poste vers un dossier cloud (nom de base conservé).
func (s *Service) CloudUpload(name, localPath, remoteDir, pin string) error {
	if err := s.authorizeCloud(name, config.CloudWrite, pin); err != nil {
		return err
	}
	fi, err := os.Stat(localPath)
	if err != nil {
		return err
	}
	if fi.IsDir() {
		return errors.New("l'import d'un dossier n'est pas pris en charge (sélectionnez des fichiers)")
	}
	if fi.Size() > maxUploadBytes {
		return webdav.ErrTooLarge
	}
	content, err := os.ReadFile(localPath)
	if err != nil {
		return err
	}
	remote := path.Join("/"+strings.Trim(config.CloudCleanPath(remoteDir), "/"), filepathBase(localPath))
	cl, _, err := s.fileCloudClientFor(name)
	if err != nil {
		return err
	}
	ctx, cancel := opCtx()
	defer cancel()
	if err := cl.Put(ctx, remote, content); err != nil {
		return err
	}
	return nil
}

// CloudWrite écrit (crée ou remplace) un fichier cloud à partir d'un contenu fourni — notamment
// l'IA qui dépose un résumé .md dans le CORPUS.
func (s *Service) CloudWrite(name, remotePath, content, pin string) error {
	if err := s.authorizeCloud(name, config.CloudWrite, pin); err != nil {
		return err
	}
	cl, _, err := s.fileCloudClientFor(name)
	if err != nil {
		return err
	}
	ctx, cancel := opCtx()
	defer cancel()
	if err := cl.Put(ctx, remotePath, []byte(content)); err != nil {
		return err
	}
	return nil
}

// CloudDelete supprime un fichier ou un dossier cloud (geste destructif → PIN par défaut).
func (s *Service) CloudDelete(name, p, pin string) error {
	if err := s.authorizeCloud(name, config.CloudDelete, pin); err != nil {
		return err
	}
	cl, _, err := s.fileCloudClientFor(name)
	if err != nil {
		return err
	}
	ctx, cancel := opCtx()
	defer cancel()
	if err := cl.Delete(ctx, p); err != nil {
		return err
	}
	return nil
}

// filepathBase renvoie le nom de base d'un chemin local (séparateurs OS ou POSIX).
func filepathBase(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	return path.Base(p)
}
