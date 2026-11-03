// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"situlab/internal/config"
	"situlab/internal/kdrive"
	"situlab/internal/keyring"
	"situlab/internal/webdav"
)

// pathUnder dit si p est EGAL a root ou situe dessous (apres nettoyage). Fail-closed sur erreur.
func pathUnder(p, root string) bool {
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".." && !filepath.IsAbs(rel))
}

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

// resolveKDriveID déduit l'ID numérique du drive depuis une saisie libre : un ID nu (« 3869957 »), ou
// une URL collée (app web ksuite/kdrive). On NE concatène PAS les chiffres : on prend le segment après
// « /drive/ » (ou le dernier segment numérique), via kdriveID. Chaîne vide si rien d'exploitable.
func resolveKDriveID(raw string) string {
	raw = strings.TrimSpace(raw)
	if isDigits(raw) {
		return raw
	}
	s := raw
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	if u, err := url.Parse(s); err == nil && u.Host != "" {
		if id := kdriveID(u.Host, u.Path); id != "" {
			return id
		}
		// Forme endpoint « <ID>.connect.kdrive.infomaniak.com » : l'ID est le 1er label de l'hôte.
		if dot := strings.IndexByte(u.Host, '.'); dot > 0 && isDigits(u.Host[:dot]) {
			return u.Host[:dot]
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
		return fmt.Errorf("échec de connexion WebDAV à %s : %w", clean, err)
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

// EditFileCloud modifie une connexion cloud WebDAV EXISTANTE (R6, 1.18.3) : URL, identifiant et,
// optionnellement, mot de passe (vide = on garde l'actuel). Nom stable (identifiant du trousseau).
// Teste la connexion AVANT de persister. Les clouds REST (kDrive par jeton) ne passent pas par ici.
func (s *Service) EditFileCloud(name, rawURL, user, newPassword string) error {
	if err := s.assertWritable(); err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	user = strings.TrimSpace(user)
	newPassword = strings.TrimSpace(newPassword)
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
	fc := cfg.FindCloud(name)
	if fc == nil {
		return fmt.Errorf("connexion cloud « %s » introuvable", name)
	}
	if fc.Method == "rest" {
		return errors.New("ce cloud est connecté par jeton (REST) : retirez-le puis reconnectez-le pour le modifier")
	}
	if user == "" {
		return errors.New("l'identifiant est requis")
	}
	built, err := buildWebDAVBase(fc.Kind, rawURL, user)
	if err != nil {
		return err
	}
	clean, err := normalizeInstanceURL(built)
	if err != nil {
		return err
	}
	kr, err := keyring.Load()
	if err != nil {
		return err
	}
	password := newPassword
	if password == "" {
		if password, err = kr.GetToken(dek, cloudKeyID(name)); err != nil || password == "" {
			return errors.New("mot de passe actuel introuvable ; saisissez-en un nouveau")
		}
	}
	ctx, cancel := opCtx()
	defer cancel()
	if err := webdav.New(clean, user, password, s.version).TestConnection(ctx); err != nil {
		return fmt.Errorf("échec de connexion WebDAV à %s : %w", clean, err)
	}
	if err := kr.SetToken(dek, cloudKeyID(name), clean, nil, password); err != nil {
		return fmt.Errorf("écriture du secret dans le trousseau impossible : %w", err)
	}
	for i := range cfg.FileClouds {
		if cfg.FileClouds[i].Name == name {
			cfg.FileClouds[i].URL = clean
			cfg.FileClouds[i].User = user
		}
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}

// AddKDriveREST connecte un kDrive via l'API REST avec un JETON API personnel (scope « drive »). Teste
// le jeton sur le drive AVANT de persister. Donne accès aux « partagés avec moi » (absents du WebDAV).
// FENÊTRE uniquement. v1.18.2.
func (s *Service) AddKDriveREST(name, driveID, token, email string) error {
	if err := s.assertWritable(); err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	token = strings.TrimSpace(token)
	// L'utilisateur peut coller l'URL de l'app web (ksuite.infomaniak.com/<compte>/kdrive/app/drive/<ID>)
	// ou juste l'ID. onlyDigits() concaténait TOUS les chiffres (compte + drive) → mauvais drive → « Access
	// denied ». On extrait l'ID réel : le segment juste après « /drive/ » (kdriveID), sinon le nombre brut.
	drive := resolveKDriveID(driveID)
	email = strings.TrimSpace(email)
	if name == "" || token == "" || drive == "" {
		return errors.New("nom, identifiant du drive et jeton API sont requis")
	}
	ctx, cancel := opCtx()
	defer cancel()
	if err := kdrive.New(drive, token, s.version).TestConnection(ctx); err != nil {
		return fmt.Errorf("échec de connexion à l'API kDrive (jeton de scope « drive » requis) : %w", err)
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
	if err := kr.SetToken(dek, cloudKeyID(name), "https://api.infomaniak.com", nil, token); err != nil {
		return fmt.Errorf("écriture du secret dans le trousseau impossible : %w", err)
	}
	fc := config.FileCloud{Name: name, Kind: config.CloudKDrive, Method: "rest", URL: "https://api.infomaniak.com", DriveID: drive, User: email, Default: len(cfg.FileClouds) == 0}
	cfg.FileClouds = append(cfg.FileClouds, fc)
	s.resignPolicy(cfg)
	if err := cfg.Save(); err != nil {
		_ = kr.DeleteToken(cloudKeyID(name))
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

// ErrCloudOperationForbidden : opération formellement interdite à l'IA sur ce chemin (niveau
// « interdiction », v1.18.2). Distincte d'ErrCloudForbidden (hors allowlist/visibilité). À la
// frontière agent (démon), refusée AVANT tout prompt. En fenêtre, l'humain reste l'autorité.
var ErrCloudOperationForbidden = errors.New("opération interdite sur ce chemin par la politique de droits")

// resolveCloud lit une connexion et l'état d'intégrité de la politique sous verrou.
func (s *Service) resolveCloud(name string) (*config.FileCloud, bool) {
	s.mu.Lock()
	s.lockedReload()
	cfg := s.cfg
	fc := cfg.ResolveCloud(name)
	ok := s.policyOK(cfg)
	s.mu.Unlock()
	return fc, fc != nil && ok
}

// cloudFS : surface commune WebDAV / kDrive REST (v1.18.2). Le cœur route les gestes de fichiers vers
// le bon transport selon la méthode de la connexion, sans dupliquer la logique.
type cloudFS interface {
	List(ctx context.Context, dir string) ([]webdav.Entry, error)
	Get(ctx context.Context, p string) ([]byte, error)
	Put(ctx context.Context, p string, content []byte) error
	Mkcol(ctx context.Context, p string) error
	Delete(ctx context.Context, p string) error
	Move(ctx context.Context, from, to string) error
	TestConnection(ctx context.Context) error
}

// fileCloudClientFor construit un client authentifié (WebDAV ou kDrive REST selon la méthode) pour
// une connexion nommée (session déverrouillée requise : le secret est déchiffré du trousseau).
func (s *Service) fileCloudClientFor(name string) (cloudFS, config.FileCloud, error) {
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
	// Mode REST (kDrive, v1.18.2) : le secret est un JETON API ; l'accès passe par api.infomaniak.com
	// (HTTPS en dur). L'ID du drive vit dans DriveID (ou, par tolérance, dans URL).
	if fc.AccessMethod() == "rest" {
		driveID := strings.TrimSpace(fc.DriveID)
		if driveID == "" {
			driveID = fc.URL
		}
		return kdrive.New(driveID, pass, s.version), fc, nil
	}
	// Mode WebDAV (défaut) : défense en profondeur, la DESTINATION du mot de passe d'application doit
	// rester HTTPS, revérifié à l'usage (bloque une URL réécrite en http:// → fuite d'identifiant).
	if u, perr := url.Parse(strings.TrimSpace(fc.URL)); perr != nil || u.Scheme != "https" {
		return nil, fc, fmt.Errorf("adresse cloud non sécurisée (HTTPS requis) : %q", fc.URL)
	}
	return webdav.New(fc.URL, fc.User, pass, s.version), fc, nil
}
