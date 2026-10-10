// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

// Package config gère la configuration locale de SituLab (instances, dépôts locaux
// suivis, réglages). Aucun secret n'y est stocké : les jetons sont chiffrés dans le
// trousseau local (Argon2id → KEK → DEK, AES-256-GCM), protégé par le PIN. Le condensé
// Argon2id du PIN et les jetons chiffrés vivent dans le trousseau, pas ici.
//
// Deux garanties importantes pour un logiciel destiné à des non-développeurs :
//   - écriture ATOMIQUE (fichier temporaire + rename) : un crash en cours d'écriture
//     ne laisse jamais un config.json tronqué ;
//   - lecture TOLÉRANTE : un fichier illisible est sauvegardé en .bak et l'app repart
//     d'une config vide plutôt que de refuser de démarrer.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Rôle d'une connexion (v1.7) : atelier = terrain de dev (source de vérité, l'IA peut y être
// autonome) ; production = vitrine protégée (on n'y écrit que par promotion, tout sous PIN).
const (
	RoleAtelier    = "atelier"
	RoleProduction = "production"
)

// PINAction : classe d'action distante soumise à la politique PIN par connexion (v1.7).
type PINAction string

const (
	ActionPush      PINAction = "push"       // envoi de code (push, sync)
	ActionIssues    PINAction = "issues"     // issues, PR, commentaires, labels, revues…
	ActionRepoAdmin PINAction = "repo_admin" // créer/supprimer/renommer/visibilité/archiver/branche défaut, releases/tags
	ActionUserAdmin PINAction = "user_admin" // administration des comptes, gestion des jetons
)

// PINPolicy : pour chaque classe d'action, exiger le PIN (true) ou l'autoriser session
// déverrouillée (false). Un champ nil retombe sur le défaut du rôle (voir defaultRequiresPIN).
// Les pointeurs distinguent « non défini » (hérite du défaut) de « explicitement false ».
type PINPolicy struct {
	Push      *bool `json:"push,omitempty"`
	Issues    *bool `json:"issues,omitempty"`
	RepoAdmin *bool `json:"repo_admin,omitempty"`
	UserAdmin *bool `json:"user_admin,omitempty"`
}

// Instance décrit une instance git enregistrée (Forgejo/Gitea ou GitHub).
type Instance struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Default  bool   `json:"default"`
	TwoFA    bool   `json:"two_fa,omitempty"`   // le compte utilise la 2FA (→ demander le code pour gérer les jetons)
	Provider string `json:"provider,omitempty"` // "forgejo" (défaut si vide) | "github"
	Remember bool   `json:"remember,omitempty"` // mémoriser le mot de passe de connexion (choix explicite à l'ajout)

	Role      string     `json:"role,omitempty"`       // "atelier" (défaut) | "production" (v1.7)
	PINPolicy *PINPolicy `json:"pin_policy,omitempty"` // politique PIN par action ; nil = défauts du rôle (v1.7)

	// SignCert : certificat d'appareil (JSON d'attest.Cert) liant la clé d'appareil à l'identité
	// vérifiée, émis par la racine APCOM. Non secret. Vide = signature de garantie non activée.
	SignCert string `json:"sign_cert,omitempty"`

	// Mode de signature (v1.9.6). "server" = l'empreinte est demandée au service de signature serveur
	// (clé privée jamais sur le poste, agent-proof) ; vide/"local" = signature logicielle embarquée.
	// Réglé EN FENÊTRE uniquement (non exposé socket/CLI/MCP) — même doctrine que la politique PIN.
	SignMode      string `json:"sign_mode,omitempty"`       // "" | "local" | "server"
	SignServerURL string `json:"sign_server_url,omitempty"` // base du service ; vide = défaut
}

// IsGitHub indique si l'instance cible GitHub (plutôt que Forgejo/Gitea).
func (i Instance) IsGitHub() bool { return i.Provider == "github" }

// IsProduction indique si la connexion est une vitrine de production (sinon : atelier).
func (i Instance) IsProduction() bool { return i.Role == RoleProduction }

// NormalizedRole renvoie le rôle effectif (défaut : atelier).
func (i Instance) NormalizedRole() string {
	if i.Role == RoleProduction {
		return RoleProduction
	}
	return RoleAtelier
}

