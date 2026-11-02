// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"errors"
	"situlab/internal/config"
	"situlab/internal/forgejo"
	"situlab/internal/keyring"
	"sort"
	"strings"
)

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
