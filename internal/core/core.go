// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

// Package core est le noyau métier de SituLab. Il ne connaît ni la fenêtre ni le
// terminal : la GUI (Wails) et la CLI (Cobra) l'appellent à l'identique, ce qui
// garantit la parité GUI <-> CLI. Toute capacité de SituLab existe ici.
//
// Concurrence : Wails invoque le Service depuis plusieurs goroutines.
//   - s.mu protège l'état en mémoire (cfg + repoCache) ; on le tient le temps de
//     lire/copier/publier, jamais pendant un appel réseau, git ou coffre.
//   - s.ioMu sérialise les MUTATIONS persistantes (coffre + écriture disque), qui
//     peuvent bloquer (invite du trousseau) : elles se font hors s.mu, puis publient
//     le nouvel état en mémoire sous un court s.mu.
package core

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"situlab/internal/attest"
	"situlab/internal/config"
	"situlab/internal/forgejo"
	"situlab/internal/gitx"
	"situlab/internal/keyring"
)

// PINPromptTTL est la SOURCE UNIQUE du délai de garde d'une saisie de PIN (fenêtre de confirmation
// déclenchée par un agent). Tout le reste en découle — le garde-fou du prompteur, le compte à rebours
// affiché, le message d'expiration, l'indice CLI — pour qu'aucune durée ne soit recopiée en dur ailleurs
// (v1.17, besoin #6). PINPromptTTLSeconds en donne la valeur entière, prête à insérer dans un texte.
const PINPromptTTL = 2 * time.Minute

// PINPromptTTLSeconds : le TTL en secondes (entier), pour les messages (« max 120 s »).
func PINPromptTTLSeconds() int { return int(PINPromptTTL.Seconds()) }

// Erreurs sentinelles mappées sur des codes de sortie CLI.
var (
	ErrNoInstance = errors.New("instance introuvable ou non configurée")
	ErrPINNeeded  = errors.New("PIN requis : confirmez l'action dans la fenêtre SituLab")
	// ErrPINTimeout distingue, pour l'agent, le DÉLAI écoulé (personne n'a validé dans la fenêtre à
	// temps) d'un refus explicite (ErrPINNeeded). Permet à la CLI/au MCP de dire « relance et valide »
	// plutôt que « refusé ». La fenêtre reste le seul point de saisie (invariant inchangé). Durée tirée
	// de PINPromptTTL (source unique) pour rester synchrone avec le compte à rebours affiché.
	ErrPINTimeout = fmt.Errorf("délai de confirmation du PIN écoulé : relancez l'action et validez la fenêtre SituLab sur le poste (max %d s)", PINPromptTTLSeconds())
	ErrNoGit      = errors.New("git n'est pas installé sur cette machine")
	ErrPINLocked  = errors.New("trop d'essais : PIN temporairement verrouillé")
	ErrLocked     = errors.New("session verrouillée : déverrouillez avec le PIN")
	// Erreurs guidées de Synchroniser (v1.3) : phrases claires, affichées telles quelles dans
	// la zone d'erreur de l'interface, plutôt qu'une sortie git brute.
	ErrNotARepo = errors.New("ce dossier n'est pas un dépôt suivi")
	ErrDetached = errors.New("vous consultez une ancienne version, revenez sur votre version de travail avant de synchroniser")
	ErrDiverged = errors.New("vos changements et ceux des autres se croisent, ils sont à réunir")
	ErrNoRemote = errors.New("ce dépôt n'a pas de serveur associé, reliez-le d'abord à une instance")
	// v1.5.2 : cas de Synchroniser qui doivent s'arrêter proprement, sans jamais enregistrer de
	// fichier en conflit.
	ErrDirtyBehind = errors.New("des nouveautés sont en ligne ET vous avez des modifications non enregistrées, enregistrez-les (ou mettez-les de côté) d'abord, puis synchronisez")
	ErrConflict    = errors.New("des fichiers sont en conflit (modifiés des deux côtés), résolvez-les avant d'enregistrer ou de synchroniser")
	// ErrReadOnly : mode lecture seule actif (v1.4) — toute mutation est refusée tant qu'il n'est
	// pas désactivé dans Réglages → Sécurité.
	ErrReadOnly = errors.New("mode lecture seule actif : désactivez-le dans Réglages pour modifier")
)

// Réglages du verrou PIN et des opérations.
const (
	pinMaxFails  = 5
	pinLockFor   = 60 * time.Second
	pinMinLen    = 6
	opTimeout    = 30 * time.Second
	repoCacheTTL = 12 * time.Second
	sessionTTL   = 15 * time.Minute // plafond interne (glissement) si aucun délai n'est fixé
	// defaultLockTTL : auto-verrouillage par défaut (LockAfterMin=0). Fenêtre d'exposition des
	// secrets déchiffrés volontairement courte (durcissement 1.16 : était 15 min).
	defaultLockTTL = 5 * time.Minute
)

// Service est le point d'entrée unique du noyau.
type Service struct {
	mu         sync.Mutex
	cfg        *config.Config
	repoCache  map[string]repoCache // dépôts distants par instance (TTL court)
	ioMu       sync.Mutex           // sérialise les mutations persistantes (keyring + disque)
	fetchMu    sync.Mutex           // anti-chevauchement de la récupération de fond (FetchAllLocal)
	pinGuardMu sync.Mutex           // sérialise l'écriture du verrou anti-bruteforce persistant (pinguard.json)
	repoLocks  sync.Map             // map[string]*sync.Mutex : sérialise les opérations git PAR dépôt (lockRepo)
	version    string

	// devKey : clé d'appareil (32 o, fichier 0600) qui signe l'intégrité de la POLITIQUE de sécurité
	// (v1.16, fermeture P3). Fixée une fois au démarrage, immuable ensuite (pas de verrou requis en
	// lecture). nil = indisponible → dégradation gracieuse (comportement d'avant 1.16).
	devKey []byte

	// Session : DEK du trousseau tenue en mémoire après déverrouillage par le PIN.
	// Protégée par s.mu. sessUntil borne l'inactivité ; au-delà, la session est verrouillée.
	// NB : chaque invocation CLI étant un process neuf, cette session ne vit QUE pour la GUI
	// (et le futur démon, Phase 4). Une commande CLI one-shot repart verrouillée.
	sessDEK   []byte
	sessUntil time.Time
	sessTTL   time.Duration // délai d'auto-verrouillage effectif de la session courante (s.mu)
	lockTimer *time.Timer   // zéroise la DEK à l'expiration du TTL même sans accès (s.mu)

	// Anti-bruteforce du PIN tenu EN MÉMOIRE : le démon est l'unique écrivain, donc un
	// compteur en mémoire (protégé par s.mu) est race-free et ne peut pas être écrasé par
	// une écriture de config concurrente — contrairement à un champ de config.json relu et
	// réécrit par chaque mutation (faille corrigée en 1.1.0). Remis à zéro à chaque succès.
	pinFails  int
	pinLockTo int64 // instant de fin de verrou (Unix s) ; 0 = pas de verrou

	// migrated : l'import depuis l'ancien coffre OS est retenté à chaque ouverture de
	// session TANT QUE non entièrement réussi (ne passe à true que sur succès complet).
	migrated bool

	// Avancement des SAUVEGARDES, poussé vers l'UI (événement "backup:progress") pour un suivi visuel
	// permanent (manuel / ouverture / fermeture). bkBusy compte les sauvegardes en cours (≥1 = occupé).
	bkMu   sync.Mutex
	bkBusy int
	bkProg func(BackupProgress)
	// agProg : avancement des purges d'agent IA, poussé vers l'UI (posé au démarrage).
	agProg func(AgentPurgeProgress)
	bkRun  map[string]bool // plans en cours (anti-concurrence du MÊME plan : la rotation de l'un ne doit pas effacer le bundle frais de l'autre)
}