// RequiresPIN dit si une action distante exige le PIN pour CETTE connexion. Un champ de politique
// non défini retombe sur le défaut du rôle. Défauts : atelier = IA autonome (push/issues sans PIN,
// administration sous PIN) ; production = tout sous PIN (vitrine protégée).
func (i Instance) RequiresPIN(a PINAction) bool {
	def := defaultRequiresPIN(i.NormalizedRole(), a)
	if i.PINPolicy == nil {
		return def
	}
	var p *bool
	switch a {
	case ActionPush:
		p = i.PINPolicy.Push
	case ActionIssues:
		p = i.PINPolicy.Issues
	case ActionRepoAdmin:
		p = i.PINPolicy.RepoAdmin
	case ActionUserAdmin:
		p = i.PINPolicy.UserAdmin
	}
	if p == nil {
		return def
	}
	return *p
}

// defaultRequiresPIN : politique par défaut selon le rôle, quand rien n'est explicitement choisi.
func defaultRequiresPIN(role string, a PINAction) bool {
	if role == RoleProduction {
		return true // production : tout sous PIN
	}
	switch a { // atelier : autonome sur le contenu, PIN sur l'administration
	case ActionRepoAdmin, ActionUserAdmin:
		return true
	default:
		return false
	}
}

// RepoRef désigne un dépôt précis sur une connexion (v1.7) : instance + owner/repo.
type RepoRef struct {
	Instance string `json:"instance"`
	Owner    string `json:"owner"`
	Repo     string `json:"repo"`
}

// Key renvoie une clé canonique « instance/owner/repo » (casse d'origine conservée).
func (r RepoRef) Key() string { return r.Instance + "/" + r.Owner + "/" + r.Repo }

// Promo mémorise le dernier état promu vers une production : la version (tag) publiée et la date.
type Promo struct {
	Version string `json:"version,omitempty"` // tag/version promu en dernier
	Date    string `json:"date,omitempty"`    // date de la promotion (RFC3339)
}

// Project relie UN dépôt atelier (source de vérité du dev) à 1..N dépôts production (vitrines).
// La prod n'est jamais poussée depuis le local : on y publie par PROMOTION (v1.7.2). Promotions
// mémorise, par production (clé RepoRef.Key()), le dernier état promu — pour lire le point d'équilibre.
type Project struct {
	Name        string           `json:"name"`
	Icon        string           `json:"icon,omitempty"` // emoji/picto choisi pour le projet (facultatif)
	Atelier     RepoRef          `json:"atelier"`
	Productions []RepoRef        `json:"productions"` // vitrines (production) ; peut être vide
	Promotions  map[string]Promo `json:"promotions,omitempty"`
}

// Config est l'état persistant de SituLab (aucun secret).
type Config struct {
	Instances      []Instance `json:"instances"`
	LockAfterMin   int        `json:"lock_after_min,omitempty"`   // auto-verrouillage (minutes) ; 0 = défaut, -1 = jamais
	LockAfterWrite bool       `json:"lock_after_write,omitempty"` // reverrouille après chaque écriture réussie
	ReadOnly       bool       `json:"read_only,omitempty"`        // mode lecture seule : bloque toute mutation (v1.4)
	AgentNotes     string     `json:"agent_notes,omitempty"`      // fiche de collaboration avec l'IA (v1.5.1)
	Projects       []Project  `json:"projects,omitempty"`         // liens atelier↔production (v1.7.1)
	LocalRepos     []string   `json:"local_repos"`
	RevokedSerials []string   `json:"revoked_serials,omitempty"` // serials de certs d'appareil révoqués (signature de garantie)
	// Bibliothèque de prompts (v1.9.4) : les prompts PAR DÉFAUT sont livrés avec le binaire (évoluent
	// aux mises à jour) ; les personnalisations de l'utilisateur vivent ICI (config) et ne sont donc
	// JAMAIS écrasées par une mise à jour. Surcharge d'un prompt par défaut par id ; masquage par id ;
	// prompts ajoutés par l'utilisateur.
	PromptOverrides map[string]Prompt `json:"prompt_overrides,omitempty"`
	HiddenPrompts   []string          `json:"hidden_prompts,omitempty"`
	UserPrompts     []Prompt          `json:"user_prompts,omitempty"`
}

// Prompt : un prompt réutilisable pour l'assistant IA (titre + corps).
type Prompt struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Body  string `json:"body"`
}

