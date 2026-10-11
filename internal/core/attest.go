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

// attestCommit signe (best-effort, NON bloquant) un commit créé par SituLab : si l'instance du dépôt a
// une clé d'appareil + un cert, écrit l'attestation en note. La discipline est non bloquante → aucune
// erreur n'est propagée au geste de commit.
func (s *Service) attestCommit(path, sha string) {
	if sha == "" {
		return
	}
	inst := s.instanceForPath(path)
	// Mode SERVEUR (1.9.6) : si la connexion du dépôt est en « signature serveur », le démon demande
	// l'empreinte au service (clé jamais sur le poste). Prioritaire sur toute signature locale ; on ne
	// retombe pas sur une clé locale (ce serait contraire au sens du mode serveur). Best-effort non bloquant.
	if inst != "" && s.signModeServer(inst) {
		s.attestCommitViaServer(inst, path, sha)
		return
	}
	// Chemins LOCAUX souverains → attestation v2 IMMUABLE portée par un trailer de commit (voyage avec
	// le commit, plus de note à pousser). Préférence (1.9.5) : signature LOGICIELLE unique « SituLab »
	// si elle est embarquée — le logiciel signe, pas l'utilisateur. Repli sur le cert d'appareil de
	// l'instance sinon.
	if priv, cert, ok := attest.SoftwareSigner(); ok {
		defer zero(priv)
		s.attestLocalTrailer(path, sha, priv, cert)
		return
	}
	if inst == "" {
		return
	}
	cert := s.signCertFor(inst)
	if cert == nil {
		return
	}
	priv := s.signKeyFor(inst)
	if priv == nil {
		return
	}
	defer zero(priv)
	s.attestLocalTrailer(path, sha, priv, *cert)
}

// attestLocalTrailer signe le pré-image du commit avec la clé d'appareil puis recrée le commit en y
// ajoutant le trailer « SituLab-Approved: ». Best-effort non bloquant : à la moindre anicroche, le
// commit reste tel quel (sans attestation), jamais d'erreur propagée au geste de commit.
func (s *Service) attestLocalTrailer(path, sha string, priv ed25519.PrivateKey, cert attest.Cert) {
	obj, err := gitx.CommitObject(path, sha)
	if err != nil {
		return
	}
	body := strings.TrimRight(obj.Message, "\n")
	pre := attest.CommitPreimage(obj.Tree, obj.Parents, obj.Author, obj.Committer, body)
	raw, err := attest.SignPreimage(priv, cert, pre).Encode()
	if err != nil {
		return
	}
	_, _ = gitx.RecommitWithTrailer(path, sha, attestTrailerKey, base64.StdEncoding.EncodeToString(raw))
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

// signModeServer indique si la connexion est configurée en mode « signature serveur » (1.9.6).
func (s *Service) signModeServer(inst string) bool {
	cfg, err := config.Load()
	if err != nil {
		return false
	}
	in := cfg.Resolve(inst)
	return in != nil && in.SignMode == "server"
}

// attestCommitViaServer demande l'empreinte du commit au service de signature serveur et l'écrit en note.
// Le jeton Forgejo (détenu par le démon) autorise la demande ; il n'est JAMAIS remis au client/agent.
// Best-effort non bloquant : un échec (réseau, 403, 429, 503) laisse simplement le commit sans note.
func (s *Service) attestCommitViaServer(inst, path, sha string) {
	cfg, err := config.Load()
	if err != nil {
		return
	}
	in := cfg.Resolve(inst)
	if in == nil {
		return
	}
	token := s.instanceToken(inst)
	if token == "" {
		return
	}
	remote := ""
	if st, e := gitx.StatusOf(path); e == nil {
		remote = st.Remote
	}
	att, e := requestServerAttestation(in.SignServerURL, token, sha, remote)
	if e != nil {
		return
	}
	_ = gitx.WriteNote(path, attestNotesRef, sha, string(att))
}

// RefreshRevokedServers récupère les listes de révocation des services de signature des connexions en
// mode serveur et les fusionne dans RevokedSerials (best-effort, non bloquant). À appeler périodiquement.
func (s *Service) RefreshRevokedServers() {
	cfg, err := config.Load()
	if err != nil {
		return
	}
	seen := map[string]bool{}
	var urls []string
	for _, in := range cfg.Instances {
		if in.SignMode != "server" {
			continue
		}
		u := in.SignServerURL
		if u == "" {
			u = defaultSignServerURL
		}
		if !seen[u] {
			seen[u] = true
			urls = append(urls, u)
		}
	}
	if len(urls) == 0 {
		return
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
		return fmt.Errorf("aucune clé d'appareil pour « %s » — générez-la d'abord", instance)
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
