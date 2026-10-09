// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"situlab/internal/attest"
	"situlab/internal/config"
	"situlab/internal/gitx"
	"situlab/internal/keyring"
)

// attestNotesRef : ref de notes git qui porte les attestations « SituLab approved ».
const attestNotesRef = "situlab"

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
	b, err := attest.Sign(priv, *cert, sha).Encode()
	if err != nil {
		return
	}
	_ = gitx.WriteNote(path, attestNotesRef, sha, string(b))
}

// CommitAttestation renvoie l'état d'attestation d'un commit (lecture locale, pas de PIN, fail-closed).
func (s *Service) CommitAttestation(path, sha string) attest.Verdict {
	raw := gitx.ReadNote(path, attestNotesRef, sha)
	if raw == "" {
		return attest.Verdict{} // pas de note = non attesté
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
