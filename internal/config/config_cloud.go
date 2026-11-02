// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package config

import (
	"path"
	"strings"
)

// CloudAction : classe d'action cloud soumise à la politique PIN par connexion (v1.10).
type CloudAction string

// CloudOpLevel : niveau tri-état d'une opération cloud pour l'IA (v1.18.2). "" = hérite du niveau
// parent (règle de chemin plus large, puis cloud, puis défaut). Remplace le binaire sans PIN|PIN par
// un modèle sans PIN | PIN | interdiction. L'interdiction est fail-closed : l'agent est refusé AVANT
// toute fenêtre PIN (il ne peut même pas tenter) ; l'humain en fenêtre reste l'autorité.
type CloudOpLevel string

// CloudPINPolicy : politique de droits cloud au niveau de la connexion (v1.10, étendue v1.18.2).
// Write/Delete *bool restent LUS en migration-repli (true→pin, false→none) ; les champs *Level,
// écrits par la modale, les remplacent et ajoutent le niveau « interdiction » + la classe « rights ».
// Réglée EN FENÊTRE uniquement, comme la politique PIN git.
type CloudPINPolicy struct {
	Write       *bool        `json:"write,omitempty"`        // LEGACY (lu en repli, plus écrit)
	Delete      *bool        `json:"delete,omitempty"`       // LEGACY (lu en repli, plus écrit)
	WriteLevel  CloudOpLevel `json:"write_level,omitempty"`  // v1.18.2
	DeleteLevel CloudOpLevel `json:"delete_level,omitempty"` // v1.18.2
	RightsLevel CloudOpLevel `json:"rights_level,omitempty"` // v1.18.2 (défaut : interdiction à l'IA)
}

// CloudPathRule : surcharge de droits sur un chemin précis (dossier ou fichier) d'un cloud (v1.18.2).
// La résolution suit le préfixe le PLUS SPÉCIFIQUE ; à spécificité égale, le niveau le plus restrictif
// gagne (forbid > pin > none), cohérent avec « l'exclusion gagne » de PathAllowed.
type CloudPathRule struct {
	Path   string       `json:"path"`
	Write  CloudOpLevel `json:"write,omitempty"`
	Delete CloudOpLevel `json:"delete,omitempty"`
	Rights CloudOpLevel `json:"rights,omitempty"`
}

// FileCloud décrit une connexion à un cloud de fichiers souverain (WebDAV). Aucun secret : le mot
// de passe (ou app-password) est chiffré au trousseau sous l'id « filecloud/<Name> ».
type FileCloud struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"`             // "nextcloud" | "kdrive"
	Method string `json:"method,omitempty"` // "" | "webdav" (défaut) | "rest" (kDrive via API REST Infomaniak, v1.18.2)
	URL    string `json:"url"`              // base WebDAV (ex. https://host/remote.php/dav/files/user) ; pour rest : id du drive
	User   string `json:"user"`
	// DriveID : identifiant du drive kDrive (méthode rest). Vide pour WebDAV.
	DriveID      string          `json:"drive_id,omitempty"`
	Default      bool            `json:"default,omitempty"`
	AllowedDirs  []string        `json:"allowed_dirs,omitempty"` // chemins AUTORISÉS à l'IA (dossier/fichier) ; vide = aucun
	DeniedDirs   []string        `json:"denied_dirs,omitempty"`  // chemins RETIRÉS sous un parent autorisé (exclusions fines, v1.11)
	PINPolicy    *CloudPINPolicy `json:"pin_policy,omitempty"`
	PathPolicies []CloudPathRule `json:"path_policies,omitempty"` // surcharges de droits par chemin (v1.18.2)
}

