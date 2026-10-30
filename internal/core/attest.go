// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"situlab/internal/attest"
	"situlab/internal/config"
	"situlab/internal/gitx"
	"situlab/internal/keyring"
)

// attestNotesRef : ref de notes git qui portait les attestations v1 (legacy) et porte encore celles du
// mode serveur (signées sur le SHA, non transposables en trailer). Le portage par défaut depuis 1.9.7
// est le TRAILER de commit « SituLab-Approved: » (immuable, voyage avec le commit) — cf. attestTrailerKey.
const attestNotesRef = "situlab"

// attestTrailerKey : clé du trailer de commit qui porte l'attestation v2 (immuable, dans l'objet commit).
const attestTrailerKey = "SituLab-Approved"

// signCertFor charge le certificat d'appareil d'une instance (config, non secret), ou nil.
func (s *Service) signCertFor(instance string) *attest.Cert {
	cfg, err := config.Load()
	if err != nil {
		return nil
	}
	in := cfg.Resolve(instance)
	if in == nil || in.SignCert == "" {
		return nil
	}
	var c attest.Cert
	if json.Unmarshal([]byte(in.SignCert), &c) != nil {
		return nil
	}
	return &c
}

// signKeyFor charge la clé PRIVÉE d'appareil d'une instance depuis le trousseau (session ouverte).
// L'appelant efface le privé (zero). nil si pas de clé, session fermée ou trousseau illisible.
func (s *Service) signKeyFor(instance string) ed25519.PrivateKey {
	dek, err := s.sessionDEK()
	if err != nil {
		return nil
	}
	defer zero(dek)
	kr, err := keyring.Load()
	if err != nil {
		return nil
	}
	priv, err := kr.GetSignKey(dek, instance)
	if err != nil || len(priv) != ed25519.PrivateKeySize {
		return nil
	}
	return ed25519.PrivateKey(priv)
}

// attestCommit signe (best-effort, NON bloquant) un commit créé par SituLab, par un TRAILER immuable
// « SituLab-Approved: » (v2 : voyage AVEC le commit → visible dans l'historique en ligne, sans note à
// pousser). La signature vient du SERVICE APCOM par défaut (clé jamais sur le poste) ; repli sur une
// signature locale si elle est disponible. Best-effort : aucune erreur n'est propagée au geste de commit.
func (s *Service) attestCommit(path, sha string) {
	if sha == "" {
		return
	}
	// Idempotent : ne JAMAIS re-signer un commit déjà scellé (éviterait de doubler le trailer et de
	// changer le SHA d'un commit déjà publié).
	if obj, err := gitx.CommitObject(path, sha); err == nil {
		if _, _, has := splitAttestTrailer(obj.Message); has {
			return
		}
	}
	val := s.buildAttestationTrailer(s.instanceForPath(path), path, sha)
	if val == "" {
		return
	}
	_, _ = gitx.RecommitWithTrailer(path, sha, attestTrailerKey, val)
}

// attestFreshCommit signe le commit de HEAD seulement s'il vient d'être CRÉÉ (HEAD a changé depuis
// `before`) — pour les opérations qui produisent un nouveau commit non-fusion (cherry-pick, revert).
func (s *Service) attestFreshCommit(path, before string) {
	sha := gitx.HeadSHA(path)
	if sha == "" || sha == before {
		return
	}
	s.attestCommit(path, sha)
}

// attestMergeCommit signe le commit de HEAD seulement si une VRAIE fusion a été créée (HEAD changé ET
// commit à ≥2 parents) — jamais une avance-rapide (qui ne fait que pointer un commit existant).
func (s *Service) attestMergeCommit(path, before string) {
	sha := gitx.HeadSHA(path)
	if sha == "" || sha == before {
		return
	}
	if obj, err := gitx.CommitObject(path, sha); err == nil && len(obj.Parents) >= 2 {
		s.attestCommit(path, sha)
	}
}

