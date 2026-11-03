// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package daemon

// Args porte les paramètres de toutes les opérations (seuls les champs utiles sont remplis).
type Args struct {
	Instance    string   `json:"instance,omitempty"`
	Owner       string   `json:"owner,omitempty"`
	Repo        string   `json:"repo,omitempty"`
	Ref         string   `json:"ref,omitempty"`
	Limit       int      `json:"limit,omitempty"`
	Path        string   `json:"path,omitempty"`
	Dir         string   `json:"dir,omitempty"`
	Message     string   `json:"message,omitempty"`
	Name        string   `json:"name,omitempty"`
	URL         string   `json:"url,omitempty"`
	Token       string   `json:"token,omitempty"`
	Desc        string   `json:"desc,omitempty"`
	Private     bool     `json:"private,omitempty"`
	Title       string   `json:"title,omitempty"`
	Body        string   `json:"body,omitempty"`
	State       string   `json:"state,omitempty"`
	User        string   `json:"user,omitempty"`
	Password    string   `json:"password,omitempty"`
	OTP         string   `json:"otp,omitempty"`
	Scopes      []string `json:"scopes,omitempty"`
	Repos       []string `json:"repos,omitempty"`
	ID          int64    `json:"id,omitempty"`
	Email       string   `json:"email,omitempty"`
	Provider    string   `json:"provider,omitempty"` // instance.add : "forgejo" (défaut) | "github"
	Index       int      `json:"index,omitempty"`    // numéro d'issue/PR
	Head        string   `json:"head,omitempty"`     // pull.create : branche source
	Base        string   `json:"base,omitempty"`     // pull.create : branche cible
	Method      string   `json:"method,omitempty"`   // pull.merge : merge|rebase|squash
	Mode        string   `json:"mode,omitempty"`     // project.promote : code|tag|release
	Event       string   `json:"event,omitempty"`    // pull.review : APPROVED|REQUEST_CHANGES|COMMENT
	Tag         string   `json:"tag,omitempty"`      // release/tag
	Target      string   `json:"target,omitempty"`   // release/tag : branche/commit cible
	Draft       bool     `json:"draft,omitempty"`
	Prerelease  bool     `json:"prerelease,omitempty"`
	Archived    bool     `json:"archived,omitempty"`
	Branch      string   `json:"branch,omitempty"`    // repo.default-branch
	Labels      []string `json:"labels,omitempty"`    // issue.labels (noms)
	Assignees   []string `json:"assignees,omitempty"` // issue.assignees (logins)
	MilestoneID int64    `json:"milestone_id,omitempty"`
	File        string   `json:"file,omitempty"`  // git.discard : fichier précis (vide = tout)
	Paths       []string `json:"paths,omitempty"` // git.commit-paths : staging sélectif
	Admin       bool     `json:"admin,omitempty"`
	AdminScope  bool     `json:"admin_scope,omitempty"` // onboarding : demander la portée admin
	Remember    bool     `json:"remember,omitempty"`    // onboarding : mémoriser le mot de passe (chiffré par le PIN)
	Force       bool     `json:"force,omitempty"`       // git.branch-delete / git.worktree-remove : forcer malgré refus git
	Create      bool     `json:"create,omitempty"`      // git.worktree-add : créer une NOUVELLE branche (sinon extraire l'existante)
	Intent      string   `json:"intent,omitempty"`      // Point 4 : phrase libre de l'agent (« pourquoi »), AFFICHÉE telle quelle dans la fenêtre PIN, distincte des actions constatées par SituLab. N'autorise rien.
	// Clouds de fichiers (v1.10)
	Cloud   string `json:"cloud,omitempty"`   // nom de la connexion cloud
	Content string `json:"content,omitempty"` // cloud.write : contenu texte à écrire
	Local   string `json:"local,omitempty"`   // cloud.upload : chemin local source ; cloud.get/download : destination locale
	Dest    string `json:"dest,omitempty"`    // backup.add : dossier cloud de destination
}

// Status est l'état renvoyé par OpStatus.
type Status struct {
	Unlocked  bool `json:"unlocked"`
	PINSet    bool `json:"pin_set"`
	Instances int  `json:"instances"`
}

// PINOutcome décrit le RÉSULTAT d'une demande de PIN, pour que le client (souvent un agent) distingue
// clairement l'état au lieu d'un échec opaque : confirmé / refusé par l'humain / délai écoulé / pas
// d'interface pour demander. L'invariant est préservé : le PIN se saisit dans la fenêtre, jamais ici.
type PINOutcome int

// PINPrompter demande le PIN à l'humain sur le poste. reason décrit l'action à autoriser.
// Renvoie (pin, PINConfirmed) si saisi, ("", …) sinon selon la cause (voir PINOutcome).
type PINPrompter interface {
	// detail porte un aperçu optionnel de l'action (ex. *core.PushPreview pour un envoi) afin
	// d'enrichir le consentement ; nil quand il n'y a rien de plus à montrer que `reason`.
	// action est une CLÉ de classe d'action (push|promote|delete|create|config|issue|admin|unlock)
	// pour que la fenêtre adapte son visuel (couleur/icône) — ce n'est jamais une autorisation en soi.
	// intent est la phrase LIBRE de l'agent (son « pourquoi »), affichée distinctement des actions que
	// SituLab constate ; vide quand l'humain agit directement. N'autorise rien non plus : pur affichage.
	PromptPIN(reason string, detail any, action, intent string) (string, PINOutcome)
}
