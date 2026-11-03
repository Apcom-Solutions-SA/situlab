// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"situlab/internal/config"
	"situlab/internal/forgejo"
	"situlab/internal/keyring"
	"strings"
	"time"
)

// Instances renvoie une copie des instances configurées (relecture disque).
func (s *Service) Instances() []config.Instance {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lockedReload()
	return append([]config.Instance(nil), s.cfg.Instances...)
}

// AddInstance ajoute une instance Forgejo/Gitea par jeton collé (compat historique, CLI).
func (s *Service) AddInstance(name, rawURL, token string) error {
	return s.AddInstanceToken(name, "forgejo", rawURL, token)
}

// AddInstanceToken ajoute une instance par JETON déjà en main (Forgejo/Gitea ou GitHub).
// Pour GitHub, l'utilisateur colle un Personal Access Token (SituLab ne peut pas en créer
// via l'API) ; l'URL web est fixée à https://github.com. Valide la connexion AVANT de
// persister. Transactionnel : si la sauvegarde échoue, le jeton est retiré du trousseau.
func (s *Service) AddInstanceToken(name, provider, rawURL, token string) error {
	if err := s.assertWritable(); err != nil { // ajout de connexion → refusé en lecture seule
		return err
	}
	name = strings.TrimSpace(name)
	token = strings.TrimSpace(token)
	provider = strings.TrimSpace(provider)
	if name == "" || token == "" {
		return errors.New("nom et jeton sont requis")
	}

	var clean string
	kind := forgejo.KindForgejo
	if provider == "github" {
		clean = "https://github.com"
		kind = forgejo.KindGitHub
	} else {
		provider = "forgejo"
		var err error
		if clean, err = normalizeInstanceURL(rawURL); err != nil {
			return err
		}
	}

	// Test de connexion AVANT de persister quoi que ce soit.
	ctx, cancel := opCtx()
	defer cancel()
	if _, err := forgejo.NewKind(clean, token, s.version, kind).CurrentUser(ctx); err != nil {
		return err
	}

	// Le token se range dans le trousseau chiffré : il faut une session déverrouillée.
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
	if cfg.Find(name) != nil {
		return fmt.Errorf("une instance « %s » existe déjà", name)
	}
	kr, err := keyring.Load()
	if err != nil {
		return err
	}
	if err := kr.SetToken(dek, name, clean, nil, token); err != nil {
		return fmt.Errorf("écriture du token dans le trousseau impossible : %w", err)
	}
	inst := config.Instance{Name: name, URL: clean, Default: len(cfg.Instances) == 0}
	if provider == "github" {
		inst.Provider = "github"
	}
	cfg.Instances = append(cfg.Instances, inst)
	s.resignPolicy(cfg) // la policy inclut cette connexion (fermeture P3)
	if err := cfg.Save(); err != nil {
		_ = kr.DeleteToken(name) // rollback trousseau (l'état mémoire n'a pas bougé)
		return err
	}
	s.publish(cfg)
	return nil
}

// EditInstance modifie une connexion git EXISTANTE (R6/R7, 1.18.3) : URL et, optionnellement, jeton
// (vide = on garde le jeton actuel). Le NOM reste l'identifiant du trousseau — pas de renommage, donc
// aucun risque d'orphelin. Teste la connexion AVANT de persister : en cas d'échec, rien n'est modifié.
func (s *Service) EditInstance(name, newURL, newToken string) error {
	if err := s.assertWritable(); err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	newToken = strings.TrimSpace(newToken)
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
	inst := cfg.Find(name)
	if inst == nil {
		return ErrNoInstance
	}
	kr, err := keyring.Load()
	if err != nil {
		return err
	}
	clean := inst.URL
	kind := forgejo.KindForgejo
	if inst.Provider == "github" {
		clean = "https://github.com" // GitHub : URL fixe, non modifiable
		kind = forgejo.KindGitHub
	} else if strings.TrimSpace(newURL) != "" {
		if clean, err = normalizeInstanceURL(newURL); err != nil {
			return err
		}
	}
	token := newToken
	if token == "" {
		if token, err = kr.GetToken(dek, name); err != nil || token == "" {
			return errors.New("jeton actuel introuvable ; fournissez un nouveau jeton")
		}
	}
	ctx, cancel := opCtx()
	defer cancel()
	if _, err := forgejo.NewKind(clean, token, s.version, kind).CurrentUser(ctx); err != nil {
		return err
	}
	if err := kr.SetToken(dek, name, clean, nil, token); err != nil {
		return fmt.Errorf("écriture du token dans le trousseau impossible : %w", err)
	}
	for i := range cfg.Instances {
		if cfg.Instances[i].Name == name {
			cfg.Instances[i].URL = clean
		}
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}

// DefaultTokenScopes : portées du token créé à l'onboarding (profil Standard, décision
// gouvernance 2026-10-08). Un write: inclut le read: correspondant. write:organization est
// nécessaire pour créer un dépôt dans une organisation (beaucoup de serveurs interdisent
// l'espace personnel et imposent les orgs).
var DefaultTokenScopes = []string{"write:repository", "write:issue", "write:organization", "read:user", "read:misc"}

// tokenName nomme le token créé par SituLab de façon identifiable/révocable côté Forgejo.
// Inclut l'instance pour éviter les collisions de nom (Forgejo impose l'unicité par compte)
// quand plusieurs instances SituLab visent le même compte.
func tokenName(instance string) string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "poste"
	}
	instance = strings.TrimSpace(instance)
	if instance == "" {
		return "SituLab@" + host
	}
	return "SituLab@" + host + "/" + instance
}

