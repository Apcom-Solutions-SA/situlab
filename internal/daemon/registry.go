// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package daemon

import "situlab/internal/config"

// Registre des opérations — SOURCE DE VÉRITÉ UNIQUE de la CLASSIFICATION d'une op (1.20).
//
// Avant : trois listes parallèles (isWriteOp, actionOf, actionLabel) qu'il fallait tenir à la main à
// chaque nouvelle op — un oubli = PIN manquant, reverrouillage manquant ou mauvais visuel. Désormais,
// une op de MUTATION a UNE entrée dans opRegistry, et les accesseurs en dérivent. Un test-golden
// (registry_golden_test.go) fige le comportement et un test de complétude interdit d'ajouter une op
// sans la classer.
//
// Les LECTURES pures ne figurent pas dans la table : elles retombent sur les défauts (write=false,
// ActionUserAdmin, « admin ») — exactement l'ancien comportement par défaut des trois switch.
type opMeta struct {
	write      bool             // mutation : refusée en lecture seule + reverrouillage-après-écriture + signal git
	mutatesGit bool             // change l'état git SANS être un write classique (ex. clone) → signal git quand même
	action     config.PINAction // classe de politique PIN (vide = défaut ActionUserAdmin)
	label      string           // clé de VISUEL de la fenêtre PIN (vide = défaut « admin »)
}

// opRegistry : une entrée par op de MUTATION (ou à signal git). Groupé par classe pour la lisibilité.
var opRegistry = map[string]opMeta{
	// Envoi de code (push/sync et dérivés) — politique ActionPush.
	OpPush:           {write: true, action: config.ActionPush, label: "push"},
	OpSync:           {write: true, action: config.ActionPush, label: "push"},
	OpGitPushForce:   {write: true, action: config.ActionPush, label: "push"},
	OpGitPushTag:     {write: true, action: config.ActionPush, label: "push"},
	OpKernelSave:     {write: true, action: config.ActionPush, label: "push"},
	OpSpokeSave:      {write: true, action: config.ActionPush, label: "push"},
	OpProjectPromote: {write: true, action: config.ActionPush, label: "promote"},

	// Issues / PR / revues — politique ActionIssues.
	OpIssueCreate:  {write: true, action: config.ActionIssues, label: "issue"},
	OpIssueComment: {write: true, action: config.ActionIssues, label: "issue"},
	OpIssueState:   {write: true, action: config.ActionIssues, label: "issue"},
	OpIssueEdit:    {write: true, action: config.ActionIssues, label: "issue"},
	OpIssueLabels:  {write: true, action: config.ActionIssues, label: "issue"},
	OpIssueAssign:  {write: true, action: config.ActionIssues, label: "issue"},
	OpIssueMilestn: {write: true, action: config.ActionIssues, label: "issue"},
	OpPullCreate:   {write: true, action: config.ActionIssues, label: "issue"},
	OpPullMerge:    {write: true, action: config.ActionIssues, label: "issue"},
	OpPullReview:   {write: true, action: config.ActionIssues, label: "issue"},
	OpPullState:    {write: true, action: config.ActionIssues, label: "issue"},
	OpPullEdit:     {write: true, action: config.ActionIssues, label: "issue"},

	// Administration de dépôt (créer/supprimer/config, releases, tags) — politique ActionRepoAdmin.
	OpRepoCreate:    {write: true, action: config.ActionRepoAdmin, label: "create"},
	OpReleaseNew:    {write: true, action: config.ActionRepoAdmin, label: "create"},
	OpTagNew:        {write: true, action: config.ActionRepoAdmin, label: "create"},
	OpRepoDelete:    {write: true, action: config.ActionRepoAdmin, label: "delete"},
	OpRepoDelBranch: {write: true, action: config.ActionRepoAdmin, label: "delete"},
	OpReleaseDel:    {write: true, action: config.ActionRepoAdmin, label: "delete"},
	OpTagDel:        {write: true, action: config.ActionRepoAdmin, label: "delete"},
	OpRepoSetDesc:   {write: true, action: config.ActionRepoAdmin, label: "config"},
	OpRepoRename:    {write: true, action: config.ActionRepoAdmin, label: "config"},
	OpRepoVis:       {write: true, action: config.ActionRepoAdmin, label: "config"},
	OpRepoArchive:   {write: true, action: config.ActionRepoAdmin, label: "config"},
	OpRepoDefBranch: {write: true, action: config.ActionRepoAdmin, label: "config"},

	// Administration de comptes / jetons — politique ActionUserAdmin.
	OpAdminUserNew: {write: true, action: config.ActionUserAdmin, label: "create"},
	OpTokenMgrNew:  {write: true, action: config.ActionUserAdmin, label: "create"},
	OpTokenMgrDel:  {write: true, action: config.ActionUserAdmin, label: "delete"},

	// Mutations dont le PIN/visuel retombe sur le défaut (ActionUserAdmin / « admin ») : le choix par
	// défaut (défini au cœur) suffit. Les écritures CLOUD passent par gateCloud (politique cloud), donc
	// action/label ne servent pas pour elles ; seul write=true compte (reverrouillage + signal).
	OpInstanceDef: {write: true},
	OpBackupRun:   {write: true},
	OpCloudMkdir:  {write: true},
	OpCloudUpload: {write: true},
	OpCloudWrite:  {write: true},
	OpCloudDelete: {write: true},

	// Non-write mais change l'état git local (nouveau dépôt suivi) → signal de rafraîchissement.
	OpClone: {mutatesGit: true},
}

// isWriteOp : l'op est-elle une MUTATION ? (lecture seule refusée, reverrouillage-après-écriture).
func isWriteOp(op string) bool { return opRegistry[op].write }

// actionOf : classe de politique PIN. Défaut prudent (op de lecture ou non classée) : ActionUserAdmin.
func actionOf(op string) config.PINAction {
	if m, ok := opRegistry[op]; ok && m.action != "" {
		return m.action
	}
	return config.ActionUserAdmin
}

// actionLabel : clé de VISUEL de la fenêtre PIN. Défaut : « admin ». N'autorise rien (pur habillage).
func actionLabel(op string) string {
	if m, ok := opRegistry[op]; ok && m.label != "" {
		return m.label
	}
	return "admin"
}

// signalsGitChange : l'op doit-elle déclencher un rafraîchissement de l'UI (événement « git:changed ») ?
// Tout write (mutation distante) + les gestes qui changent l'état git local sans verrou de dépôt (clone).
// Les gestes LOCAUX (commit, merge, branche…) signalent déjà via le verrou de dépôt dans le cœur.
func signalsGitChange(op string) bool {
	m := opRegistry[op]
	return m.write || m.mutatesGit
}
