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

const (
	ActionPush      PINAction = "push"       // envoi de code (push, sync)
	ActionIssues    PINAction = "issues"     // issues, PR, commentaires, labels, revues…
	ActionRepoAdmin PINAction = "repo_admin" // créer/supprimer/renommer/visibilité/archiver/branche défaut, releases/tags
	ActionUserAdmin PINAction = "user_admin" // administration des comptes, gestion des jetons
)

// Types de cloud autorisés — souverains uniquement. Les clouds propriétaires non souverains
// (OneDrive/Google Drive/Dropbox) sont écartés par principe : ils ne sont pas proposés.
const (
	CloudNextcloud = "nextcloud" // Nextcloud auto-hébergé (souverain autonome)
	CloudKDrive    = "kdrive"    // kDrive Infomaniak (souverain délégué, droit suisse)
)

const (
	CloudWrite  CloudAction = "write"  // envoyer/modifier un fichier, créer un dossier
	CloudDelete CloudAction = "delete" // supprimer un fichier ou un dossier
	CloudRights CloudAction = "rights" // modifier les droits eux-mêmes (classe réservée FENÊTRE, v1.18.2)
)

const (
	CloudLevelInherit CloudOpLevel = ""       // hérite (chemin parent → cloud → défaut)
	CloudLevelNone    CloudOpLevel = "none"   // autorisé sans PIN (session déverrouillée)
	CloudLevelPIN     CloudOpLevel = "pin"    // exige le PIN
	CloudLevelForbid  CloudOpLevel = "forbid" // interdiction formelle à l'IA (refus avant prompt)
)

// Config est l'état persistant de SituLab (aucun secret).
type Config struct {
	Instances      []Instance `json:"instances"`
	LockAfterMin   int        `json:"lock_after_min,omitempty"`   // auto-verrouillage (minutes) ; 0 = défaut, -1 = jamais
	LockAfterWrite bool       `json:"lock_after_write,omitempty"` // reverrouille après chaque écriture réussie
	ReadOnly       bool       `json:"read_only,omitempty"`        // mode lecture seule : bloque toute mutation (v1.4)
	JournalLevel   string     `json:"journal_level,omitempty"`    // journal local des opérations : off | errors (défaut) | all (1.20)
	AgentNotes     string     `json:"agent_notes,omitempty"`      // fiche de collaboration avec l'IA (v1.5.1)
	Projects       []Project  `json:"projects,omitempty"`         // liens atelier↔production (v1.7.1)
	TagDefs        []TagDef   `json:"tag_defs,omitempty"`         // registre des tags projet : couleur par tag (v1.18.2)
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
	// Suggestions manuelles d'appairage atelier→vitrine (noms différents, ex. glim↔glim_v2) — l'IA/
	// l'utilisateur ASSEMBLE la paire ; la confirmation reste un geste explicite (bouton « Appairer »),
	// qui la transforme en Project. (1.18.1)
	VitrineHints []VitrineHint `json:"vitrine_hints,omitempty"`
	// MODULES d'interface (v1.13) : groupes de fonctions activables/désactivables selon le profil
	// (utilisateur « porté » vs développeur). Clé = groupe (agents, git_advanced, spokes, corpus,
	// backup). Un groupe ABSENT de la map est ACTIVÉ par défaut (tout est montré tant qu'on n'a pas
	// réduit). On ne stocke donc que les groupes explicitement coupés (false) ou rallumés (true).
	Modules map[string]bool `json:"modules,omitempty"`

	// ClosePurgeAgents (1.18.5) : clés des agents IA (knownAgents) pour lesquels l'utilisateur veut une
	// purge PROPOSÉE à la fermeture de SituLab (modale intelligente). Vide = aucune proposition.
	ClosePurgeAgents []string `json:"close_purge_agents,omitempty"`

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
	type pathPol struct {
		Path   string       `json:"p"`
		Write  CloudOpLevel `json:"w,omitempty"`
		Delete CloudOpLevel `json:"d,omitempty"`
		Rights CloudOpLevel `json:"r,omitempty"`
	}
	type cloudPol struct {
		Name    string          `json:"n"`
		Kind    string          `json:"k"`            // type de cloud (nextcloud/kdrive) : partie de l'identité signée
		Method  string          `json:"m,omitempty"`  // méthode d'accès (webdav/rest) : partie de l'identité signée (v1.18.2)
		DriveID string          `json:"di,omitempty"` // identifiant du drive (kDrive REST) : signé, sinon redirection silencieuse possible (v1.18.2)
		URL     string          `json:"u"`            // DESTINATION du mot de passe d'application : doit être intègre (anti-redirection des identifiants vers un hôte attaquant)
		User    string          `json:"us"`
		Allowed []string        `json:"a"`
		Denied  []string        `json:"d"`
		PIN     *CloudPINPolicy `json:"p"`
		Paths   []pathPol       `json:"pp,omitempty"` // surcharges de droits par chemin (v1.18.2) : signées, sinon élévation IA silencieuse
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
		pp := make([]pathPol, 0, len(fc.PathPolicies))
		for _, r := range fc.PathPolicies {
			pp = append(pp, pathPol{CloudCleanPath(r.Path), r.Write, r.Delete, r.Rights})
		}
		sort.Slice(pp, func(i, j int) bool { return pp[i].Path < pp[j].Path })
		cs = append(cs, cloudPol{strings.ToLower(strings.TrimSpace(fc.Name)), strings.TrimSpace(fc.Kind), strings.TrimSpace(fc.AccessMethod()), strings.TrimSpace(fc.DriveID), strings.TrimSpace(fc.URL), strings.TrimSpace(fc.User), a, d, fc.PINPolicy, pp})
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

// Prompt : un prompt réutilisable pour l'assistant IA (titre + corps).
type Prompt struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Body  string `json:"body"`
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