// AccessMethod renvoie la méthode d'accès effective : "rest" pour un kDrive explicitement en REST,
// "webdav" sinon (défaut rétro-compatible pour toutes les connexions existantes).
func (fc FileCloud) AccessMethod() string {
	if fc.Method == "rest" {
		return "rest"
	}
	return "webdav"
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

// defaultCloudLevel : niveau par défaut d'une opération quand rien n'est explicitement choisi.
// write = sans PIN (dépôt de résumés), delete = PIN (geste destructif), rights = interdiction à
// l'IA (un agent ne modifie jamais les droits).
func defaultCloudLevel(a CloudAction) CloudOpLevel {
	switch a {
	case CloudDelete:
		return CloudLevelPIN
	case CloudRights:
		return CloudLevelForbid
	default:
		return CloudLevelNone
	}
}

// cloudLevelAt : niveau d'une CloudPINPolicy pour une action, migration-repli depuis les *bool.
func (p *CloudPINPolicy) levelFor(a CloudAction) CloudOpLevel {
	if p == nil {
		return CloudLevelInherit
	}
	var lv CloudOpLevel
	var legacy *bool
	switch a {
	case CloudWrite:
		lv, legacy = p.WriteLevel, p.Write
	case CloudDelete:
		lv, legacy = p.DeleteLevel, p.Delete
	case CloudRights:
		lv = p.RightsLevel
	}
	if lv != CloudLevelInherit {
		return lv
	}
	if legacy != nil { // migration-repli : true→pin, false→none
		if *legacy {
			return CloudLevelPIN
		}
		return CloudLevelNone
	}
	return CloudLevelInherit
}

func (r CloudPathRule) levelFor(a CloudAction) CloudOpLevel {
	switch a {
	case CloudWrite:
		return r.Write
	case CloudDelete:
		return r.Delete
	case CloudRights:
		return r.Rights
	}
	return CloudLevelInherit
}

// moreRestrictive classe les niveaux : forbid > pin > none (le plus restrictif gagne à égalité).
func moreRestrictive(a, b CloudOpLevel) CloudOpLevel {
	rank := func(l CloudOpLevel) int {
		switch l {
		case CloudLevelForbid:
			return 3
		case CloudLevelPIN:
			return 2
		case CloudLevelNone:
			return 1
		}
		return 0
	}
	if rank(a) >= rank(b) {
		return a
	}
	return b
}

// EffectiveCloudLevel résout le niveau d'une opération pour un chemin : règle de chemin la PLUS
// SPÉCIFIQUE qui définit l'op gagne (à spécificité égale, le plus restrictif), sinon le niveau de la
// connexion, sinon le défaut. Fail-closed : rien ne surclasse jamais le défaut vers plus permissif
// sans règle explicite. Miroir de PathAllowed pour la granularité.
func (fc FileCloud) EffectiveCloudLevel(p string, a CloudAction) CloudOpLevel {
	np := CloudCleanPath(p)
	bestLen, best, matched := -1, CloudLevelInherit, false
	for _, rule := range fc.PathPolicies {
		lv := rule.levelFor(a)
		if lv == CloudLevelInherit {
			continue
		}
		nd := CloudCleanPath(rule.Path)
		covers := nd == "/" || np == nd || strings.HasPrefix(np, nd+"/")
		if !covers {
			continue
		}
		l := len(nd)
		if nd == "/" {
			l = 0
		}
		if l > bestLen {
			bestLen, best, matched = l, lv, true
		} else if l == bestLen {
			best = moreRestrictive(best, lv)
		}
	}
	if matched {
		return best
	}
	if lv := fc.PINPolicy.levelFor(a); lv != CloudLevelInherit {
		return lv
	}
	return defaultCloudLevel(a)
}

// RequiresPINCloudAt : l'opération exige-t-elle le PIN sur ce chemin ?
func (fc FileCloud) RequiresPINCloudAt(p string, a CloudAction) bool {
	return fc.EffectiveCloudLevel(p, a) == CloudLevelPIN
}

// CloudOpForbiddenAt : l'opération est-elle formellement interdite à l'IA sur ce chemin ?
func (fc FileCloud) CloudOpForbiddenAt(p string, a CloudAction) bool {
	return fc.EffectiveCloudLevel(p, a) == CloudLevelForbid
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