type repoCache struct {
	repos []forgejo.Repo
	trunc bool
	at    time.Time
}

// New charge la configuration et retourne le service. version alimente l'User-Agent.
func New(version string) (*Service, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	s := &Service{cfg: cfg, repoCache: map[string]repoCache{}, version: version}
	s.devKey = ensureDeviceKey() // clé d'intégrité de la politique (fermeture P3)
	s.anchorPolicy()             // signe la policy si jamais signée (mise à niveau < 1.16 / install fraîche)
	s.loadPINGuard()             // restaure le verrou anti-bruteforce persisté (best-effort, fail-safe)
	return s, nil
}

// lockedReload relit la config depuis le disque (reflète les modifs d'un autre
// processus). L'appelant DOIT tenir s.mu.
func (s *Service) lockedReload() {
	if c, err := config.Load(); err == nil {
		s.cfg = c
	}
}

func opCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), opTimeout)
}

// clientFor construit un client Forgejo authentifié pour une instance nommée.
func (s *Service) clientFor(name string) (*forgejo.Client, config.Instance, error) {
	s.mu.Lock()
	s.lockedReload()
	in := s.cfg.Resolve(name)
	if in == nil {
		s.mu.Unlock()
		return nil, config.Instance{}, ErrNoInstance
	}
	inst := *in
	s.mu.Unlock()

	token, err := s.tokenFor(inst.Name)
	if err != nil {
		return nil, inst, err
	}
	return forgejo.NewKind(inst.URL, token, s.version, providerKind(inst)), inst, nil
}

// providerKind traduit le fournisseur d'une instance en type de client.
func providerKind(inst config.Instance) forgejo.Kind {
	if inst.IsGitHub() {
		return forgejo.KindGitHub
	}
	return forgejo.KindForgejo
}

// --- Instances ---

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

// Repos renvoie les dépôts accessibles (via le cache partagé, comme le dashboard).
func (s *Service) Repos(instance string) ([]forgejo.Repo, error) {
	cl, inst, err := s.clientFor(instance)
	if err != nil {
		return nil, err
	}
	ctx, cancel := opCtx()
	defer cancel()
	repos, _, err := s.cachedRepos(ctx, inst.Name, cl)
	return repos, err
}

// ViewRepo renvoie le détail d'un dépôt owner/repo.
func (s *Service) ViewRepo(instance, owner, repo string) (*forgejo.Repo, error) {
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return nil, err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.ViewRepo(ctx, owner, repo)
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

// SetRepoDescription met à jour la description d'un dépôt distant (métadonnée, auth par token).
func (s *Service) SetRepoDescription(instance, owner, repo, desc, pin string) error {
	if err := s.authorize(instance, config.ActionRepoAdmin, pin); err != nil {
		return err
	}
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return err
	}
	ctx, cancel := opCtx()
	defer cancel()
	_, err = cl.EditRepo(ctx, owner, repo, map[string]any{"description": desc})
	return err
}

