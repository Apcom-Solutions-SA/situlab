// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

// Package attest porte la « signature de garantie SituLab approved » (T2) : une attestation d'origine
// qu'un commit est NÉ dans SituLab, liée à l'identité vérifiée du collaborateur (le certificat), et
// vérifiable localement contre une racine APCOM épinglée. Non bloquante : on rend visible qui respecte
// la discipline, on ne rejette rien.
//
// Chaîne de confiance : Racine APCOM (ed25519, privé hors ligne) → certifie une clé d'appareil
// (ed25519, dans le trousseau chiffré par le PIN) → l'appareil signe chaque commit créé par SituLab.
// La vérification (cert → racine + fenêtre de validité + non révoqué + signature liée au commit) est
// 100 % locale, en Go, contre la racine publique embarquée.
//
// La clé privée d'appareil ne quitte jamais le démon. Plafond assumé (T2) : un attaquant sous le même
// compte OS peut lire la clé en mémoire du démon déverrouillé et forger un faux « approved » — même
// plafond que partout dans SituLab, borné par des certs courts et la révocation. On ne prétend JAMAIS
// que c'est infalsifiable en local (seul un T3 côté serveur le serait).
package attest

import (
	"crypto/ed25519"
	"crypto/rand"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// apcom_root.pub : clé publique racine APCOM (base64 ed25519), ancre de confiance de la vérification.
// Valeur ACTUELLE = racine de TEST ; la gouvernance remplacera ce fichier par la vraie racine publique
// (le privé racine ne doit jamais toucher un poste).
//
//go:embed apcom_root.pub
var rootPubB64 string

// testRootB64 : clé publique de la racine de TEST (graine fixe). Tant que la racine embarquée vaut
// celle-ci, la signature est « de test » : forgeable par qui lit les tests. À remplacer par la vraie
// racine de gouvernance avant toute mise en prod de la fonction.
const testRootB64 = "T9CZzNR9eJPf6ewkQU7LDZtUICMqrTDZHEZb4zy+ZcQ="

// UsingTestRoot indique si la racine embarquée est encore la racine de TEST.
func UsingTestRoot() bool {
	return strings.TrimSpace(rootPubB64) == testRootB64
}

// Root renvoie la clé publique racine APCOM épinglée.
func Root() (ed25519.PublicKey, error) {
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(rootPubB64))
	if err != nil || len(b) != ed25519.PublicKeySize {
		return nil, errors.New("racine APCOM embarquée invalide")
	}
	return ed25519.PublicKey(b), nil
}

// Cert lie une clé d'appareil à une identité vérifiée, pour une fenêtre de validité, signé par la racine.
// Le certificat n'est pas secret (stocké en config).
type Cert struct {
	DevicePub ed25519.PublicKey `json:"device_pub"`
	Subject   string            `json:"subject"`
	NotBefore time.Time         `json:"not_before"`
	NotAfter  time.Time         `json:"not_after"`
	Serial    string            `json:"serial"`
	RootSig   []byte            `json:"root_sig"` // signature racine sur certSigningBytes (hors RootSig)
}

// certSigningBytes est la représentation canonique STABLE signée par la racine (timestamps en Unix pour
// éviter toute ambiguïté de sérialisation). Préfixe de domaine anti-confusion.
func certSigningBytes(devicePub ed25519.PublicKey, subject string, nb, na time.Time, serial string) []byte {
	payload, _ := json.Marshal(struct {
		DevicePub []byte `json:"device_pub"`
		Subject   string `json:"subject"`
		NotBefore int64  `json:"not_before"`
		NotAfter  int64  `json:"not_after"`
		Serial    string `json:"serial"`
	}{devicePub, subject, nb.Unix(), na.Unix(), serial})
	return append([]byte("situlab-cert-v1\x00"), payload...)
}

// IssueCert signe un certificat d'appareil avec la clé privée RACINE. Geste de GOUVERNANCE (hors ligne)
// — exposé ici pour l'émission manuelle et les tests ; le privé racine n'est jamais présent dans le démon.
func IssueCert(rootPriv ed25519.PrivateKey, devicePub ed25519.PublicKey, subject string, notBefore, notAfter time.Time, serial string) Cert {
	c := Cert{DevicePub: devicePub, Subject: subject, NotBefore: notBefore, NotAfter: notAfter, Serial: serial}
	c.RootSig = ed25519.Sign(rootPriv, certSigningBytes(devicePub, subject, notBefore, notAfter, serial))
	return c
}

