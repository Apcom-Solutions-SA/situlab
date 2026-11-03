// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"errors"
	"fmt"
	"situlab/internal/gitx"
	"strings"
	"time"
)

// Clone clone owner/repo depuis l'instance vers dir (token injecté pour un dépôt privé).
func (s *Service) Clone(instance, owner, repo, dir string) (string, error) {
	if err := s.assertWritable(); err != nil {
		return "", err
	}
	if !gitx.Available() {
		return "", ErrNoGit
	}
	r, err := s.ViewRepo(instance, owner, repo)
	if err != nil {
		return "", err
	}
	out, err := gitx.Clone(r.CloneURL, dir, s.instanceToken(instance), s.isGitHub(instance), s.instanceBaseURL(instance))
	if err != nil {
		return "", err
	}
	_ = s.AddLocalRepo(dir) // on suit automatiquement le nouveau clone
	return out, nil
}

// Fetch récupère les nouveautés distantes (lecture — pas de PIN).
func (s *Service) Fetch(instance, path string) (string, error) {
	if !gitx.Available() {
		return "", ErrNoGit
	}
	defer s.lockRepo(path)() // pas de fetch de fond concurrent sur ce dépôt
	out, err := gitx.Fetch(path, s.instanceToken(instance), s.isGitHub(instance), s.instanceBaseURL(instance))
	if err == nil {
		s.fetchAttestNotes(instance, path) // récupère les attestations des autres (best-effort)
	}
	return out, err
}

// Pull met à jour la branche locale en fast-forward (pas de PIN : rien ne part en ligne, mais
// l'arbre de travail est modifié → refusé en lecture seule).
func (s *Service) Pull(instance, path string) (string, error) {
	if !gitx.Available() {
		return "", ErrNoGit
	}
	if err := s.assertWritable(); err != nil {
		return "", err
	}
	defer s.lockRepo(path)() // pas de fetch de fond concurrent sur ce dépôt
	out, err := gitx.Pull(path, s.instanceToken(instance), s.isGitHub(instance), s.instanceBaseURL(instance), false)
	if err == nil {
		s.fetchAttestNotes(instance, path) // récupère les attestations des autres (best-effort)
	}
	return out, err
}

// Commit enregistre une version locale (pas de PIN : rien ne part en ligne ; refusé en lecture seule).
func (s *Service) Commit(path, message string) (string, error) {
	if !gitx.Available() {
		return "", ErrNoGit
	}
	if err := s.assertWritable(); err != nil {
		return "", err
	}
	defer s.lockRepo(path)() // pas de fetch de fond concurrent pendant l'enregistrement
	out, err := gitx.Commit(path, message)
	if err == nil {
		s.attestCommit(path, gitx.HeadSHA(path)) // signature de garantie (best-effort, non bloquant)
	}
	return out, asConflict(err)
}

// asConflict remappe le refus d'enregistrement pour marques de conflit (gitx.ErrConflicted) sur
// le message guidé stable du cœur (ErrConflict), pour un retour cohérent sur toutes les surfaces.
func asConflict(err error) error {
	if errors.Is(err, gitx.ErrConflicted) {
		return ErrConflict
	}
	return err
}

// Diff renvoie le diff des modifications non enregistrées d'un dépôt local.
func (s *Service) Diff(path string) (string, error) {
	if !gitx.Available() {
		return "", ErrNoGit
	}
	return gitx.Diff(path)
}

// Push pousse un dépôt local vers le distant. Le PIN est OBLIGATOIRE et protégé par
// un verrou anti-bruteforce (consentement humain garanti).
func (s *Service) Push(instance, path, pinCode string) (string, error) {
	if !gitx.Available() {
		return "", ErrNoGit
	}
	if err := s.authorizePush(instance, path, pinCode); err != nil {
		return "", err
	}
	defer s.lockRepo(path)() // pas de fetch de fond concurrent sur ce dépôt
	out, err := gitx.Push(path, s.instanceToken(instance), s.isGitHub(instance), s.instanceBaseURL(instance))
	if err == nil {
		s.pushAttestNotes(instance, path) // rend la signature de garantie visible en ligne (best-effort)
	}
	return out, err
}

// SyncResult décrit, en clair, ce que Synchroniser a réalisé — pour un retour non technique.
type SyncResult struct {
	Committed bool   `json:"committed"` // une version a été enregistrée localement
	Pulled    bool   `json:"pulled"`    // des nouveautés ont été récupérées du serveur
	Pushed    bool   `json:"pushed"`    // les enregistrements locaux sont partis en ligne
	Nothing   bool   `json:"nothing"`   // rien à faire : déjà à jour
	Summary   string `json:"summary"`   // phrase lisible résumant l'opération
}