// Orgs renvoie les organisations.
func (s *Service) Orgs(instance string) ([]forgejo.Org, error) {
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return nil, err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.ListOrgs(ctx)
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

// Doctor décrit l'état de l'environnement.
type Doctor struct {
	GitAvailable  bool   `json:"git_available"`
	GitVersion    string `json:"git_version"`
	ConfigPath    string `json:"config_path"`
	KeyringPath   string `json:"keyring_path"`
	Instances     int    `json:"instances"`
	PINSet        bool   `json:"pin_set"`
	Unlocked      bool   `json:"unlocked"`
	VaultResidual int    `json:"vault_residual"` // tokens restant dans l'ancien coffre OS (doit être 0)
	SignTestRoot  bool   `json:"sign_test_root"` // la signature de garantie utilise encore la racine de TEST (à remplacer par la vraie racine de gouvernance)
}

// Doctor diagnostique la machine.
func (s *Service) Doctor() Doctor {
	s.mu.Lock()
	s.lockedReload()
	d := Doctor{
		ConfigPath:  config.Path(),
		KeyringPath: keyring.Path(),
		Instances:   len(s.cfg.Instances),
		PINSet:      keyring.Initialized(),
		Unlocked:    len(s.sessDEK) > 0 && s.sessUntil.After(time.Now()),
	}
	s.mu.Unlock()
	d.GitAvailable = gitx.Available()
	if d.GitAvailable {
		d.GitVersion, _ = gitx.Version()
	}
	d.VaultResidual = s.vaultResidual()
	d.SignTestRoot = attest.UsingTestRoot()
	return d
}

// AddLocalRepo enregistre un dépôt local à suivre (après vérification que c'en est un).
func (s *Service) AddLocalRepo(path string) error {
	if err := s.assertWritable(); err != nil {
		return err
	}
	if !gitx.Available() {
		return ErrNoGit
	}
	st, err := gitx.StatusOf(path)
	if err != nil {
		return err
	}
	if !st.IsRepo {
		// Les worktrees et sous-modules initialisés (dont le .git est un FICHIER de lien) sont
		// acceptés normalement. Un refus ici signifie que git ne voit aucun dépôt utilisable au
		// chemin indiqué — le plus souvent un sous-module PAS ENCORE initialisé.
		return fmt.Errorf("« %s » n'est pas (encore) un dépôt git. S'il s'agit d'un sous-module, initialisez-le d'abord (git submodule update --init) ; d'un worktree, vérifiez que son dépôt principal existe", path)
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.AddLocalRepo(path)
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}

// LocalStatus renvoie le statut git détaillé d'un dossier.
func (s *Service) LocalStatus(path string) (*gitx.Status, error) {
	if !gitx.Available() {
		return nil, ErrNoGit
	}
	return gitx.StatusOf(path)
}

// LocalDetail renvoie l'état détaillé d'un dépôt local (commits, branches, non poussés).
func (s *Service) LocalDetail(path string) (*gitx.Detail, error) {
	if !gitx.Available() {
		return nil, ErrNoGit
	}
	return gitx.DetailOf(path)
}

// BranchMatrix renvoie l'état par branche d'un dépôt local (ce poste vs en ligne,
// versions et dates). Local, sans réseau, à la demande.
func (s *Service) BranchMatrix(path string) (*gitx.BranchMatrix, error) {
	if !gitx.Available() {
		return nil, ErrNoGit
	}
	return gitx.MatrixOf(path)
}

// instanceToken renvoie le token déchiffré pour une instance (vide si session verrouillée
// ou instance inconnue). Les opérations git qui en dépendent échoueront proprement si vide.
func (s *Service) instanceToken(instance string) string {
	s.mu.Lock()
	s.lockedReload()
	in := s.cfg.Resolve(instance)
	s.mu.Unlock()
	if in == nil {
		return ""
	}
	t, _ := s.tokenFor(in.Name)
	return t
}

// isGitHub indique si l'instance nommée cible GitHub (pour choisir le schéma d'auth git).
func (s *Service) isGitHub(instance string) bool {
	s.mu.Lock()
	s.lockedReload()
	in := s.cfg.Resolve(instance)
	s.mu.Unlock()
	return in != nil && in.IsGitHub()
}

// instanceBaseURL renvoie l'URL de l'instance, qui scope l'en-tête d'authentification git à
// cet hôte : le jeton n'est envoyé qu'aux URL de cette instance, jamais à la cible d'une
// redirection cross-host.
func (s *Service) instanceBaseURL(instance string) string {
	s.mu.Lock()
	s.lockedReload()
	in := s.cfg.Resolve(instance)
	s.mu.Unlock()
	if in == nil {
		return ""
	}
	return in.URL
}

// connFor résout en UNE passe le jeton, le type (GitHub) et l'URL de base d'une connexion, pour
// éviter de relire la config trois fois (utile dans la boucle de fetch de fond).
func (s *Service) connFor(instance string) (token string, github bool, baseURL string) {
	s.mu.Lock()
	s.lockedReload()
	in := s.cfg.Resolve(instance)
	s.mu.Unlock()
	if in == nil {
		return "", false, ""
	}
	token, _ = s.tokenFor(in.Name)
	return token, in.IsGitHub(), in.URL
}

// Clone clone owner/repo depuis l'instance vers dir (token injecté pour un dépôt privé).
func (s *Service) Clone(instance, owner, repo, dir string) (string, error) {
	if err := s.assertWritable(); err != nil {
		return "", err
	}
	if !gitx.Available() {
		return "", ErrNoGit
	}
	r, err := s.ViewRepo(instance, owner, repo)
	if err != nil {
		return "", err
	}
	out, err := gitx.Clone(r.CloneURL, dir, s.instanceToken(instance), s.isGitHub(instance), s.instanceBaseURL(instance))
	if err != nil {
		return "", err
	}
	_ = s.AddLocalRepo(dir) // on suit automatiquement le nouveau clone
	return out, nil
}

// Fetch récupère les nouveautés distantes (lecture — pas de PIN).
func (s *Service) Fetch(instance, path string) (string, error) {
	if !gitx.Available() {
		return "", ErrNoGit
	}
	defer s.lockRepo(path)() // pas de fetch de fond concurrent sur ce dépôt
	out, err := gitx.Fetch(path, s.instanceToken(instance), s.isGitHub(instance), s.instanceBaseURL(instance))
	if err == nil {
		s.fetchAttestNotes(instance, path) // récupère les attestations des autres (best-effort)
	}
	return out, err
}

// Pull met à jour la branche locale en fast-forward (pas de PIN : rien ne part en ligne, mais
// l'arbre de travail est modifié → refusé en lecture seule).
func (s *Service) Pull(instance, path string) (string, error) {
	if !gitx.Available() {
		return "", ErrNoGit
	}
	if err := s.assertWritable(); err != nil {
		return "", err
	}
	defer s.lockRepo(path)() // pas de fetch de fond concurrent sur ce dépôt
	out, err := gitx.Pull(path, s.instanceToken(instance), s.isGitHub(instance), s.instanceBaseURL(instance), false)
	if err == nil {
		s.fetchAttestNotes(instance, path) // récupère les attestations des autres (best-effort)
	}
	return out, err
}

// Commit enregistre une version locale (pas de PIN : rien ne part en ligne ; refusé en lecture seule).
func (s *Service) Commit(path, message string) (string, error) {
	if !gitx.Available() {
		return "", ErrNoGit
	}
	if err := s.assertWritable(); err != nil {
		return "", err
	}
	defer s.lockRepo(path)() // pas de fetch de fond concurrent pendant l'enregistrement
	out, err := gitx.Commit(path, message)
	if err == nil {
		s.attestCommit(path, gitx.HeadSHA(path)) // signature de garantie (best-effort, non bloquant)
	}
	return out, asConflict(err)
}

// asConflict remappe le refus d'enregistrement pour marques de conflit (gitx.ErrConflicted) sur
// le message guidé stable du cœur (ErrConflict), pour un retour cohérent sur toutes les surfaces.
func asConflict(err error) error {
	if errors.Is(err, gitx.ErrConflicted) {
		return ErrConflict
	}
	return err
}

// Diff renvoie le diff des modifications non enregistrées d'un dépôt local.
func (s *Service) Diff(path string) (string, error) {
	if !gitx.Available() {
		return "", ErrNoGit
	}
	return gitx.Diff(path)
}

// RemoveLocalRepo cesse de suivre un dépôt local (ne supprime rien sur le disque).
func (s *Service) RemoveLocalRepo(path string) error {
	if err := s.assertWritable(); err != nil {
		return err
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	clean := config.CleanPath(path)
	out := cfg.LocalRepos[:0]
	for _, p := range cfg.LocalRepos {
		if config.CleanPath(p) != clean {
			out = append(out, p)
		}
	}
	cfg.LocalRepos = out
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}

// --- PIN & push ---

// PINSet indique si un PIN est configuré (c.-à-d. si le trousseau est initialisé).
func (s *Service) PINSet() bool {
	return keyring.Initialized()
}

// isTrivialPIN rejette un code fait d'un seul caractère répété (000000, 111111…). La vraie résistance
// hors-ligne du trousseau volé tient à l'espace du PIN + au coût Argon2id (documenté) ; bannir des motifs
// n'y change presque rien — on écarte seulement le pire cas, sans contraindre l'usage clé-en-main.
func isTrivialPIN(code string) bool {
	if len(code) < 2 {
		return false
	}
	for i := 1; i < len(code); i++ {
		if code[i] != code[0] {
			return false
		}
	}
	return true
}

// SetPIN définit (1re fois) ou change le PIN.
//   - 1re fois : initialise le trousseau chiffré et OUVRE la session (la DEK naît ici).
//   - changement : l'ancien PIN est exigé et vérifié par le MÊME chemin verrouillé que
//     Push (sinon SetPIN serait un oracle de bruteforce), puis la DEK est ré-enveloppée
//     sous le nouveau PIN sans re-chiffrer les tokens.
func (s *Service) SetPIN(oldCode, newCode string) error {
	if len(newCode) < pinMinLen {
		return fmt.Errorf("le PIN doit comporter au moins %d caractères", pinMinLen)
	}
	if isTrivialPIN(newCode) {
		return errors.New("PIN trop prévisible : évitez un code fait d'un seul chiffre répété (ex. 000000)")
	}
	if !keyring.Initialized() {
		_, dek, err := keyring.Init(newCode)
		if err != nil {
			return err
		}
		s.openSession(dek)
		zero(dek) // openSession en a pris une copie ; on efface l'original (hygiène, comme Unlock)
		s.resetPINCounters()
		s.migrateVaultOnce()
		return nil
	}
	// Changement : vérifier l'ancien PIN (rate-limité) → récupérer la DEK → ré-envelopper.
	dek, ok, err := s.verifyPINGated(oldCode)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("ancien PIN incorrect")
	}
	defer zero(dek)
	// La ré-enveloppe mute le trousseau : elle doit passer par ioMu comme toute mutation
	// persistante (sinon course last-writer-wins avec AddInstanceLogin, etc.).
	s.ioMu.Lock()
	kr, err := keyring.Load()
	if err != nil {
		s.ioMu.Unlock()
		return err
	}
	if err := kr.Rewrap(dek, newCode); err != nil {
		s.ioMu.Unlock()
		return err
	}
	s.ioMu.Unlock()
	s.openSession(dek)
	s.migrateVaultOnce()
	return nil
}

// ResetKeyring efface le trousseau chiffré (cas « PIN oublié » : le PIN étant la seule clé,
// il est irrécupérable). Les instances sont CONSERVÉES dans la config mais perdent leur token
// (elles deviennent « déconnectées ») : l'utilisateur recrée un PIN puis les reconnecte une à
// une. Session fermée, compteurs remis à zéro. Les dépôts locaux ne sont PAS touchés.
func (s *Service) ResetKeyring() error {
	s.ioMu.Lock()
	if err := os.Remove(keyring.Path()); err != nil && !os.IsNotExist(err) {
		s.ioMu.Unlock()
		return err
	}
	s.ioMu.Unlock()
	s.Lock()
	s.mu.Lock()
	s.pinFails, s.pinLockTo, s.migrated = 0, 0, false
	s.mu.Unlock()
	return nil
}

// DisconnectedInstances renvoie les noms des instances sans token valide dans le trousseau
// (après un reset de PIN, ou si le trousseau n'est pas initialisé) — à reconnecter.
func (s *Service) DisconnectedInstances() []string {
	cfg, err := config.Load()
	if err != nil {
		return nil
	}
	kr, kerr := keyring.Load()
	var out []string
	for _, in := range cfg.Instances {
		if kerr != nil || !kr.Has(in.Name) {
			out = append(out, in.Name)
		}
	}
	return out
}

// lockTTL renvoie le délai d'auto-verrouillage réglé par l'utilisateur (config LockAfterMin) :
// 0 ⇒ défaut (defaultLockTTL) ; -1 ⇒ « jamais » (durée très longue, effectivement sans expiration).
func (s *Service) lockTTL() time.Duration {
	m := 0
	if c, err := config.Load(); err == nil {
		m = c.LockAfterMin
	}
	switch {
	case m < 0:
		return 100 * 365 * 24 * time.Hour // « jamais »
	case m == 0:
		return defaultLockTTL // défaut durci (1.16)
	default:
		return time.Duration(m) * time.Minute
	}
}

// LockDelay renvoie le réglage courant (minutes ; -1 = jamais ; 0 = défaut 5, durci en 1.16).
func (s *Service) LockDelay() int {
	if c, err := config.Load(); err == nil {
		return c.LockAfterMin
	}
	return 0
}

// SetLockDelay enregistre le délai d'auto-verrouillage (minutes ; -1 = jamais ; 0 = défaut)
// et le ré-applique à la session en cours.
func (s *Service) SetLockDelay(min int) error {
	s.ioMu.Lock()
	cfg, err := config.Load()
	if err != nil {
		s.ioMu.Unlock()
		return err
	}
	cfg.LockAfterMin = min
	if err := cfg.Save(); err != nil {
		s.ioMu.Unlock()
		return err
	}
	s.publish(cfg)
	s.ioMu.Unlock()
	// Ré-arme la session ouverte avec le nouveau délai.
	s.mu.Lock()
	if len(s.sessDEK) > 0 {
		s.sessTTL = s.lockTTL()
		s.sessUntil = time.Now().Add(s.sessTTL)
		if s.lockTimer != nil {
			s.lockTimer.Stop()
		}
		s.lockTimer = time.AfterFunc(s.sessTTL, s.lockIfExpired)
	}
	s.mu.Unlock()
	return nil
}

// LockAfterWrite indique si la session doit se reverrouiller après chaque écriture réussie.
func (s *Service) LockAfterWrite() bool {
	if c, err := config.Load(); err == nil {
		return c.LockAfterWrite
	}
	return false
}

// SetLockAfterWrite active/désactive le verrouillage après écriture.
func (s *Service) SetLockAfterWrite(on bool) error {
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.LockAfterWrite = on
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}

// ReadOnly indique si le mode lecture seule est actif (v1.4) : toute mutation est alors refusée.
// FAIL-CLOSED : si la config est illisible (erreur d'E/S, verrou antivirus…), on considère la
// lecture seule ACTIVE — un garde-fou de sûreté doit échouer en position fermée. (Une config
// absente renvoie une config vide SANS erreur, donc une install neuve n'est pas bloquée.)
func (s *Service) ReadOnly() bool {
	c, err := config.Load()
	if err != nil {
		return true
	}
	if !s.policyOK(c) {
		return true // politique falsifiée → position fermée (lecture seule), fermeture P3
	}
	return c.ReadOnly
}

// AgentNotes renvoie la fiche de collaboration avec l'IA (consignes libres de l'utilisateur,
// reprises dans AGENTS.md au moment du dépôt). v1.5.1.
func (s *Service) AgentNotes() string {
	if c, err := config.Load(); err == nil {
		return c.AgentNotes
	}
	return ""
}

// SetAgentNotes enregistre la fiche de collaboration avec l'IA.
func (s *Service) SetAgentNotes(notes string) error {
	if err := s.assertWritable(); err != nil {
		return err
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.AgentNotes = notes
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}

// assertWritable renvoie ErrReadOnly si le mode lecture seule est actif. À appeler en tête de
// TOUTE mutation qui ne passe pas déjà par gatePIN (mutations git locales, mutations de config) :
// c'est le point unique qui garantit que « lecture seule » couvre vraiment toute modification,
// sur les trois surfaces (fenêtre, CLI, MCP).
func (s *Service) assertWritable() error {
	if s.ReadOnly() {
		return ErrReadOnly
	}
	return nil
}

// SetReadOnly active/désactive le mode lecture seule.
func (s *Service) SetReadOnly(on bool) error {
	if err := s.assertPolicyIntact(); err != nil { // refus de re-signer par-dessus une policy falsifiée
		return err
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.ReadOnly = on
	s.resignPolicy(cfg) // read_only fait partie de la policy signée (fermeture P3)
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}

// resetPINCounters remet à zéro le compteur anti-bruteforce (succès / init).
func (s *Service) resetPINCounters() {
	s.mu.Lock()
	s.pinFails = 0
	s.pinLockTo = 0
	s.mu.Unlock()
	s.savePINGuard() // persiste la remise à zéro (hors s.mu)
}

// verifyPINGated vérifie un code en tentant de déballer la DEK du trousseau, en RÉSERVANT
// un essai de façon atomique AVANT le calcul coûteux (Argon2id). Ainsi le verrou 5 essais
// / 60 s résiste à la concurrence intra-processus (TOCTOU éliminé) : au plus pinMaxFails
// dérivations par fenêtre, les appels en excès sont rejetés à la porte. Renvoie (dek, ok,
// err) ; err non nil signale un verrou actif, l'absence de trousseau, ou un fichier
// illisible. En cas de succès la DEK est renvoyée (l'appelant l'ouvre en session ou l'efface).
func (s *Service) verifyPINGated(code string) ([]byte, bool, error) {
	s.mu.Lock()
	now := time.Now().Unix()
	if s.pinLockTo > now {
		wait := s.pinLockTo - now
		s.mu.Unlock()
		return nil, false, fmt.Errorf("%w : réessayez dans %d s", ErrPINLocked, wait)
	}
	if !keyring.Initialized() {
		s.mu.Unlock()
		return nil, false, errors.New("aucun PIN configuré : définissez-en un d'abord")
	}
	// Réservation atomique d'un créneau d'essai, verrou tenu, AVANT la dérivation Argon2id.
	// Compteur EN MÉMOIRE (s.mu) : aucune écriture de config concurrente ne peut l'écraser.
	if s.pinFails >= pinMaxFails {
		s.pinLockTo = time.Now().Add(pinLockFor).Unix()
		s.pinFails = 0
		s.mu.Unlock()
		s.savePINGuard() // le verrou doit survivre à un redémarrage de l'app
		return nil, false, fmt.Errorf("%w : verrouillé %d s après %d essais", ErrPINLocked, int(pinLockFor.Seconds()), pinMaxFails)
	}
	s.pinFails++
	s.mu.Unlock()
	s.savePINGuard() // persiste le compteur d'essais (hors s.mu)

	kr, lerr := keyring.Load() // hors verrou
	if lerr != nil {
		return nil, false, errors.New("trousseau illisible : redéfinissez le PIN")
	}
	dek, derr := kr.Unlock(code) // CPU (Argon2id), hors verrou
	if derr != nil {
		return nil, false, nil // PIN faux : l'essai est déjà comptabilisé par la réservation
	}

	// Succès : on remet le compteur à zéro.
	s.resetPINCounters()
	return dek, true, nil
}

// VerifyPIN vérifie un PIN À BLANC (sans ouvrir de session), throttlé par le MÊME verrou anti-bruteforce
// que le déverrouillage. La fenêtre s'en sert pour signaler un PIN FAUX avant de soumettre la demande au
// démon — au lieu de refermer la modale en silence et de laisser l'action échouer sans explication.
// Renvoie (ok, err) ; err non nil = verrou actif, trousseau absent/illisible. La DEK n'est jamais retenue.
func (s *Service) VerifyPIN(code string) (bool, error) {
	dek, ok, err := s.verifyPINGated(code)
	if ok {
		zero(dek)
	}
	return ok, err
}

// Push pousse un dépôt local vers le distant. Le PIN est OBLIGATOIRE et protégé par
// un verrou anti-bruteforce (consentement humain garanti).
func (s *Service) Push(instance, path, pinCode string) (string, error) {
	if !gitx.Available() {
		return "", ErrNoGit
	}
	if err := s.authorizePush(instance, path, pinCode); err != nil {
		return "", err
	}
	defer s.lockRepo(path)() // pas de fetch de fond concurrent sur ce dépôt
	out, err := gitx.Push(path, s.instanceToken(instance), s.isGitHub(instance), s.instanceBaseURL(instance))
	if err == nil {
		s.pushAttestNotes(instance, path) // rend la signature de garantie visible en ligne (best-effort)
	}
	return out, err
}

// SyncResult décrit, en clair, ce que Synchroniser a réalisé — pour un retour non technique.
type SyncResult struct {
	Committed bool   `json:"committed"` // une version a été enregistrée localement
	Pulled    bool   `json:"pulled"`    // des nouveautés ont été récupérées du serveur
	Pushed    bool   `json:"pushed"`    // les enregistrements locaux sont partis en ligne
	Nothing   bool   `json:"nothing"`   // rien à faire : déjà à jour
	Summary   string `json:"summary"`   // phrase lisible résumant l'opération
}

// Synchronize est le geste unique « Synchroniser » des non-développeurs : récupérer les
// nouveautés du serveur, enregistrer les modifications en cours, puis envoyer. Le PIN est
// OBLIGATOIRE (un envoi en ligne a lieu) et vérifié UNE SEULE FOIS pour toute la séquence. Les cas
// délicats renvoient une erreur guidée stable (ErrDiverged, ErrDetached, ErrNoRemote…), une phrase
// claire plutôt qu'une sortie git brute. Si message est vide, un libellé daté est utilisé pour
// l'enregistrement.
//
// ORDRE : on récupère AVANT d'enregistrer. SÛRETÉ (v1.5.2) : Synchroniser ne produit JAMAIS de
// fusion susceptible de laisser des marques de conflit — l'avance rapide n'a lieu que si l'arbre
// est propre ; « en retard + modifications en cours » et « commits des deux côtés » s'arrêtent avec
// un message guidé (ErrDirtyBehind, ErrDiverged) ; un conflit préexistant (ErrConflict) interdit
// tout enregistrement.
func (s *Service) Synchronize(instance, path, message, pinCode string) (*SyncResult, error) {
	if !gitx.Available() {
		return nil, ErrNoGit
	}
	if err := s.authorizePush(instance, path, pinCode); err != nil { // politique liée au dépôt réel ; une seule confirmation pour tout le geste
		return nil, err
	}
	defer s.lockRepo(path)() // tout le geste récupérer→enregistrer→envoyer tient le verrou du dépôt
	st, err := gitx.StatusOf(path)
	if err != nil {
		return nil, err
	}
	if !st.IsRepo {
		return nil, ErrNotARepo
	}
	if st.Detached {
		return nil, ErrDetached
	}
	if !gitx.HasRemote(path) {
		return nil, ErrNoRemote
	}
	res := &SyncResult{}
	token, github, base := s.instanceToken(instance), s.isGitHub(instance), s.instanceBaseURL(instance)

	// 1) Récupérer d'abord. On relit l'état APRÈS fetch (l'avance/retard n'est fiable qu'une fois
	//    les refs distantes rapatriées) et on CAPTURE l'erreur de relecture : un `git status` qui
	//    échoue ne doit pas être lu comme « rien à faire » et déclencher un envoi injustifié.
	if _, err := gitx.Fetch(path, token, github, base); err != nil {
		return nil, err
	}
	_, _ = gitx.FetchNotes(path, attestNotesRef, token, github, base) // attestations des autres (best-effort)
	st2, err := gitx.StatusOf(path)
	if err != nil {
		return nil, err
	}
	if len(st2.Conflicted) > 0 {
		return nil, ErrConflict // conflit préexistant : on n'enregistre JAMAIS des marques de conflit
	}
	if st2.Behind > 0 {
		if st2.Ahead > 0 {
			return nil, ErrDiverged // vrais changements croisés : commits des deux côtés
		}
		if !st2.Clean {
			// En retard ET modifications non enregistrées : une avance rapide écraserait ou
			// entrerait en conflit avec le travail en cours. On s'arrête proprement (jamais de
			// fusion automatique qui laisserait des marques de conflit) et on guide.
			return nil, ErrDirtyBehind
		}
		// Arbre propre et en retard : avance rapide sûre (aucun conflit possible).
		if _, err := gitx.Pull(path, token, github, base, false); err != nil {
			return nil, err
		}
		res.Pulled = true
	}

	// 2) Enregistrer les modifications en cours, APRÈS avoir intégré le distant. Garde-fou : si un
	//    conflit est apparu, on NE commite PAS — on s'arrête et on guide (jamais de marques de
	//    conflit enregistrées ni publiées).
	st3, err := gitx.StatusOf(path)
	if err != nil {
		return nil, err
	}
	if len(st3.Conflicted) > 0 {
		return nil, ErrConflict
	}
	if !st3.Clean {
		msg := strings.TrimSpace(message)
		if msg == "" {
			msg = "Modifications du " + time.Now().Format("02/01/2006 15:04")
		}
		if _, err := gitx.Commit(path, msg); err != nil {
			return nil, asConflict(err)
		}
		s.attestCommit(path, gitx.HeadSHA(path)) // signature de garantie (best-effort)
		res.Committed = true
	}

	// 3) Envoyer : seulement sur un signal POSITIF (on est en avance, ou la branche n'a pas encore
	//    d'amont et doit être publiée) — jamais sur une valeur par défaut issue d'une lecture ratée.
	st4, err := gitx.StatusOf(path)
	if err != nil {
		return nil, err
	}
	if st4.Ahead > 0 || st4.Upstream == "" {
		if _, err := gitx.Push(path, token, github, base); err != nil {
			return nil, fmt.Errorf("enregistré sur ce poste, mais l'envoi en ligne a échoué : %w", err)
		}
		_, _ = gitx.PushNotes(path, attestNotesRef, token, github, base) // publie la signature de garantie (best-effort)
		res.Pushed = true
	}

	res.Nothing = !res.Committed && !res.Pulled && !res.Pushed
	res.Summary = syncSummary(res)
	return res, nil
}

// syncSummary compose une phrase lisible à partir de ce qui a été fait.
func syncSummary(r *SyncResult) string {
	if r.Nothing {
		return "Déjà à jour, rien à synchroniser."
	}
	var parts []string
	if r.Committed {
		parts = append(parts, "version enregistrée")
	}
	if r.Pulled {
		parts = append(parts, "nouveautés récupérées")
	}
	if r.Pushed {
		parts = append(parts, "envoyé en ligne")
	}
	return "Synchronisé : " + strings.Join(parts, ", ") + "."
}

// gatePIN exige un PIN valide pour une action écrivant vers le distant (push, création
// de dépôt/issue…). Passe par le chemin verrouillé anti-bruteforce. Un PIN correct
// rafraîchit aussi la session de lecture (confirmer une écriture ⇒ session déverrouillée).
func (s *Service) gatePIN(pinCode string) error {
	// Mode lecture seule (v1.4) : on refuse AVANT toute vérification de PIN. gatePIN couvre les
	// mutations DISTANTES (push, création/suppression, issues, PR, admin, jetons) ; les mutations
	// LOCALES (commit, pull, stash, branche…) et de config, qui ne passent pas par ici, sont
	// gardées séparément par assertWritable().
	if s.ReadOnly() {
		return ErrReadOnly
	}
	if pinCode == "" {
		if !s.PINSet() {
			return errors.New("aucun PIN configuré : définissez-en un d'abord")
		}
		return ErrPINNeeded
	}
	dek, ok, err := s.verifyPINGated(pinCode)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("PIN incorrect")
	}
	s.openSession(dek)
	zero(dek) // openSession en a pris une copie ; on efface l'original (DEK = clé maîtresse)
	return nil
}

// RequiresPIN expose la politique PIN par connexion (v1.7) : l'action distante a exige-t-elle le
// PIN sur cette instance, ou la session déverrouillée suffit-elle ? Fail-closed : instance inconnue
// → PIN exigé.
func (s *Service) RequiresPIN(instance string, a config.PINAction) bool {
	s.mu.Lock()
	s.lockedReload()
	cfg := s.cfg
	in := cfg.Resolve(instance)
	ok := s.policyOK(cfg) // intégrité de la politique (fermeture P3)
	s.mu.Unlock()
	if in == nil || !ok {
		return true // fail-closed : connexion inconnue OU politique falsifiée → PIN exigé
	}
	return in.RequiresPIN(a)
}

// authorize autorise une mutation DISTANTE selon la politique de la connexion (v1.7). Lecture seule
// → refus. Si la politique exige le PIN pour cette action → chemin gatePIN classique (qui ouvre la
// session). Sinon (ex. atelier autonome) → la session déjà déverrouillée suffit, sans PIN.
// Fail-closed : session fermée sur un chemin sans PIN → ErrLocked (le démon aurait dû déverrouiller).
func (s *Service) authorize(instance string, a config.PINAction, pin string) error {
	if s.ReadOnly() {
		return ErrReadOnly
	}
	if !s.RequiresPIN(instance, a) {
		if s.Unlocked() {
			return nil
		}
		return ErrLocked
	}
	return s.gatePIN(pin)
}

// RequiresPINForPush décide de l'exigence de PIN pour un ENVOI de code en liant la politique au
// DÉPÔT RÉELLEMENT poussé (son remote origin), pas seulement à la connexion nommée par l'appelant.
// Un agent ne peut donc pas pousser vers une production via une connexion atelier : on prend la
// politique la plus stricte entre la connexion nommée et celle déduite de l'origin du dépôt, et on
// exige le PIN (fail-closed) si l'origin ne correspond à aucune connexion unique (inconnu/ambigu).
func (s *Service) RequiresPINForPush(named, path string) bool {
	if s.RequiresPIN(named, config.ActionPush) {
		return true
	}
	m := s.instanceForPath(path)
	if m == "" {
		return true // remote inconnu ou hôte partagé par plusieurs connexions → prudence
	}
	return s.RequiresPIN(m, config.ActionPush)
}

// authorizePush autorise un envoi de code selon RequiresPINForPush (politique liée au dépôt réel).
func (s *Service) authorizePush(named, path, pin string) error {
	if s.ReadOnly() {
		return ErrReadOnly
	}
	if !s.RequiresPINForPush(named, path) {
		if s.Unlocked() {
			return nil
		}
		return ErrLocked
	}
	return s.gatePIN(pin)
}

// instanceForPath déduit la connexion propriétaire d'un dépôt local à partir de l'hôte de son
// remote origin. Renvoie "" si inconnu ou si l'hôte est partagé par plusieurs connexions.
func (s *Service) instanceForPath(path string) string {
	st, err := gitx.StatusOf(path)
	if err != nil || st.Remote == "" {
		return ""
	}
	return s.instanceForHost(hostOf(st.Remote))
}

// --- Écritures et lectures API étendues (v1) ---

// CreateRepo crée un dépôt distant (écriture — PIN requis). Le PIN est vérifié AVANT de
// construire le client : il ouvre la session, dont clientFor a besoin pour le token.
func (s *Service) CreateRepo(instance, owner, name, desc string, private bool, pin string) (*forgejo.Repo, error) {
	if err := s.authorize(instance, config.ActionRepoAdmin, pin); err != nil {
		return nil, err
	}
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return nil, err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.CreateRepo(ctx, owner, name, desc, private)
}

// DeleteRepo supprime définitivement un dépôt distant (écriture — PIN requis).
func (s *Service) DeleteRepo(instance, owner, repo, pin string) error {
	if err := s.authorize(instance, config.ActionRepoAdmin, pin); err != nil {
		return err
	}
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.DeleteRepo(ctx, owner, repo)
}

// CreateIssue crée une issue (écriture — PIN requis).
func (s *Service) CreateIssue(instance, owner, repo, title, body, pin string) (*forgejo.Issue, error) {
	if err := s.authorize(instance, config.ActionIssues, pin); err != nil {
		return nil, err
	}
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return nil, err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.CreateIssue(ctx, owner, repo, title, body)
}

// CommentIssue ajoute un commentaire à une issue/PR (écriture — PIN requis).
func (s *Service) CommentIssue(instance, owner, repo string, index int, body, pin string) error {
	if err := s.authorize(instance, config.ActionIssues, pin); err != nil {
		return err
	}
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.CommentIssue(ctx, owner, repo, index, body)
}

// SetIssueState ferme/rouvre une issue (écriture — PIN requis).
func (s *Service) SetIssueState(instance, owner, repo string, index int, state, pin string) error {
	if err := s.authorize(instance, config.ActionIssues, pin); err != nil {
		return err
	}
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.SetIssueState(ctx, owner, repo, index, state)
}

// EditIssue modifie titre/corps d'une issue (écriture — PIN requis).
func (s *Service) EditIssue(instance, owner, repo string, index int, title, body, pin string) error {
	if err := s.authorize(instance, config.ActionIssues, pin); err != nil {
		return err
	}
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.EditIssue(ctx, owner, repo, index, title, body)
}

// CreatePull ouvre une demande de fusion (écriture — PIN requis).
func (s *Service) CreatePull(instance, owner, repo, title, head, base, body, pin string) (*forgejo.Issue, error) {
	if err := s.authorize(instance, config.ActionIssues, pin); err != nil {
		return nil, err
	}
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return nil, err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.CreatePull(ctx, owner, repo, title, head, base, body)
}

// MergePull fusionne une demande de fusion (écriture — PIN requis).
func (s *Service) MergePull(instance, owner, repo string, index int, method, pin string) error {
	if err := s.authorize(instance, config.ActionIssues, pin); err != nil {
		return err
	}
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.MergePull(ctx, owner, repo, index, method)
}

// RepoIssues / RepoPulls / RepoReleases : lectures (pas de PIN).
func (s *Service) RepoIssues(instance, owner, repo, state string) ([]forgejo.Issue, error) {
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return nil, err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.ListIssues(ctx, owner, repo, state)
}
func (s *Service) RepoPulls(instance, owner, repo, state string) ([]forgejo.Issue, error) {
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return nil, err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.ListPulls(ctx, owner, repo, state)
}
func (s *Service) RepoReleases(instance, owner, repo string) ([]forgejo.Release, error) {
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return nil, err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.ListReleases(ctx, owner, repo)
}

// --- Administration (nécessite un token admin) ---

// AdminScopeFor indique si le jeton enregistré pour l'instance porte déjà le droit
// d'administration (write:admin). Permet de ne proposer « Activer l'administration » que quand
// c'est nécessaire — le compte peut être admin côté serveur sans que le jeton ait ce droit.
func (s *Service) AdminScopeFor(instance string) bool {
	cfg, err := config.Load()
	if err != nil {
		return false
	}
	in := cfg.Resolve(instance)
	if in == nil {
		return false
	}
	kr, err := keyring.Load()
	if err != nil {
		return false
	}
	for _, sc := range kr.Scopes(in.Name) {
		if sc == "write:admin" {
			return true
		}
	}
	return false
}

// AdminUsers liste les utilisateurs (lecture).
func (s *Service) AdminUsers(instance string) ([]forgejo.AdminUser, error) {
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return nil, err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.ListUsers(ctx)
}

// --- Annuaire consolidé multi-instances (v1.4) ---

// DirInstance résume l'interrogation d'une instance pour l'annuaire.
type DirInstance struct {
	Name  string `json:"name"`
	OK    bool   `json:"ok"`              // interrogée avec succès (compte admin)
	Error string `json:"error,omitempty"` // raison si KO (pas admin, injoignable…)
}

// DirEntry est une personne vue dans l'annuaire : identité + présence par instance.
type DirEntry struct {
	Login    string            `json:"login"`
	Email    string            `json:"email"`     // e-mail affiché (le 1er rencontré)
	FullName string            `json:"full_name"` // nom affiché (le 1er rencontré)
	Admin    bool              `json:"admin"`     // administrateur sur au moins une instance
	Presence map[string]string `json:"presence"`  // instance -> "actif"|"suspendu"|"inactif"|"absent"
	Search   string            `json:"search"`    // login + TOUS les e-mails/noms distincts (minuscule) : base de la recherche transverse
}

// Directory est l'annuaire consolidé : l'état des instances interrogées et la liste des personnes.
type Directory struct {
	Instances []DirInstance `json:"instances"`
	Entries   []DirEntry    `json:"entries"`
}

// userDirStatus traduit le statut d'un compte Forgejo en mot lisible.
func userDirStatus(u forgejo.AdminUser) string {
	switch {
	case u.ProhibitLogin:
		return "suspendu"
	case !u.Active:
		return "inactif"
	default:
		return "actif"
	}
}

// ConsolidatedDirectory agrège les comptes de TOUTES les instances Forgejo configurées en une
// seule liste, avec la présence par instance (actif/suspendu/inactif/absent). Lecture seule et
// tolérante : une instance sans droits admin ou injoignable est signalée sans faire échouer le
// tout. Les personnes sont regroupées par identifiant (insensible à la casse).
func (s *Service) ConsolidatedDirectory() (*Directory, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	dir := &Directory{Instances: []DirInstance{}, Entries: []DirEntry{}}
	collected := map[string][]forgejo.AdminUser{}
	var okOrder []string

	for _, in := range cfg.Instances {
		if in.IsGitHub() {
			continue // l'administration est propre à Forgejo/Gitea
		}
		di := DirInstance{Name: in.Name}
		users, uerr := s.AdminUsers(in.Name)
		if uerr != nil {
			// Un verrou de session est GLOBAL (pas propre à une instance) : on le remonte tel
			// quel pour que l'interface redemande le PIN, au lieu de l'afficher comme « instance
			// injoignable » (ce qui laisserait la fenêtre dans une impasse trompeuse).
			if errors.Is(uerr, ErrLocked) || errors.Is(uerr, ErrPINLocked) {
				return nil, uerr
			}
			di.OK, di.Error = false, uerr.Error()
			dir.Instances = append(dir.Instances, di)
			continue
		}
		di.OK = true
		dir.Instances = append(dir.Instances, di)
		okOrder = append(okOrder, in.Name)
		collected[in.Name] = users
	}
	dir.Entries = mergeDirectoryEntries(okOrder, collected)
	return dir, nil
}

// mergeDirectoryEntries fusionne les comptes de plusieurs instances en une liste unique, regroupés
// par identifiant (insensible à la casse), avec la présence par instance et « absent » là où la
// personne ne figure pas. Fonction PURE (sans réseau) : c'est le cœur testable de l'annuaire.
func mergeDirectoryEntries(okOrder []string, byInstance map[string][]forgejo.AdminUser) []DirEntry {
	byKey := map[string]*DirEntry{}
	order := []string{}
	terms := map[string]map[string]bool{} // clé -> ensemble des termes de recherche distincts
	addTerm := func(key, term string) {
		term = strings.ToLower(strings.TrimSpace(term))
		if term == "" {
			return
		}
		if terms[key] == nil {
			terms[key] = map[string]bool{}
		}
		terms[key][term] = true
	}
	for _, name := range okOrder {
		for _, u := range byInstance[name] {
			key := strings.ToLower(u.Login)
			e := byKey[key]
			if e == nil {
				e = &DirEntry{Login: u.Login, Presence: map[string]string{}}
				byKey[key] = e
				order = append(order, key)
			}
			if e.Email == "" {
				e.Email = u.Email
			}
			if e.FullName == "" {
				e.FullName = u.FullName
			}
			if u.IsAdmin {
				e.Admin = true
			}
			e.Presence[name] = userDirStatus(u)
			// Indexe TOUTES les variantes (login + e-mails + noms de chaque instance) pour que la
			// recherche transverse trouve la personne même si son e-mail/nom diffère d'une instance
			// à l'autre.
			addTerm(key, u.Login)
			addTerm(key, u.Email)
			addTerm(key, u.FullName)
		}
	}
	out := make([]DirEntry, 0, len(order))
	for _, key := range order {
		e := byKey[key]
		for _, name := range okOrder {
			if _, ok := e.Presence[name]; !ok {
				e.Presence[name] = "absent"
			}
		}
		set := terms[key]
		parts := make([]string, 0, len(set))
		for t := range set {
			parts = append(parts, t)
		}
		sort.Strings(parts) // ordre stable (sinon dépend de l'itération de map)
		e.Search = strings.Join(parts, " ")
		out = append(out, *e)
	}
	return out
}

// AdminCreateUser crée un utilisateur (écriture — PIN requis).
func (s *Service) AdminCreateUser(instance, username, email, password string, admin bool, pin string) (*forgejo.AdminUser, error) {
	if err := s.authorize(instance, config.ActionUserAdmin, pin); err != nil {
		return nil, err
	}
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return nil, err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.CreateUser(ctx, username, email, password, admin)
}

// AdminUserRepos liste les dépôts accessibles à un utilisateur (lecture — ce qu'il peut
// voir : ses dépôts et ceux de ses organisations/équipes). Clé = identifiant numérique.
func (s *Service) AdminUserRepos(instance string, uid int64) ([]forgejo.Repo, error) {
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return nil, err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.UserAccessibleRepos(ctx, uid, 50)
}

// AdminUserActivities renvoie les dernières activités d'un utilisateur (lecture).
// Léger et à la demande : une seule page, chargée quand on ouvre la fiche.
func (s *Service) AdminUserActivities(instance, username string) ([]forgejo.Activity, error) {
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return nil, err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.UserActivities(ctx, username, 20)
}

// AdminSuspendUser suspend (suspend=true) ou réactive un compte (écriture — PIN requis).
// Révocation réversible : la connexion est bloquée, rien n'est effacé.
func (s *Service) AdminSuspendUser(instance, username string, suspend bool, pin string) error {
	if err := s.authorize(instance, config.ActionUserAdmin, pin); err != nil {
		return err
	}
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.SetUserLogin(ctx, username, suspend)
}

// AdminDeleteUser supprime définitivement un compte (écriture — PIN requis).
func (s *Service) AdminDeleteUser(instance, username string, purge bool, pin string) error {
	if err := s.authorize(instance, config.ActionUserAdmin, pin); err != nil {
		return err
	}
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.DeleteUser(ctx, username, purge)
}

// --- Gestionnaire de tokens (auth basique : user + mot de passe fournis à l'appel,
// jamais stockés). Les actions portent sur le compte fourni, avec ses droits. ---

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

// TokenList liste les jetons du compte (lecture). otp = code 2FA si le compte en a un.
// Si password est vide, on utilise le mot de passe mémorisé (comptes sans 2FA) → PIN seul.
func (s *Service) TokenList(instance, user, password, otp string) ([]forgejo.Token, error) {
	if password == "" {
		password = s.storedPassword(instance)
	}
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return nil, err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.TokenList(ctx, user, password, otp)
}

// TokenCreate crée un jeton. Renvoie sa valeur. Si repos (liste de « owner/repo ») est non vide,
// le token est restreint à ces dépôts. otp = code 2FA. v1.5.0 : plus de PIN par action —
// l'authentification est le mot de passe Forgejo (+ 2FA) sur une session déverrouillée ; le mode
// lecture seule reste bloquant (assertWritable). Le paramètre pin n'est plus utilisé (compat).
func (s *Service) TokenCreate(instance, user, password, otp, name string, scopes, repos []string, pin string) (string, error) {
	// Création d'un jeton = mutation d'administration sensible (le jeton forgé a des droits
	// d'écriture) soumise à la politique PIN (ActionUserAdmin) en plus de l'auth mot de passe/2FA :
	// sinon un agent sur session déverrouillée utiliserait le mot de passe mémorisé et obtiendrait
	// un jeton d'écriture sans aucun PIN.
	if err := s.authorize(instance, config.ActionUserAdmin, pin); err != nil {
		return "", err
	}
	if password == "" {
		password = s.storedPassword(instance)
	}
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return "", err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.TokenCreate(ctx, user, password, otp, name, scopes, repos)
}

// TokenDelete supprime un jeton. otp = code 2FA. v1.5.0 : plus de PIN par action (voir
// TokenCreate) ; lecture seule bloquante. Le paramètre pin n'est plus utilisé (compat).
func (s *Service) TokenDelete(instance, user, password, otp string, id int64, pin string) error {
	if err := s.authorize(instance, config.ActionUserAdmin, pin); err != nil { // v1.7 : sous politique PIN (admin)
		return err
	}
	if password == "" {
		password = s.storedPassword(instance)
	}
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.TokenDelete(ctx, user, password, otp, id)
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
