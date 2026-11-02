// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package daemon

import (
	"errors"
	"situlab/internal/config"
	"situlab/internal/core"
	"strings"
)

// kindOf classe une erreur pour que le client CLI retrouve son code de sortie stable.
func kindOf(err error) string {
	switch {
	case errors.Is(err, core.ErrNoInstance):
		return "noinstance"
	case errors.Is(err, core.ErrPINNeeded):
		return "pinneeded"
	case errors.Is(err, core.ErrPINTimeout):
		return "pintimeout"
	case errors.Is(err, core.ErrPINLocked):
		return "pinlocked"
	case errors.Is(err, core.ErrNoGit):
		return "nogit"
	case errors.Is(err, core.ErrLocked):
		return "locked"
	case errors.Is(err, core.ErrCloudOperationForbidden):
		return "cloudforbidden-op"
	default:
		return "generic"
	}
}

// ensureUnlocked garantit une session de lecture : si verrouillée, la modale demande le PIN (invite
// générique de déverrouillage, pour les lectures).
func (d *Daemon) ensureUnlocked() error {
	return d.ensureUnlockedCtx("Déverrouiller SituLab", nil, "unlock", "")
}

// ensureUnlockedCtx est ensureUnlocked AVEC le contexte de l'action en attente. Quand la politique
// n'exige pas le PIN mais que la session est verrouillée, l'invite UNIQUE montre l'action + aperçu
// (Point 5 : un seul PIN quand verrouillé, et le BON message) au lieu d'un générique « Déverrouiller ».
// Le même secret déverrouille ET ouvre la session (core.Unlock→openSession) : une seule saisie.
func (d *Daemon) ensureUnlockedCtx(reason string, detail any, action, intent string) error {
	if d.svc.Unlocked() {
		return nil
	}
	if !d.svc.PINSet() {
		return errors.New("aucun PIN configuré : définissez-en un dans la fenêtre SituLab")
	}
	pin, outcome := d.prompter.PromptPIN(reason, detail, action, intent)
	switch outcome {
	case PINConfirmed:
		return d.svc.Unlock(pin)
	case PINTimedOut:
		return core.ErrPINTimeout
	default: // refusé / indisponible
		return core.ErrLocked
	}
}

// writePIN obtient le PIN d'autorisation d'une écriture EXCLUSIVEMENT via la modale sur le
// poste : le PIN ne transite jamais par le socket (invariant). Modale refusée/indisponible
// => ErrPINNeeded (code 3). C'est le point de consentement humain, non contournable par le
// client (agent) même s'il connaît le PIN.
// gate applique la politique PIN par connexion (v1.7) pour une opération d'écriture : selon le
// rôle de l'instance et sa politique, soit on demande le PIN à l'humain (writePIN, avec `reason`),
// soit la session déverrouillée suffit (ensureUnlocked, pas de PIN). Le cœur revérifie la même
// politique (fail-closed), donc sauter la modale ici ne peut jamais SUR-autoriser.
func (d *Daemon) gate(op string, a Args, reason string) (string, error) {
	act := actionOf(op)
	action := actionLabel(op)
	intent := clampIntent(a.Intent) // phrase libre de l'agent (Point 4), bornée ; affichage seul
	needPIN := d.svc.RequiresPIN(a.Instance, act)
	if act == config.ActionPush { // la politique du push suit le DÉPÔT réel, pas la connexion nommée
		needPIN = d.svc.RequiresPINForPush(a.Instance, a.Path)
	}
	var detail any
	if act == config.ActionPush && a.Path != "" { // aperçu éclairé pour un envoi/synchro
		detail = d.svc.PushPreview(a.Path)
	}
	if needPIN {
		return d.writePIN(reason, detail, action, intent)
	}
	// Politique « sans PIN » mais session verrouillée : l'invite UNIQUE de déverrouillage porte le
	// contexte de l'action (Point 5), pas un générique « Déverrouiller SituLab ».
	if err := d.ensureUnlockedCtx(reason, detail, action, intent); err != nil {
		return "", err
	}
	return "", nil
}

// clampIntent borne la phrase d'intention de l'agent (affichée dans la fenêtre PIN). On protège la
// modale d'un texte démesuré ; l'échappement du contenu (XSS) est fait côté frontend à l'affichage.
func clampIntent(s string) string {
	s = strings.TrimSpace(s)
	const max = 400
	if r := []rune(s); len(r) > max {
		return string(r[:max]) + "…"
	}
	return s
}

func (d *Daemon) writePIN(reason string, detail any, action, intent string) (string, error) {
	if !d.svc.PINSet() {
		return "", errors.New("aucun PIN configuré : définissez-en un d'abord")
	}
	pin, outcome := d.prompter.PromptPIN(reason, detail, action, intent)
	switch outcome {
	case PINConfirmed:
		return pin, nil
	case PINTimedOut:
		return "", core.ErrPINTimeout
	default: // refusé / indisponible
		return "", core.ErrPINNeeded
	}
}

// gateCloud applique la politique PIN de la connexion CLOUD pour une mutation (même modèle que
// gate() côté git, mais sur la politique cloud write/delete). Le cœur revérifie (fail-closed).
func (d *Daemon) gateCloud(op string, a Args, finalPath, reason string) (string, error) {
	act := config.CloudWrite
	label := "create"
	if op == OpCloudDelete {
		act = config.CloudDelete
		label = "delete"
	}
	// Interdiction formelle (v1.18.2) : refus À LA FRONTIÈRE AGENT, AVANT toute fenêtre PIN. L'agent
	// ne peut même pas tenter ; l'humain, lui, agit en fenêtre via app.go (non soumis à cette borne).
	if d.svc.CloudOpForbidden(a.Cloud, finalPath, act) {
		return "", core.ErrCloudOperationForbidden
	}
	intent := clampIntent(a.Intent)
	if d.svc.CloudRequiresPINAt(a.Cloud, finalPath, act) {
		return d.writePIN(reason, nil, label, intent)
	}
	if err := d.ensureUnlockedCtx(reason, nil, label, intent); err != nil {
		return "", err
	}
	return "", nil
}

// cloudGuard impose l'allowlist de dossiers À LA FRONTIÈRE AGENT (CLI/MCP passent par le démon).
// La fenêtre (humain) appelle le cœur en direct (app.go) et n'est donc pas soumise à cette borne :
// c'est l'humain qui CHOISIT le périmètre, l'agent qui s'y tient. Fail-closed.
func (d *Daemon) cloudGuard(cloud, p string) error {
	if !d.svc.CloudPathAllowed(cloud, p) {
		return core.ErrCloudForbidden
	}
	return nil
}

// filterProjectCorpus borne le mapping CORPUS d'une carte projet à l'allowlist IA (frontière agent).
// Chaque chemin relié hors allowlist est retiré ; un lien devenu vide disparaît. Fail-closed.
func (d *Daemon) filterProjectCorpus(views []core.ProjectLightView) []core.ProjectLightView {
	for i := range views {
		if len(views[i].Corpus) == 0 {
			continue
		}
		links := views[i].Corpus[:0]
		for _, l := range views[i].Corpus {
			paths := l.Paths[:0]
			for _, p := range l.Paths {
				if d.svc.CloudPathAllowed(l.Cloud, p.Path) {
					paths = append(paths, p)
				}
			}
			if len(paths) > 0 {
				l.Paths = paths
				links = append(links, l)
			}
		}
		views[i].Corpus = links
	}
	return views
}