// Synchronize est le geste unique « Synchroniser » des non-développeurs : récupérer les
// nouveautés du serveur, enregistrer les modifications en cours, puis envoyer. Le PIN est
// OBLIGATOIRE (un envoi en ligne a lieu) et vérifié UNE SEULE FOIS pour toute la séquence. Les cas
// délicats renvoient une erreur guidée stable (ErrDiverged, ErrDetached, ErrNoRemote…), une phrase
// claire plutôt qu'une sortie git brute. Si message est vide, un libellé daté est utilisé pour
// l'enregistrement.
//
// ORDRE : on récupère AVANT d'enregistrer. SÛRETÉ (v1.5.2) : Synchroniser ne produit JAMAIS de
// fusion susceptible de laisser des marques de conflit — l'avance rapide n'a lieu que si l'arbre
// est propre ; « en retard + modifications en cours » et « commits des deux côtés » s'arrêtent avec
// un message guidé (ErrDirtyBehind, ErrDiverged) ; un conflit préexistant (ErrConflict) interdit
// tout enregistrement.
func (s *Service) Synchronize(instance, path, message, pinCode string) (*SyncResult, error) {
	if !gitx.Available() {
		return nil, ErrNoGit
	}
	if err := s.authorizePush(instance, path, pinCode); err != nil { // politique liée au dépôt réel ; une seule confirmation pour tout le geste
		return nil, err
	}
	defer s.lockRepo(path)() // tout le geste récupérer→enregistrer→envoyer tient le verrou du dépôt
	st, err := gitx.StatusOf(path)
	if err != nil {
		return nil, err
	}
	if !st.IsRepo {
		return nil, ErrNotARepo
	}
	if st.Detached {
		return nil, ErrDetached
	}
	if !gitx.HasRemote(path) {
		return nil, ErrNoRemote
	}
	res := &SyncResult{}
	token, github, base := s.instanceToken(instance), s.isGitHub(instance), s.instanceBaseURL(instance)

	// 1) Récupérer d'abord. On relit l'état APRÈS fetch (l'avance/retard n'est fiable qu'une fois
	//    les refs distantes rapatriées) et on CAPTURE l'erreur de relecture : un `git status` qui
	//    échoue ne doit pas être lu comme « rien à faire » et déclencher un envoi injustifié.
	if _, err := gitx.Fetch(path, token, github, base); err != nil {
		return nil, err
	}
	_, _ = gitx.FetchNotes(path, attestNotesRef, token, github, base) // attestations des autres (best-effort)
	st2, err := gitx.StatusOf(path)
	if err != nil {
		return nil, err
	}
	if len(st2.Conflicted) > 0 {
		return nil, ErrConflict // conflit préexistant : on n'enregistre JAMAIS des marques de conflit
	}
	if st2.Behind > 0 {
		if st2.Ahead > 0 {
			return nil, ErrDiverged // vrais changements croisés : commits des deux côtés
		}
		if !st2.Clean {
			// En retard ET modifications non enregistrées : une avance rapide écraserait ou
			// entrerait en conflit avec le travail en cours. On s'arrête proprement (jamais de
			// fusion automatique qui laisserait des marques de conflit) et on guide.
			return nil, ErrDirtyBehind
		}
		// Arbre propre et en retard : avance rapide sûre (aucun conflit possible).
		if _, err := gitx.Pull(path, token, github, base, false); err != nil {
			return nil, err
		}
		res.Pulled = true
	}

	// 2) Enregistrer les modifications en cours, APRÈS avoir intégré le distant. Garde-fou : si un
	//    conflit est apparu, on NE commite PAS — on s'arrête et on guide (jamais de marques de
	//    conflit enregistrées ni publiées).
	st3, err := gitx.StatusOf(path)
	if err != nil {
		return nil, err
	}
	if len(st3.Conflicted) > 0 {
		return nil, ErrConflict
	}
	if !st3.Clean {
		msg := strings.TrimSpace(message)
		if msg == "" {
			msg = "Modifications du " + time.Now().Format("02/01/2006 15:04")
		}
		if _, err := gitx.Commit(path, msg); err != nil {
			return nil, asConflict(err)
		}
		s.attestCommit(path, gitx.HeadSHA(path)) // signature de garantie (best-effort)
		res.Committed = true
	}

	// 3) Envoyer : seulement sur un signal POSITIF (on est en avance, ou la branche n'a pas encore
	//    d'amont et doit être publiée) — jamais sur une valeur par défaut issue d'une lecture ratée.
	st4, err := gitx.StatusOf(path)
	if err != nil {
		return nil, err
	}
	if st4.Ahead > 0 || st4.Upstream == "" {
		if _, err := gitx.Push(path, token, github, base); err != nil {
			return nil, fmt.Errorf("enregistré sur ce poste, mais l'envoi en ligne a échoué : %w", err)
		}
		_, _ = gitx.PushNotes(path, attestNotesRef, token, github, base) // publie la signature de garantie (best-effort)
		res.Pushed = true
	}

	res.Nothing = !res.Committed && !res.Pulled && !res.Pushed
	res.Summary = syncSummary(res)
	return res, nil
}

// syncSummary compose une phrase lisible à partir de ce qui a été fait.
func syncSummary(r *SyncResult) string {
	if r.Nothing {
		return "Déjà à jour, rien à synchroniser."
	}
	var parts []string
	if r.Committed {
		parts = append(parts, "version enregistrée")
	}
	if r.Pulled {
		parts = append(parts, "nouveautés récupérées")
	}
	if r.Pushed {
		parts = append(parts, "envoyé en ligne")
	}
	return "Synchronisé : " + strings.Join(parts, ", ") + "."
}