// attestPromotionSHA signe le commit de PROMOTION (créé par plomberie, encore DÉTACHÉ de toute branche)
// par le même trailer immuable, et renvoie le NOUVEAU sha à publier vers la vitrine — la garantie
// d'origine voyage ainsi avec le commit poussé. Best-effort : signature indisponible → sha d'origine
// inchangé (la promotion n'est JAMAIS bloquée par la signature).
func (s *Service) attestPromotionSHA(path, sha string) string {
	if sha == "" {
		return sha
	}
	val := s.buildAttestationTrailer(s.instanceForPath(path), path, sha)
	if val == "" {
		return sha
	}
	newSha, err := gitx.CommitWithTrailerObject(path, sha, attestTrailerKey, val)
	if err != nil || newSha == "" {
		return sha
	}
	return newSha
}

// buildAttestationTrailer produit la valeur du trailer d'attestation v2 (base64 du JSON d'attestation)
// pour le commit `sha`, par le SERVICE DE SIGNATURE APCOM — UNE SEULE VOIE, zéro clé sur le poste.
// La clé privée ne quitte jamais le serveur et n'est jamais remise à l'agent. Conditions : connexion
// Forgejo (le service autorise via l'API Forgejo — donc pas GitHub), session déverrouillée (le jeton
// autorise l'accès au dépôt visé) et origine distante connue. Renvoie "" si indisponible (best-effort :
// commit sans sceau, jamais d'erreur propagée au geste de commit).
func (s *Service) buildAttestationTrailer(inst, path, sha string) string {
	if inst == "" || s.isGitHub(inst) {
		return ""
	}
	token := s.instanceToken(inst)
	if token == "" {
		return ""
	}
	obj, err := gitx.CommitObject(path, sha)
	if err != nil {
		return ""
	}
	remote := ""
	if st, e := gitx.StatusOf(path); e == nil {
		remote = st.Remote
	}
	if remote == "" {
		return ""
	}
	body := strings.TrimRight(obj.Message, "\n")
	pre := attest.CommitPreimage(obj.Tree, obj.Parents, obj.Author, obj.Committer, body)
	attJSON, e := requestServerPreimageAttestation(s.signServerURLFor(inst), token, base64.StdEncoding.EncodeToString(pre), remote)
	if e != nil || len(attJSON) == 0 {
		return ""
	}
	// Vérification LOCALE avant de sceller : le serveur peut se tromper, être compromis ou renvoyer une
	// attestation déjà morte (cert hors fenêtre, serial révoqué, chaîne ne remontant pas à la racine
	// épinglée, signature ne couvrant pas CE pré-image). On ne pose JAMAIS un sceau qu'on ne sait pas
	// vérifier soi-même : sinon attestPromotionSHA réécrirait le SHA d'un commit autour d'un trailer
	// invérifiable, et l'historique afficherait une « garantie d'origine » que le poste ne peut pas
	// confirmer. Fail-closed : attestation non vérifiée → pas de sceau (best-effort, jamais d'erreur).
	att, derr := attest.Decode(attJSON)
	if derr != nil {
		return ""
	}
	if v := attest.VerifyPreimage(att, pre, time.Now(), s.revokedSerials()); !v.Verified {
		return ""
	}
	return base64.StdEncoding.EncodeToString(attJSON)
}

// signServerURLFor : URL du service de signature pour une connexion (sa valeur, sinon le défaut APCOM).
func (s *Service) signServerURLFor(inst string) string {
	cfg, err := config.Load()
	if err != nil {
		return defaultSignServerURL
	}
	if in := cfg.Resolve(inst); in != nil && strings.TrimSpace(in.SignServerURL) != "" {
		return in.SignServerURL
	}
	return defaultSignServerURL
}

