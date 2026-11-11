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
	"path/filepath"
	"sync"
	"time"

	"situlab/internal/attest"
	"situlab/internal/config"
	"situlab/internal/forgejo"
	"situlab/internal/gitx"
	"situlab/internal/journal"
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
	// gitChg : signal « l'état git a changé » (mutation locale ou distante). Posé au démarrage, câblé
	// par l'App sur un événement Wails « git:changed » pour FORCER le rafraîchissement de l'UI sans
	// attendre la récupération périodique (optimisation 1.20).
	gitChg func()
	jnl    *journal.Journal // journal local des opérations (1.20) : audit des gestes agent + échecs best-effort
	bkRun  map[string]bool  // plans en cours (anti-concurrence du MÊME plan : la rotation de l'un ne doit pas effacer le bundle frais de l'autre)
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
	s.jnl = journal.New(filepath.Join(config.Dir(), "journal"), journal.NormalizeLevel(cfg.JournalLevel))
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

// clientForPeek est clientFor SANS glissement d'inactivité : le jeton est lu par une SONDE
// d'arrière-plan (sonde « en ligne » périodique), pas par une action de l'utilisateur ou de l'agent.
// Voir sessionDEKPeek — indispensable pour que l'auto-verrouillage tombe même sur une vue connectée.
func (s *Service) clientForPeek(name string) (*forgejo.Client, config.Instance, error) {
	s.mu.Lock()
	s.lockedReload()
	in := s.cfg.Resolve(name)
	if in == nil {
		s.mu.Unlock()
		return nil, config.Instance{}, ErrNoInstance
	}
	inst := *in
	s.mu.Unlock()

	token, err := s.tokenForPeek(inst.Name)
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

// connForPeek est le pendant SANS glissement d'inactivité (ancien connFor, retiré car son seul
// appelant — la boucle de fetch de fond — utilise désormais cette variante) : le jeton est lu par la boucle de FETCH DE
// FOND (sonde automatique), pas par une action de l'utilisateur ou de l'agent. Voir sessionDEKPeek.
func (s *Service) connForPeek(instance string) (token string, github bool, baseURL string) {
	s.mu.Lock()
	s.lockedReload()
	in := s.cfg.Resolve(instance)
	s.mu.Unlock()
	if in == nil {
		return "", false, ""
	}
	token, _ = s.tokenForPeek(in.Name)
	return token, in.IsGitHub(), in.URL
}