// AddInstanceLogin réalise l'onboarding « PIN-only » : à partir d'un identifiant + mot de
// passe (+ code 2FA si le compte en a un), SituLab CRÉE lui-même un token via l'API Forgejo,
// le vérifie, puis le chiffre dans le trousseau. Le mot de passe et l'OTP ne sont jamais
// stockés. Ensuite, seul le PIN est redemandé. Nécessite une session déverrouillée.
func (s *Service) AddInstanceLogin(name, rawURL, user, password, otp string, adminScope, remember bool) error {
	if err := s.assertWritable(); err != nil { // crée un jeton côté serveur → mutation, refusée en lecture seule
		return err
	}
	name = strings.TrimSpace(name)
	user = strings.TrimSpace(user)
	if name == "" || rawURL == "" || user == "" || password == "" {
		return errors.New("nom, url, identifiant et mot de passe sont requis")
	}
	clean, err := normalizeInstanceURL(rawURL)
	if err != nil {
		return err
	}
	// Le token se range chiffré : session requise (le PIN a normalement été défini juste avant).
	dek, err := s.sessionDEK()
	if err != nil {
		return err
	}
	defer zero(dek)

	// Vérifier le doublon AVANT tout appel réseau : sinon on créerait un token sur le
	// serveur pour ensuite refuser l'instance → token orphelin à portée d'écriture.
	if cfg0, err := config.Load(); err == nil && cfg0.Find(name) != nil {
		return fmt.Errorf("une instance « %s » existe déjà", name)
	}

	ctx, cancel := opCtx()
	defer cancel()
	tname := tokenName(name)
	// Portées : Standard, + admin si le compte est administrateur et que l'utilisateur l'a
	// demandé (nécessaire pour gérer les utilisateurs depuis SituLab).
	scopes := append([]string(nil), DefaultTokenScopes...)
	if adminScope {
		scopes = append(scopes, "write:admin")
	}
	// Création du token par l'API (auth basique + OTP), sans client à token préalable.
	cl := forgejo.New(clean, "", s.version)
	// Idempotence : si un jeton du même nom traîne côté serveur (ex. instance supprimée
	// localement sans révoquer le jeton distant), on le retire d'abord — sinon Forgejo
	// répond « access token name has been used already ». Best-effort.
	_ = cl.TokenDeleteByName(ctx, user, password, otp, tname)
	token, err := cl.TokenCreate(ctx, user, password, otp, tname, scopes, nil)
	if err != nil {
		return err
	}
	// Vérification immédiate avec le token fraîchement créé.
	if _, err := forgejo.New(clean, token, s.version).CurrentUser(ctx); err != nil {
		s.revokeOrphan(clean, user, password, otp, tname)
		return fmt.Errorf("token créé mais vérification échouée : %w", err)
	}

	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		s.revokeOrphan(clean, user, password, otp, tname)
		return err
	}
	if cfg.Find(name) != nil { // course : instance apparue entre-temps
		s.revokeOrphan(clean, user, password, otp, tname)
		return fmt.Errorf("une instance « %s » existe déjà", name)
	}
	kr, err := keyring.Load()
	if err != nil {
		s.revokeOrphan(clean, user, password, otp, tname)
		return err
	}
	if err := kr.SetToken(dek, name, clean, scopes, token); err != nil {
		s.revokeOrphan(clean, user, password, otp, tname)
		return fmt.Errorf("écriture du token dans le trousseau impossible : %w", err)
	}
	// Mémorisation du mot de passe UNIQUEMENT sur choix explicite de l'utilisateur (case à
	// cocher), chiffré par le PIN : la gestion des jetons ne le redemande alors plus (un compte
	// 2FA redemande seulement le code du moment). Sinon, rien n'est conservé. Best-effort.
	if remember {
		_ = kr.SetPassword(dek, name, password)
	}
	cfg.Instances = append(cfg.Instances, config.Instance{Name: name, URL: clean, Default: len(cfg.Instances) == 0, TwoFA: otp != "", Remember: remember})
	s.resignPolicy(cfg) // la policy inclut cette connexion (fermeture P3)
	if err := cfg.Save(); err != nil {
		_ = kr.DeleteToken(name)
		s.revokeOrphan(clean, user, password, otp, tname)
		return err
	}
	s.publish(cfg)
	return nil
}