// verifyCert : le cert chaîne à la racine épinglée, est dans sa fenêtre de validité et non révoqué.
func verifyCert(c Cert, now time.Time, revoked map[string]bool) error {
	root, err := Root()
	if err != nil {
		return err
	}
	if len(c.DevicePub) != ed25519.PublicKeySize {
		return errors.New("clé d'appareil invalide")
	}
	if !ed25519.Verify(root, certSigningBytes(c.DevicePub, c.Subject, c.NotBefore, c.NotAfter, c.Serial), c.RootSig) {
		return errors.New("certificat non émis par la racine APCOM")
	}
	if now.Before(c.NotBefore) || now.After(c.NotAfter) {
		return errors.New("certificat hors fenêtre de validité")
	}
	if revoked[c.Serial] {
		return errors.New("certificat révoqué")
	}
	return nil
}

// CheckCert valide un certificat seul (chaîne à la racine + fenêtre + non révoqué), sans commit.
// Sert à l'installation d'un cert émis par la gouvernance.
func CheckCert(c Cert, now time.Time, revoked map[string]bool) error {
	return verifyCert(c, now, revoked)
}

// Attestation : preuve, portée par la note git d'un commit, qu'il est né dans SituLab.
type Attestation struct {
	Commit string `json:"commit"`
	Cert   Cert   `json:"cert"`
	Sig    []byte `json:"sig"` // ed25519(device_priv, commitSigningBytes(commit)) — lie la signature AU commit
	TS     string `json:"ts"`  // horodatage indicatif (affichage) — NON couvert par la signature, non fiable
}

// commitSigningBytes lie la signature au SHA du commit (anti-rejeu : une signature d'un commit X ne
// vérifie jamais sur un commit Y).
func commitSigningBytes(commitSHA string) []byte {
	return []byte("situlab-attest-v1\x00" + commitSHA)
}

// Sign produit l'attestation d'un commit avec la clé d'appareil et son certificat.
func Sign(devicePriv ed25519.PrivateKey, cert Cert, commitSHA string) Attestation {
	return Attestation{
		Commit: commitSHA,
		Cert:   cert,
		Sig:    ed25519.Sign(devicePriv, commitSigningBytes(commitSHA)),
		TS:     time.Now().UTC().Format(time.RFC3339),
	}
}

// Verdict : résultat de vérification pour l'affichage (icône « approved par Untel » ou rien).
type Verdict struct {
	Verified bool   `json:"verified"`
	Subject  string `json:"subject,omitempty"`
	Reason   string `json:"reason,omitempty"` // pourquoi non vérifié (diagnostic, pas affiché à l'utilisateur)
}

// Verify vérifie une attestation pour un commit donné : liaison au commit + cert → racine + validité +
// non révoqué + signature d'appareil. Fail-closed : toute erreur → non vérifié.
func Verify(att Attestation, commitSHA string, now time.Time, revoked map[string]bool) Verdict {
	if att.Commit != commitSHA {
		return Verdict{Reason: "attestation liée à un autre commit"}
	}
	if err := verifyCert(att.Cert, now, revoked); err != nil {
		return Verdict{Reason: err.Error()}
	}
	if !ed25519.Verify(att.Cert.DevicePub, commitSigningBytes(commitSHA), att.Sig) {
		return Verdict{Reason: "signature d'appareil invalide"}
	}
	return Verdict{Verified: true, Subject: att.Cert.Subject}
}

// GenerateDeviceKey crée une paire d'appareil ed25519.
func GenerateDeviceKey() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	return ed25519.GenerateKey(rand.Reader)
}

// Encode/Decode : (dé)sérialisation de l'attestation pour la note git.
func (a Attestation) Encode() ([]byte, error) { return json.Marshal(a) }

func Decode(b []byte) (Attestation, error) {
	var a Attestation
	err := json.Unmarshal(b, &a)
	return a, err
}