// PruneDanglingPromotions retire les marqueurs de promotion dont la vitrine n'existe plus dans le
// projet (après un changement de vitrines), pour ne pas garder d'état orphelin.
func (p *Project) PruneDanglingPromotions() {
	if len(p.Promotions) == 0 {
		return
	}
	keep := map[string]bool{}
	for _, pr := range p.Productions {
		keep[pr.Key()] = true
	}
	for k := range p.Promotions {
		if !keep[k] {
			delete(p.Promotions, k)
		}
	}
}

// FindProject renvoie le projet par nom (insensible à la casse), ou nil.
func (c *Config) FindProject(name string) *Project {
	for i := range c.Projects {
		if eqName(c.Projects[i].Name, name) {
			return &c.Projects[i]
		}
	}
	return nil
}

// Dir renvoie le dossier de configuration de SituLab selon l'OS.
func Dir() string {
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "SituLab")
}

// Path renvoie le chemin du fichier config.json.
func Path() string { return filepath.Join(Dir(), "config.json") }

// Load charge la configuration. Un fichier absent donne une config vide ; un fichier
// présent mais illisible est préservé en .bak et remplacé par une config vide (l'app
// ne doit jamais devenir inutilisable à cause d'un octet corrompu).
func Load() (*Config, error) {
	c := &Config{}
	data, err := os.ReadFile(Path())
	if err != nil {
		if os.IsNotExist(err) {
			return c, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, c); err != nil {
		_ = os.Rename(Path(), Path()+".bak") // on garde la trace du fichier fautif
		return &Config{}, nil
	}
	return c, nil
}

// Save écrit la configuration de façon atomique (temp + rename sur le même volume).
// En multi-fenêtres, le dernier qui écrit gagne, mais jamais de fichier tronqué.
func (c *Config) Save() error {
	dir := Dir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "config-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op si le rename a réussi
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, Path())
}

// eqName compare deux noms d'instance en repliant la casse (« Prod » == « prod »).
func eqName(a, b string) bool { return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b)) }

// Find renvoie l'instance par nom (insensible à la casse), ou nil.
func (c *Config) Find(name string) *Instance {
	for i := range c.Instances {
		if eqName(c.Instances[i].Name, name) {
			return &c.Instances[i]
		}
	}
	return nil
}

// DefaultInstance renvoie l'instance par défaut (ou la première, ou nil).
func (c *Config) DefaultInstance() *Instance {
	for i := range c.Instances {
		if c.Instances[i].Default {
			return &c.Instances[i]
		}
	}
	if len(c.Instances) > 0 {
		return &c.Instances[0]
	}
	return nil
}

// Resolve sélectionne une instance par nom ; nom vide => instance par défaut.
func (c *Config) Resolve(name string) *Instance {
	if name == "" {
		return c.DefaultInstance()
	}
	return c.Find(name)
}

// SetDefault marque une instance comme défaut (et démarque les autres).
func (c *Config) SetDefault(name string) bool {
	found := false
	for i := range c.Instances {
		if eqName(c.Instances[i].Name, name) {
			c.Instances[i].Default = true
			found = true
		} else {
			c.Instances[i].Default = false
		}
	}
	return found
}

// Remove supprime une instance (comparaison insensible à la casse) et, si c'était la
// défaut, promeut la première instance restante. Renvoie le nom réel supprimé (pour
// retirer le token du coffre avec la bonne casse) et un booléen de succès.
func (c *Config) Remove(name string) (string, bool) {
	out := c.Instances[:0]
	removed, wasDefault := "", false
	for _, in := range c.Instances {
		if eqName(in.Name, name) {
			removed, wasDefault = in.Name, in.Default
			continue
		}
		out = append(out, in)
	}
	c.Instances = out
	if removed == "" {
		return "", false
	}
	if wasDefault && len(c.Instances) > 0 {
		c.Instances[0].Default = true
	}
	return removed, true
}

// AddLocalRepo enregistre un dépôt local (chemin nettoyé, sans doublon).
func (c *Config) AddLocalRepo(path string) {
	clean := CleanPath(path)
	for _, p := range c.LocalRepos {
		if CleanPath(p) == clean {
			return
		}
	}
	c.LocalRepos = append(c.LocalRepos, clean)
}

// CleanPath normalise un chemin pour la déduplication (absolu si possible, séparateurs
// de l'OS, sans slash final).
func CleanPath(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	return filepath.Clean(p)
}