// revokeOrphan supprime (best-effort) un token créé côté serveur mais que SituLab n'a pas
// pu enregistrer localement, pour ne pas laisser de token orphelin à portée d'écriture.
func (s *Service) revokeOrphan(url, user, password, otp, name string) {
	ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
	defer cancel()
	_ = forgejo.New(url, "", s.version).TokenDeleteByName(ctx, user, password, otp, name)
}

// publish remplace l'état en mémoire par une config fraîche (section critique courte).
func (s *Service) publish(cfg *config.Config) {
	s.mu.Lock()
	s.cfg = cfg
	s.mu.Unlock()
}

// RemoveInstance supprime une instance, son token et son cache.
func (s *Service) RemoveInstance(name string) error {
	if err := s.assertWritable(); err != nil { // retire une instance et son jeton du trousseau → refusé en lecture seule
		return err
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	real, ok := cfg.Remove(name)
	if !ok {
		return ErrNoInstance
	}
	if kr, err := keyring.Load(); err == nil {
		_ = kr.DeleteToken(real) // trousseau présent : on retire l'entrée
	}
	s.resignPolicy(cfg) // la policy ne référence plus cette connexion (fermeture P3)
	if err := cfg.Save(); err != nil {
		return err
	}
	s.mu.Lock()
	s.cfg = cfg
	delete(s.repoCache, real)
	s.mu.Unlock()
	return nil
}

// SetDefaultInstance définit l'instance par défaut.
func (s *Service) SetDefaultInstance(name string) error {
	if err := s.assertWritable(); err != nil { // mutation de config : refusée en lecture seule (comme SetDefaultCloud)
		return err
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if !cfg.SetDefault(name) {
		return ErrNoInstance
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}

// SetInstanceRole fixe le rôle d'une connexion (atelier|production). Réglage local (pas de PIN) ;
// n'est PAS exposé au socket/CLI/MCP (seule la fenêtre le modifie, comme le mode lecture seule).
func (s *Service) SetInstanceRole(name, role string) error {
	if role != config.RoleAtelier && role != config.RoleProduction {
		return fmt.Errorf("rôle inconnu : %q (atelier|production)", role)
	}
	if err := s.assertPolicyIntact(); err != nil { // refus de re-signer par-dessus une policy falsifiée
		return err
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	in := cfg.Find(name)
	if in == nil {
		return ErrNoInstance
	}
	in.Role = role
	s.resignPolicy(cfg) // le rôle décide des défauts de PIN (fermeture P3)
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}

// SetInstancePINPolicy remplace la politique PIN d'une connexion (nil = revenir aux défauts du
// rôle). Réglage local, fenêtre uniquement.
func (s *Service) SetInstancePINPolicy(name string, p *config.PINPolicy) error {
	if err := s.assertPolicyIntact(); err != nil { // refus de re-signer par-dessus une policy falsifiée
		return err
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	in := cfg.Find(name)
	if in == nil {
		return ErrNoInstance
	}
	in.PINPolicy = p
	s.resignPolicy(cfg) // politique PIN de la connexion (fermeture P3)
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}

// TestInstance vérifie la connexion et renvoie l'utilisateur.
func (s *Service) TestInstance(name string) (*forgejo.User, error) {
	return s.Whoami(name)
}

// --- Lectures distantes ---

// Whoami renvoie l'utilisateur courant d'une instance.
func (s *Service) Whoami(instance string) (*forgejo.User, error) {
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return nil, err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.CurrentUser(ctx)
}

// ActiveTokenName renvoie le nom EXACT du token que SituLab utilise pour une instance
// (`SituLab@<poste>/<instance>`), pour que l'UI ne protège QUE celui-là (les autres
// `SituLab@…` sont des résidus supprimables).
func (s *Service) ActiveTokenName(instance string) string {
	s.mu.Lock()
	in := s.cfg.Resolve(instance)
	s.mu.Unlock()
	name := instance
	if in != nil {
		name = in.Name
	}
	return tokenName(name)
}

// cachedRepos renvoie les dépôts distants, avec un cache court par instance pour
// éviter de marteler l'API pendant le rafraîchissement live. Copie défensive renvoyée.
func (s *Service) cachedRepos(ctx context.Context, instance string, cl *forgejo.Client) ([]forgejo.Repo, bool, error) {
	s.mu.Lock()
	if e, ok := s.repoCache[instance]; ok && time.Since(e.at) < repoCacheTTL {
		cp := append([]forgejo.Repo(nil), e.repos...)
		trunc := e.trunc
		s.mu.Unlock()
		return cp, trunc, nil
	}
	s.mu.Unlock()

	repos, trunc, err := cl.ListMyRepos(ctx)
	if err != nil {
		return nil, false, err
	}
	s.mu.Lock()
	s.repoCache[instance] = repoCache{repos: repos, trunc: trunc, at: time.Now()}
	s.mu.Unlock()
	return append([]forgejo.Repo(nil), repos...), trunc, nil
}

// --- Local (git) ---

// storedPassword renvoie le mot de passe mémorisé d'une instance (chiffré par le PIN),
// via la session déverrouillée ("" si aucun).
func (s *Service) storedPassword(instance string) string {
	dek, err := s.sessionDEK()
	if err != nil {
		return ""
	}
	defer zero(dek)
	kr, err := keyring.Load()
	if err != nil {
		return ""
	}
	s.mu.Lock()
	in := s.cfg.Resolve(instance)
	s.mu.Unlock()
	id := instance
	if in != nil {
		id = in.Name
	}
	p, _ := kr.GetPassword(dek, id)
	return p
}

// ReconnectInstance recrée le token d'une instance existante (même nom) : retire l'ancien
// token serveur, en crée un nouveau (avec la portée admin si demandée) et le chiffre. Le mot de
// passe n'est mémorisé que si remember est vrai (sinon toute trace est purgée). Permet de passer
// une instance en admin ou de réparer l'accès sans la supprimer puis la ré-ajouter. Nécessite une
// session déverrouillée.
func (s *Service) ReconnectInstance(name, user, password, otp string, adminScope, remember bool) error {
	if err := s.assertWritable(); err != nil { // recrée un jeton côté serveur → refusé en lecture seule
		return err
	}
	name = strings.TrimSpace(name)
	user = strings.TrimSpace(user)
	if user == "" || password == "" {
		return errors.New("identifiant et mot de passe requis")
	}
	dek, err := s.sessionDEK()
	if err != nil {
		return err
	}
	defer zero(dek)
	cfg0, err := config.Load()
	if err != nil {
		return err
	}
	inst := cfg0.Find(name)
	if inst == nil {
		return ErrNoInstance
	}
	clean := inst.URL
	ctx, cancel := opCtx()
	defer cancel()
	cl := forgejo.New(clean, "", s.version)
	tname := tokenName(name)
	_ = cl.TokenDeleteByName(ctx, user, password, otp, tname) // retire l'ancien (même nom, sinon collision)
	scopes := append([]string(nil), DefaultTokenScopes...)
	if adminScope {
		scopes = append(scopes, "write:admin")
	}
	token, err := cl.TokenCreate(ctx, user, password, otp, tname, scopes, nil)
	if err != nil {
		return err
	}
	if _, err := forgejo.New(clean, token, s.version).CurrentUser(ctx); err != nil {
		s.revokeOrphan(clean, user, password, otp, tname)
		return fmt.Errorf("token créé mais vérification échouée : %w", err)
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if ci := cfg.Find(name); ci != nil {
		ci.TwoFA = otp != ""
		ci.Remember = remember
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	kr, err := keyring.Load()
	if err != nil {
		return err
	}
	if err := kr.SetToken(dek, name, clean, scopes, token); err != nil {
		return err
	}
	// Mot de passe mémorisé seulement sur choix explicite (sinon purge) : la reconnexion ne doit
	// pas re-mémoriser en douce un mot de passe refusé (contournait S3 sur le chemin de réparation
	// imposé après une réinitialisation du PIN).
	if remember {
		_ = kr.SetPassword(dek, name, password)
	} else {
		_ = kr.SetPassword(dek, name, "")
	}
	return nil
}

// ReconnectToken remet un JETON pour une instance existante (cas GitHub : l'utilisateur colle
// un nouveau Personal Access Token). Teste la connexion puis ré-chiffre le jeton. Nécessite une
// session déverrouillée. Pour Forgejo, préférer ReconnectInstance (recrée le jeton par l'API).
func (s *Service) ReconnectToken(name, token string) error {
	if err := s.assertWritable(); err != nil { // reconnexion = changement de connexion → refusé en lecture seule
		return err
	}
	name = strings.TrimSpace(name)
	token = strings.TrimSpace(token)
	if token == "" {
		return errors.New("jeton requis")
	}
	dek, err := s.sessionDEK()
	if err != nil {
		return err
	}
	defer zero(dek)
	cfg0, err := config.Load()
	if err != nil {
		return err
	}
	inst := cfg0.Find(name)
	if inst == nil {
		return ErrNoInstance
	}
	ctx, cancel := opCtx()
	defer cancel()
	if _, err := forgejo.NewKind(inst.URL, token, s.version, providerKind(*inst)).CurrentUser(ctx); err != nil {
		return err
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	kr, err := keyring.Load()
	if err != nil {
		return err
	}
	if err := kr.SetToken(dek, name, inst.URL, nil, token); err != nil {
		return err
	}
	return nil
}

// RememberPassword marque la 2FA d'une instance et, SI l'utilisateur a choisi de mémoriser son
// mot de passe à l'ajout (config.Instance.Remember), le conserve chiffré par le PIN pour que la
// gestion des jetons ne le redemande plus. Si ce choix était « ne pas mémoriser », rien n'est
// conservé (et toute trace est purgée). Appelé au 1er déverrouillage réussi du gestionnaire.
func (s *Service) RememberPassword(instance, password string, twoFA bool) error {
	if err := s.assertWritable(); err != nil { // persiste un secret + TwoFA : mutation, refusée en lecture seule
		return err
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
	in := cfg.Find(instance)
	if in == nil {
		return ErrNoInstance
	}
	id := in.Name
	remember := in.Remember
	in.TwoFA = twoFA
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	kr, kerr := keyring.Load()
	if kerr != nil {
		return kerr
	}
	// On n'écrit (ou ne purge) le mot de passe que selon le choix explicite fait à l'ajout de
	// l'instance (config.Instance.Remember). Honorer ce choix ici empêche de re-mémoriser en
	// douce, au premier déverrouillage du gestionnaire de jetons, un mot de passe que
	// l'utilisateur avait refusé de conserver (ce qui contournait S3).
	if !remember {
		_ = kr.SetPassword(dek, id, "") // opt-out : garantir qu'aucun mot de passe ne subsiste
		return nil
	}
	if password != "" {
		return kr.SetPassword(dek, id, password)
	}
	return nil
}

// normalizeInstanceURL valide et nettoie l'URL d'une instance (schéma http/https,
// hôte présent et en minuscules, sans slash final).
func normalizeInstanceURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", errors.New("URL d'instance invalide")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", errors.New("l'URL doit être en http(s)")
	}
	// http:// expose le jeton EN CLAIR sur le réseau : on ne l'autorise que pour un hôte local
	// ou privé (poste de dev, réseau interne). Un serveur public DOIT être en https.
	if u.Scheme == "http" && !isLocalOrPrivateHost(u.Hostname()) {
		return "", errors.New("adresse en http:// refusée (le jeton circulerait en clair), utilisez https://, sauf pour un serveur local/privé")
	}
	host := strings.ToLower(u.Host)
	return strings.TrimRight(u.Scheme+"://"+host+u.Path, "/"), nil
}

// isLocalOrPrivateHost indique si l'hôte est local (localhost) ou dans une plage privée /
// loopback / link-local — seuls cas où http:// (sans TLS) est toléré. On n'accepte que le nom
// « localhost » EXACT et les IP : un simple suffixe « .localhost » ne suffit pas (il pourrait être
// détourné par un résolveur ou un fichier hosts vers une IP publique, et le jeton partirait alors
// en clair). Pour un hôte local nommé autrement, utiliser son IP ou https://.
func isLocalOrPrivateHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}
