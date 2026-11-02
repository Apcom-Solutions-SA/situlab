// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package config

// PINAction : classe d'action distante soumise à la politique PIN par connexion (v1.7).
type PINAction string

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

	// Service de signature serveur (v1.9.6) : la clé privée ne quitte jamais le serveur (agent-proof).
	// C'est la SEULE voie d'attestation depuis 1.9.7 ; l'ancien champ sign_mode (local|server) a été
	// retiré en 1.18 (plus jamais lu — l'attestation serveur est gouvernée par Forgejo + jeton).
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