// splitAttestTrailer isole le trailer d'attestation « SituLab-Approved: » du message d'un commit :
// renvoie le corps SANS le trailer (fins de ligne retaillées, pour correspondre au pré-image signé),
// la valeur base64 du trailer, et ok=false s'il n'y en a pas. On prend la DERNIÈRE occurrence (bloc
// de trailer final).
func splitAttestTrailer(message string) (body, val string, ok bool) {
	prefix := attestTrailerKey + ": "
	lines := strings.Split(message, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.HasPrefix(lines[i], prefix) {
			val = strings.TrimSpace(strings.TrimPrefix(lines[i], prefix))
			rest := append(append([]string{}, lines[:i]...), lines[i+1:]...)
			return strings.TrimRight(strings.Join(rest, "\n"), "\n"), val, true
		}
	}
	return strings.TrimRight(message, "\n"), "", false
}

// RefreshRevokedServers récupère les listes de révocation des services de signature (le service APCOM par
// défaut + toute URL de service propre à une connexion) et les fusionne dans RevokedSerials (best-effort,
// non bloquant). À appeler périodiquement.
func (s *Service) RefreshRevokedServers() {
	cfg, err := config.Load()
	if err != nil {
		return
	}
	seen := map[string]bool{defaultSignServerURL: true}
	urls := []string{defaultSignServerURL}
	for _, in := range cfg.Instances {
		if u := strings.TrimSpace(in.SignServerURL); u != "" && !seen[u] {
			seen[u] = true
			urls = append(urls, u)
		}
	}
	add := map[string]bool{}
	for _, u := range urls {
		if ser, e := fetchRevokedSerials(u); e == nil {
			for _, x := range ser {
				add[x] = true
			}
		}
	}
	if len(add) == 0 {
		return
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg2, err := config.Load()
	if err != nil {
		return
	}
	have := map[string]bool{}
	for _, x := range cfg2.RevokedSerials {
		have[x] = true
	}
	changed := false
	for x := range add {
		if !have[x] {
			cfg2.RevokedSerials = append(cfg2.RevokedSerials, x)
			changed = true
		}
	}
	if changed {
		if cfg2.Save() == nil {
			s.publish(cfg2)
		}
	}
}

// pushAttestNotes publie (best-effort, non bloquant) la ref de notes d'attestation après un envoi de
// code, pour rendre la signature « SituLab approved » visible en ligne. Un échec est silencieux : la
// discipline reste non bloquante, et un dépôt sans attestation (pas de cert) n'a simplement rien à pousser.
func (s *Service) pushAttestNotes(instance, path string) {
	_, _ = gitx.PushNotes(path, attestNotesRef, s.instanceToken(instance), s.isGitHub(instance), s.instanceBaseURL(instance))
}

// fetchAttestNotes récupère (best-effort, non bloquant) la ref de notes d'attestation depuis le distant,
// pour que l'on VOIE les signatures posées par les autres postes sur les commits récupérés.
func (s *Service) fetchAttestNotes(instance, path string) {
	_, _ = gitx.FetchNotes(path, attestNotesRef, s.instanceToken(instance), s.isGitHub(instance), s.instanceBaseURL(instance))
}

// CommitAttestation renvoie l'état d'attestation d'un commit (lecture locale, pas de PIN, fail-closed).
// v2 (défaut) : trailer immuable du commit. Repli v1 : note git (commits d'avant 1.9.7 / mode serveur).
func (s *Service) CommitAttestation(path, sha string) attest.Verdict {
	if obj, err := gitx.CommitObject(path, sha); err == nil {
		if body, val, ok := splitAttestTrailer(obj.Message); ok {
			dec, err := base64.StdEncoding.DecodeString(val)
			if err != nil {
				return attest.Verdict{Reason: "attestation illisible"}
			}
			att, err := attest.Decode(dec)
			if err != nil {
				return attest.Verdict{Reason: "attestation illisible"}
			}
			pre := attest.CommitPreimage(obj.Tree, obj.Parents, obj.Author, obj.Committer, body)
			return attest.VerifyPreimage(att, pre, time.Now(), s.revokedSerials())
		}
	}
	raw := gitx.ReadNote(path, attestNotesRef, sha)
	if raw == "" {
		return attest.Verdict{} // ni trailer ni note = non attesté
	}
	att, err := attest.Decode([]byte(raw))
	if err != nil {
		return attest.Verdict{Reason: "attestation illisible"}
	}
	return attest.Verify(att, sha, time.Now(), s.revokedSerials())
}

// CommitAttestations renvoie les verdicts pour une liste de commits (onglet Historique).
func (s *Service) CommitAttestations(path string, shas []string) map[string]attest.Verdict {
	out := make(map[string]attest.Verdict, len(shas))
	for _, sha := range shas {
		out[sha] = s.CommitAttestation(path, sha)
	}
	return out
}

// AttestedCommit : un commit local et son verdict d'attestation (pour l'onglet Historique / CLI).
type AttestedCommit struct {
	gitx.LocalCommit
	Attestation attest.Verdict `json:"attestation"`
}

// AttestHistory renvoie les derniers commits locaux d'un dépôt avec leur état d'attestation (lecture).
func (s *Service) AttestHistory(path string, limit int) ([]AttestedCommit, error) {
	if !gitx.Available() {
		return nil, ErrNoGit
	}
	commits := gitx.Log(path, limit)
	out := make([]AttestedCommit, 0, len(commits))
	for _, c := range commits {
		out = append(out, AttestedCommit{LocalCommit: c, Attestation: s.CommitAttestation(path, c.SHA)})
	}
	return out, nil
}

func (s *Service) revokedSerials() map[string]bool {
	cfg, err := config.Load()
	if err != nil {
		return nil
	}
	m := make(map[string]bool, len(cfg.RevokedSerials))
	for _, ser := range cfg.RevokedSerials {
		m[ser] = true
	}
	return m
}

// GenerateSignKey génère (ou régénère) la clé d'appareil d'une instance et renvoie sa clé PUBLIQUE
// (base64), à faire certifier par la gouvernance. Écriture : session requise + lecture seule respectée.
// Le privé est rangé chiffré dans le trousseau et ne sort jamais du démon.
func (s *Service) GenerateSignKey(instance string) (string, error) {
	if err := s.assertWritable(); err != nil {
		return "", err
	}
	dek, err := s.sessionDEK()
	if err != nil {
		return "", ErrLocked
	}
	defer zero(dek)
	pub, priv, err := attest.GenerateDeviceKey()
	if err != nil {
		return "", err
	}
	defer zero(priv)
	// Sérialiser la mutation du trousseau (lire-modifier-écrire) comme toute autre, pour ne pas
	// entrelacer avec un SetToken/SetPassword concurrent (dernière écriture gagnante = perte de données).
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	kr, err := keyring.Load()
	if err != nil {
		return "", err
	}
	if err := kr.SetSignKey(dek, instance, priv); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(pub), nil
}

// InstallSignCert installe le certificat d'appareil émis par la gouvernance, après avoir vérifié qu'il
// chaîne à la racine APCOM, est valide, et correspond à la clé d'appareil rangée. Écriture.
func (s *Service) InstallSignCert(instance, certJSON string) error {
	if err := s.assertWritable(); err != nil {
		return err
	}
	var c attest.Cert
	if err := json.Unmarshal([]byte(certJSON), &c); err != nil {
		return fmt.Errorf("certificat illisible : %w", err)
	}
	if err := attest.CheckCert(c, time.Now(), s.revokedSerials()); err != nil {
		return fmt.Errorf("certificat refusé : %w", err)
	}
	// Le cert doit correspondre à la clé d'appareil de CETTE instance.
	priv := s.signKeyFor(instance)
	if priv == nil {
		return fmt.Errorf("aucune clé d'appareil pour « %s », générez-la d'abord", instance)
	}
	defer zero(priv)
	if !priv.Public().(ed25519.PublicKey).Equal(c.DevicePub) {
		return fmt.Errorf("le certificat ne correspond pas à la clé d'appareil de « %s »", instance)
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	in := cfg.Resolve(instance)
	if in == nil {
		return fmt.Errorf("instance « %s » introuvable", instance)
	}
	in.SignCert = certJSON
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}
