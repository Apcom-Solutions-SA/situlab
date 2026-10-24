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
	"crypto/sha256"
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"sort"
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

// --- Clouds de fichiers souverains (CORPUS, v1.10) -------------------------------------------
// SituLab se branche à des clouds de fichiers SOUVERAINS pour porter le CORPUS (contenus denses
// lus à la demande). SituLab est la PORTE UNIQUE entre l'IA du poste et ces clouds : l'agent
// (CLI/MCP) ne voit que les dossiers explicitement autorisés par l'humain (AllowedDirs) ; la
// fenêtre (humain) voit tout pour choisir. Aucun secret ici : le mot de passe WebDAV est chiffré
// au trousseau, comme un jeton git.

// Types de cloud autorisés — souverains uniquement. Les clouds propriétaires non souverains
// (OneDrive/Google Drive/Dropbox) sont écartés par principe : ils ne sont pas proposés.
const (
	CloudNextcloud = "nextcloud" // Nextcloud auto-hébergé (souverain autonome)
	CloudKDrive    = "kdrive"    // kDrive Infomaniak (souverain délégué, droit suisse)
)

// CloudAction : classe d'action cloud soumise à la politique PIN par connexion (v1.10).
type CloudAction string

const (
	CloudWrite  CloudAction = "write"  // envoyer/modifier un fichier, créer un dossier
	CloudDelete CloudAction = "delete" // supprimer un fichier ou un dossier
)

// CloudPINPolicy : pour chaque classe d'action cloud, exiger le PIN (true) ou l'autoriser en
// session déverrouillée (false). nil = défaut (voir defaultCloudRequiresPIN). Réglée EN FENÊTRE
// uniquement, comme la politique PIN git.
type CloudPINPolicy struct {
	Write  *bool `json:"write,omitempty"`
	Delete *bool `json:"delete,omitempty"`
}

// FileCloud décrit une connexion à un cloud de fichiers souverain (WebDAV). Aucun secret : le mot
// de passe (ou app-password) est chiffré au trousseau sous l'id « filecloud/<Name> ».
type FileCloud struct {
	Name        string          `json:"name"`
	Kind        string          `json:"kind"` // "nextcloud" | "kdrive"
	URL         string          `json:"url"`  // base WebDAV (ex. https://host/remote.php/dav/files/user)
	User        string          `json:"user"`
	Default     bool            `json:"default,omitempty"`
	AllowedDirs []string        `json:"allowed_dirs,omitempty"` // chemins AUTORISÉS à l'IA (dossier/fichier) ; vide = aucun
	DeniedDirs  []string        `json:"denied_dirs,omitempty"`  // chemins RETIRÉS sous un parent autorisé (exclusions fines, v1.11)
	PINPolicy   *CloudPINPolicy `json:"pin_policy,omitempty"`
}

// RequiresPINCloud dit si une action cloud exige le PIN pour CETTE connexion. Champ non défini =
// défaut : write autonome (l'IA dépose souvent des résumés), delete sous PIN (geste destructif).
func (fc FileCloud) RequiresPINCloud(a CloudAction) bool {
	def := defaultCloudRequiresPIN(a)
	if fc.PINPolicy == nil {
		return def
	}
	var p *bool
	switch a {
	case CloudWrite:
		p = fc.PINPolicy.Write
	case CloudDelete:
		p = fc.PINPolicy.Delete
	}
	if p == nil {
		return def
	}
	return *p
}

func defaultCloudRequiresPIN(a CloudAction) bool {
	return a == CloudDelete // delete sous PIN ; write autonome par défaut
}

// PathAllowed dit si un chemin cloud est visible par l'IA, selon un modèle AUTORISATIONS + EXCLUSIONS
// (v1.11) : le chemin est régi par le préfixe le PLUS SPÉCIFIQUE qui le couvre, parmi les AllowedDirs
// (autorise) et les DeniedDirs (retire). À spécificité égale, l'EXCLUSION l'emporte (prudence).
// Fail-closed : aucun préfixe couvrant ⇒ non autorisé. « / » autorisé = tout le CORPUS.
func (fc FileCloud) PathAllowed(p string) bool {
	np := CloudCleanPath(p)
	bestLen, bestAllow, matched := -1, false, false
	consider := func(d string, allow bool) {
		nd := CloudCleanPath(d)
		covers := nd == "/" || np == nd || strings.HasPrefix(np, nd+"/")
		if !covers {
			return
		}
		l := len(nd)
		if nd == "/" {
			l = 0 // la racine est le préfixe le MOINS spécifique
		}
		if l > bestLen || (l == bestLen && !allow) { // à égalité, l'exclusion gagne
			bestLen, bestAllow, matched = l, allow, true
		}
	}
	for _, d := range fc.AllowedDirs {
		consider(d, true)
	}
	for _, d := range fc.DeniedDirs {
		consider(d, false)
	}
	return matched && bestAllow
}

// CloudCleanPath normalise un chemin cloud : POSIX, slash initial, sans slash final, « .. » résolu.
func CloudCleanPath(p string) string {
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

// BackupPlan : sauvegarde d'un dépôt git local vers un dossier d'un cloud connecté (v1.10). Le
// backup est un bundle git (historique complet, un fichier restaurable). Exécution manuelle pour
// l'instant (planification automatique = étape suivante).
type BackupPlan struct {
	Name     string `json:"name"`
	RepoPath string `json:"repo_path,omitempty"` // (legacy mono-dépôt v1.10 ; vide pour les plans v1.14)
	Cloud    string `json:"cloud"`               // nom de la connexion cloud de destination
	Dest     string `json:"dest"`                // dossier de destination dans le cloud
	LastRun  string `json:"last_run,omitempty"`
	// --- Tunnel clé-en-main (v1.14) ---
	Type    string   `json:"type,omitempty"`    // "generic" (portée dynamique) | "targeted" (liste figée) ; vide = legacy
	Scope   string   `json:"scope,omitempty"`   // generic : "kernels" | "spokes" | "repos" | "kernels_spokes" | "all"
	Layer   string   `json:"layer,omitempty"`   // "local" | "atelier" | "both" (défaut "local")
	Form    string   `json:"form,omitempty"`    // "bundle" (léger, re-clonable) | "mirror" (fichiers lisibles) ; défaut "bundle"
	Routine string   `json:"routine,omitempty"` // "manual" | "on_open" | "on_close" (défaut "manual")
	Repos   []string `json:"repos,omitempty"`   // targeted : chemins locaux inscrits (absolus, nettoyés)
	// Rétention (v1.17). Overwrite=false (défaut, et plans existants) = CUMULATIF : chaque exécution
	// garde son bundle horodaté, rien n'est supprimé (le cloud grossit). Overwrite=true = ÉCRASE :
	// seul le dernier bundle reste à la racine du dépôt, les précédents sont déplacés dans un
	// sous-dossier « archive/ » dont on ne garde que les Archives plus récents (0 = aucune archive).
	Overwrite bool `json:"overwrite,omitempty"`
	Archives  int  `json:"archives,omitempty"` // 0..3 : nombre d'archives conservées quand Overwrite
}

// BackupRun : une entrée du JOURNAL de sauvegarde (v1.14). Non sensible.
type BackupRun struct {
	Plan    string `json:"plan"`
	When    string `json:"when"`              // RFC3339
	OK      bool   `json:"ok"`                // succès global
	Repos   int    `json:"repos"`             // nombre de dépôts traités
	Failed  int    `json:"failed,omitempty"`  // nombre d'échecs
	Form    string `json:"form,omitempty"`    // bundle | mirror
	Trigger string `json:"trigger,omitempty"` // manual | on_open | on_close
	Detail  string `json:"detail,omitempty"`  // résumé court / 1re erreur
}

// Kernel désigne un dépôt git local comme « mémoire » (KERNEL) de l'utilisateur (v1.11) : le dépôt
// qui EST la mémoire, distingué des dépôts logiciels ordinaires. Plusieurs KERNEL peuvent coexister
// (plusieurs sphères) ; un seul est « actif » à la fois (Config.ActiveKernel). Aucun secret : le
// jeton d'accès reste celui de l'instance git (trousseau), comme pour tout dépôt. La DÉSIGNATION
// (ajout/retrait/actif) est un geste de gouvernance réservé à la FENÊTRE (jamais socket/CLI/MCP).
type Kernel struct {
	Name      string `json:"name"`               // libellé humain (« Ma mémoire APCOM »)
	LocalPath string `json:"local_path"`         // clone local qui porte la mémoire
	Instance  string `json:"instance,omitempty"` // instance git pour récupérer/enregistrer (vide = défaut)
	Backup    string `json:"backup,omitempty"`   // nom d'un BackupPlan rattaché (sauvegarde vers un cloud CORPUS)
	RunFile   string `json:"run_file,omitempty"` // fichier d'ouverture pour l'IA locale (chemin relatif au dépôt)
}

// Spoke désigne un dépôt local comme PROJECTION d'une mission (SPOKE, v1.11) dérivée d'un KERNEL.
// Ce qui distingue un spoke d'un dépôt ordinaire : il référence le KERNEL dont il dérive et porte une
// mission (intention). La DÉSIGNATION est, comme pour le KERNEL, réservée à la FENÊTRE.
type Spoke struct {
	Name      string `json:"name"`
	LocalPath string `json:"local_path"`
	Kernel    string `json:"kernel,omitempty"`   // nom du KERNEL parent dont ce spoke dérive
	Mission   string `json:"mission,omitempty"`  // à quoi sert ce spoke (intention)
	Instance  string `json:"instance,omitempty"` // instance git pour récupérer/enregistrer (vide = défaut)
	RunFile   string `json:"run_file,omitempty"` // fichier d'ouverture (run) pour l'IA locale (chemin relatif)
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
	Tag         string           `json:"tag,omitempty"`  // groupe de classement libre (« tag projet »)
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
	// Clouds de fichiers souverains (CORPUS) et plans de sauvegarde git→cloud (v1.10).
	FileClouds []FileCloud  `json:"file_clouds,omitempty"`
	Backups    []BackupPlan `json:"backups,omitempty"`
	BackupRuns []BackupRun  `json:"backup_runs,omitempty"` // journal des sauvegardes (v1.14), plafonné
	// KERNEL (v1.11) : dépôts-mémoire désignés. ActiveKernel = nom du KERNEL courant.
	Kernels      []Kernel `json:"kernels,omitempty"`
	ActiveKernel string   `json:"active_kernel,omitempty"`
	// SPOKES (v1.11) : projections de mission dérivées d'un KERNEL.
	Spokes []Spoke `json:"spokes,omitempty"`
	// Appariements ARCHIVÉS (v1.11) : clés de lignes de la matrice « Organiser mes dépôts » que
	// l'utilisateur a rangées (suggestion jugée non pertinente). On les masque de « en attente » sans
	// rien supprimer ; l'utilisateur peut les repasser en appairage depuis l'archive.
	ArchivedPairings []string `json:"archived_pairings,omitempty"`
	// MODULES d'interface (v1.13) : groupes de fonctions activables/désactivables selon le profil
	// (utilisateur « porté » vs développeur). Clé = groupe (agents, git_advanced, spokes, corpus,
	// backup). Un groupe ABSENT de la map est ACTIVÉ par défaut (tout est montré tant qu'on n'a pas
	// réduit). On ne stocke donc que les groupes explicitement coupés (false) ou rallumés (true).
	Modules map[string]bool `json:"modules,omitempty"`

	// PolicySig : signature d'INTÉGRITÉ de la politique de sécurité (v1.16, fermeture P3). HMAC (clé
	// d'appareil) sur SecurityDigest() — les champs qui décident du PIN et du périmètre (rôle +
	// pin_policy par instance, allowed/denied_dirs + pin_policy par cloud, read_only). Écrite par
	// SituLab à chaque changement de politique ; vérifiée avant toute décision. Une réécriture directe
	// de config.json (hors SituLab) invalide la signature → le cœur retombe en position fermée (PIN
	// exigé, lecture seule). N'est PAS un secret.
	PolicySig string `json:"policy_sig,omitempty"`
}

// SecurityDigest renvoie une empreinte STABLE (indépendante de l'ordre) des champs de politique qui
// décident du PIN et du périmètre d'accès. Base de la signature d'intégrité (PolicySig, fermeture P3).
// Exclut PolicySig lui-même. Toute modification d'un de ces champs change l'empreinte.
func (c *Config) SecurityDigest() []byte {
	type instPol struct {
		Name string     `json:"n"`
		URL  string     `json:"u"` // l'hôte pilote RequiresPINForPush (prod via atelier) : il doit être intègre
		Role string     `json:"r"`
		PIN  *PINPolicy `json:"p"`
	}
	type cloudPol struct {
		Name    string          `json:"n"`
		Allowed []string        `json:"a"`
		Denied  []string        `json:"d"`
		PIN     *CloudPINPolicy `json:"p"`
	}
	is := make([]instPol, 0, len(c.Instances))
	for _, in := range c.Instances {
		is = append(is, instPol{strings.ToLower(strings.TrimSpace(in.Name)), strings.TrimSpace(in.URL), in.NormalizedRole(), in.PINPolicy})
	}
	sort.Slice(is, func(a, b int) bool { return is[a].Name < is[b].Name })
	cs := make([]cloudPol, 0, len(c.FileClouds))
	for _, fc := range c.FileClouds {
		a := append([]string(nil), fc.AllowedDirs...)
		d := append([]string(nil), fc.DeniedDirs...)
		sort.Strings(a)
		sort.Strings(d)
		cs = append(cs, cloudPol{strings.ToLower(strings.TrimSpace(fc.Name)), a, d, fc.PINPolicy})
	}
	sort.Slice(cs, func(a, b int) bool { return cs[a].Name < cs[b].Name })
	payload := struct {
		I  []instPol  `json:"i"`
		C  []cloudPol `json:"c"`
		RO bool       `json:"ro"`
	}{is, cs, c.ReadOnly}
	b, _ := json.Marshal(payload)
	h := sha256.Sum256(b)
	return h[:]
}

// ModuleOn indique si un groupe de modules est activé. Défaut : activé (absent = true).
func (c *Config) ModuleOn(key string) bool {
	if c.Modules == nil {
		return true
	}
	v, ok := c.Modules[key]
	if !ok {
		return true
	}
	return v
}

// ArchivePairing range une clé d'appariement (idempotent). Renvoie true si elle a été ajoutée.
func (c *Config) ArchivePairing(key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	for _, k := range c.ArchivedPairings {
		if k == key {
			return false
		}
	}
	c.ArchivedPairings = append(c.ArchivedPairings, key)
	return true
}

// UnarchivePairing retire une clé de l'archive. Renvoie true si elle y était.
func (c *Config) UnarchivePairing(key string) bool {
	out := c.ArchivedPairings[:0]
	found := false
	for _, k := range c.ArchivedPairings {
		if k == key {
			found = true
			continue
		}
		out = append(out, k)
	}
	c.ArchivedPairings = out
	return found
}

// IsPairingArchived indique si une clé d'appariement est archivée.
func (c *Config) IsPairingArchived(key string) bool {
	for _, k := range c.ArchivedPairings {
		if k == key {
			return true
		}
	}
	return false
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

// AgentsDocPath renvoie le chemin de la fiche AGENTS.md « administrée à la source » : une copie
// canonique, éditable, qui vit dans le dossier de configuration de SituLab et SURVIT aux mises à
// jour de l'application (le dossier de config n'est jamais touché par l'installateur, contrairement
// au fichier embarqué). C'est ce chemin que l'on communique à l'agent IA (v1.17).
func AgentsDocPath() string { return filepath.Join(Dir(), "AGENTS.md") }

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

// EqName est la version exportée d'eqName (comparaison de noms insensible à la casse/espaces).
func EqName(a, b string) bool { return eqName(a, b) }

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

// FindCloud renvoie la connexion cloud par nom (insensible à la casse), ou nil.
func (c *Config) FindCloud(name string) *FileCloud {
	for i := range c.FileClouds {
		if eqName(c.FileClouds[i].Name, name) {
			return &c.FileClouds[i]
		}
	}
	return nil
}

// DefaultCloud renvoie le cloud par défaut (ou le premier, ou nil).
func (c *Config) DefaultCloud() *FileCloud {
	for i := range c.FileClouds {
		if c.FileClouds[i].Default {
			return &c.FileClouds[i]
		}
	}
	if len(c.FileClouds) > 0 {
		return &c.FileClouds[0]
	}
	return nil
}

// ResolveCloud sélectionne un cloud par nom ; nom vide => cloud par défaut.
func (c *Config) ResolveCloud(name string) *FileCloud {
	if name == "" {
		return c.DefaultCloud()
	}
	return c.FindCloud(name)
}

// SetDefaultCloud marque un cloud comme défaut (et démarque les autres).
func (c *Config) SetDefaultCloud(name string) bool {
	found := false
	for i := range c.FileClouds {
		if eqName(c.FileClouds[i].Name, name) {
			c.FileClouds[i].Default = true
			found = true
		} else {
			c.FileClouds[i].Default = false
		}
	}
	return found
}

// RemoveCloud supprime une connexion cloud (insensible à la casse). Renvoie le nom réel supprimé
// (pour retirer le secret du coffre avec la bonne casse) et un booléen de succès.
func (c *Config) RemoveCloud(name string) (string, bool) {
	out := c.FileClouds[:0]
	removed, wasDefault := "", false
	for _, fc := range c.FileClouds {
		if eqName(fc.Name, name) {
			removed, wasDefault = fc.Name, fc.Default
			continue
		}
		out = append(out, fc)
	}
	c.FileClouds = out
	if removed == "" {
		return "", false
	}
	if wasDefault && len(c.FileClouds) > 0 {
		c.FileClouds[0].Default = true
	}
	return removed, true
}

// FindBackup renvoie un plan de sauvegarde par nom (insensible à la casse), ou nil.
func (c *Config) FindBackup(name string) *BackupPlan {
	for i := range c.Backups {
		if eqName(c.Backups[i].Name, name) {
			return &c.Backups[i]
		}
	}
	return nil
}

// RemoveBackup supprime un plan de sauvegarde par nom. Renvoie vrai si trouvé.
func (c *Config) RemoveBackup(name string) bool {
	out := c.Backups[:0]
	found := false
	for _, b := range c.Backups {
		if eqName(b.Name, name) {
			found = true
			continue
		}
		out = append(out, b)
	}
	c.Backups = out
	return found
}

// AddBackupRun ajoute une entrée au journal (en tête), plafonné aux 50 dernières.
func (c *Config) AddBackupRun(r BackupRun) {
	c.BackupRuns = append([]BackupRun{r}, c.BackupRuns...)
	if len(c.BackupRuns) > 50 {
		c.BackupRuns = c.BackupRuns[:50]
	}
}

// --- KERNEL (v1.11) ---------------------------------------------------------------------------

// FindKernel renvoie le KERNEL par nom (insensible à la casse), ou nil.
func (c *Config) FindKernel(name string) *Kernel {
	for i := range c.Kernels {
		if eqName(c.Kernels[i].Name, name) {
			return &c.Kernels[i]
		}
	}
	return nil
}

// ActiveKernelRef renvoie le KERNEL actif (ActiveKernel), à défaut le premier, ou nil.
func (c *Config) ActiveKernelRef() *Kernel {
	if c.ActiveKernel != "" {
		if k := c.FindKernel(c.ActiveKernel); k != nil {
			return k
		}
	}
	if len(c.Kernels) > 0 {
		return &c.Kernels[0]
	}
	return nil
}

// ResolveKernel sélectionne un KERNEL par nom ; nom vide => KERNEL actif.
func (c *Config) ResolveKernel(name string) *Kernel {
	if name == "" {
		return c.ActiveKernelRef()
	}
	return c.FindKernel(name)
}

// SetActiveKernel marque un KERNEL comme actif (nom canonique). Renvoie vrai si trouvé.
func (c *Config) SetActiveKernel(name string) bool {
	k := c.FindKernel(name)
	if k == nil {
		return false
	}
	c.ActiveKernel = k.Name
	return true
}

// RemoveKernel supprime un KERNEL par nom. Si c'était l'actif, promeut le premier restant.
func (c *Config) RemoveKernel(name string) bool {
	out := c.Kernels[:0]
	removed, wasActive := "", false
	for _, k := range c.Kernels {
		if eqName(k.Name, name) {
			removed, wasActive = k.Name, eqName(c.ActiveKernel, k.Name)
			continue
		}
		out = append(out, k)
	}
	c.Kernels = out
	if removed == "" {
		return false
	}
	if wasActive {
		c.ActiveKernel = ""
		if len(c.Kernels) > 0 {
			c.ActiveKernel = c.Kernels[0].Name
		}
	}
	return true
}

// --- SPOKES (v1.11) ---------------------------------------------------------------------------

// FindSpoke renvoie le spoke par nom (insensible à la casse), ou nil.
func (c *Config) FindSpoke(name string) *Spoke {
	for i := range c.Spokes {
		if eqName(c.Spokes[i].Name, name) {
			return &c.Spokes[i]
		}
	}
	return nil
}

// RemoveSpoke supprime un spoke par nom. Renvoie vrai si trouvé.
func (c *Config) RemoveSpoke(name string) bool {
	out := c.Spokes[:0]
	found := false
	for _, sp := range c.Spokes {
		if eqName(sp.Name, name) {
			found = true
			continue
		}
		out = append(out, sp)
	}
	c.Spokes = out
	return found
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
