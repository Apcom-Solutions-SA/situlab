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
	"fmt"
	"strings"
	"time"
)

// apcom_root.pub : clé publique racine APCOM (base64 ed25519), ancre de confiance de la vérification.
// La vraie racine de gouvernance y est embarquée (cérémonie du 2026-10-10) ; le privé racine ne touche
// jamais un poste. UsingTestRoot() indique si, dans un build donné, c'est encore la racine de TEST
// (cf. testRootB64) — ce n'est PAS le cas d'un build de release.
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

// situlab_sign.* : identité de SIGNATURE LOGICIELLE embarquée (v1.9.5) — UNE SEULE identité « SituLab »,
// la même pour toutes les installations (ni par utilisateur, ni par organisation). Mintée une fois par la
// gouvernance (clé signée par la CA, cf. `attest gen-software`) puis embarquée dans le binaire.
//
//go:embed situlab_sign.key
var softwareKeyB64 string

//go:embed situlab_sign.cert.json
var softwareCertJSON string

// SoftwareSigner renvoie la clé privée + le certificat de signature LOGICIELLE embarqués. ok=false tant
// qu'ils ne sont pas mintés (placeholder).
//
// ⚠️ Limite ASSUMÉE (garantie logiciel LOCALE) : une clé embarquée dans le binaire est extractible par un
// process du même poste → c'est un marqueur d'authenticité « fait via SituLab », PAS une preuve
// infalsifiable face à un attaquant local. La version inviolable est la signature CÔTÉ SERVEUR (T3) :
// la clé ne quitte jamais le serveur et le poste ne peut rien forger (voir SECURITY.md / directives serveur).
func SoftwareSigner() (ed25519.PrivateKey, Cert, bool) {
	kb, err := base64.StdEncoding.DecodeString(strings.TrimSpace(softwareKeyB64))
	if err != nil || len(kb) != ed25519.PrivateKeySize {
		return nil, Cert{}, false
	}
	var c Cert
	if json.Unmarshal([]byte(softwareCertJSON), &c) != nil || len(c.DevicePub) != ed25519.PublicKeySize {
		return nil, Cert{}, false
	}
	return ed25519.PrivateKey(kb), c, true
}

// HasSoftwareSigner indique si la signature logicielle « SituLab » est active (clé+cert embarqués).
func HasSoftwareSigner() bool { _, _, ok := SoftwareSigner(); return ok }

// Root renvoie la clé publique racine APCOM épinglée.
func Root() (ed25519.PublicKey, error) {
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(rootPubB64))
	if err != nil || len(b) != ed25519.PublicKeySize {
		return nil, errors.New("racine APCOM embarquée invalide")
	}
	return ed25519.PublicKey(b), nil
}

// maxChainDepth borne la profondeur de la chaîne de certificats : au plus UNE autorité intermédiaire
// entre la racine et le certificat d'appareil (racine → CA intermédiaire → appareil). Empêche qu'une
// chaîne arbitrairement profonde serve à masquer une rupture de confiance ou à épuiser la pile.
const maxChainDepth = 1

// Cert lie une clé (d'appareil, ou d'autorité intermédiaire) à une identité, pour une fenêtre de
// validité, signé par son émetteur. Le certificat n'est pas secret (stocké en config).
//
// Chaîne à deux niveaux (v1.9.2) : un certificat d'appareil peut être signé SOIT directement par la
// racine (Issuer nil — 1 niveau, rétro-compatible), SOIT par une autorité intermédiaire (Issuer = le
// cert de cette CA, elle-même signée par la racine). `IsCA` distingue une autorité d'un appareil et
// EST couvert par la signature : un certificat d'appareil ne peut pas être détourné en autorité.
type Cert struct {
	DevicePub ed25519.PublicKey `json:"device_pub"`
	Subject   string            `json:"subject"`
	NotBefore time.Time         `json:"not_before"`
	NotAfter  time.Time         `json:"not_after"`
	Serial    string            `json:"serial"`
	IsCA      bool              `json:"is_ca,omitempty"`  // true = autorité intermédiaire (peut signer d'autres certs)
	Issuer    *Cert             `json:"issuer,omitempty"` // nil = signé par la racine ; sinon la CA émettrice (chaînée à la racine)
	RootSig   []byte            `json:"root_sig"`         // signature de l'émetteur sur certSigningBytes (hors Issuer/RootSig)
}

// certSigningBytes est la représentation canonique STABLE signée par l'émetteur (timestamps en Unix pour
// éviter toute ambiguïté de sérialisation ; IsCA couvert pour empêcher la promotion appareil→autorité).
// Préfixe de domaine anti-confusion.
func certSigningBytes(devicePub ed25519.PublicKey, subject string, nb, na time.Time, serial string, isCA bool) []byte {
	payload, _ := json.Marshal(struct {
		DevicePub []byte `json:"device_pub"`
		Subject   string `json:"subject"`
		NotBefore int64  `json:"not_before"`
		NotAfter  int64  `json:"not_after"`
		Serial    string `json:"serial"`
		IsCA      bool   `json:"is_ca"`
	}{devicePub, subject, nb.Unix(), na.Unix(), serial, isCA})
	return append([]byte("situlab-cert-v2\x00"), payload...)
}

// signCert fabrique un certificat signé par signerPriv (racine ou CA). Facteur commun des trois émissions.
func signCert(signerPriv ed25519.PrivateKey, issuer *Cert, pub ed25519.PublicKey, subject string, notBefore, notAfter time.Time, serial string, isCA bool) Cert {
	c := Cert{DevicePub: pub, Subject: subject, NotBefore: notBefore, NotAfter: notAfter, Serial: serial, IsCA: isCA, Issuer: issuer}
	c.RootSig = ed25519.Sign(signerPriv, certSigningBytes(pub, subject, notBefore, notAfter, serial, isCA))
	return c
}

// IssueCert signe un certificat d'APPAREIL directement avec la clé privée RACINE (chaîne 1 niveau).
// Geste de GOUVERNANCE (hors ligne) ; le privé racine n'est jamais présent dans le démon.
func IssueCert(rootPriv ed25519.PrivateKey, devicePub ed25519.PublicKey, subject string, notBefore, notAfter time.Time, serial string) Cert {
	return signCert(rootPriv, nil, devicePub, subject, notBefore, notAfter, serial, false)
}

// IssueCA signe un certificat d'AUTORITÉ INTERMÉDIAIRE avec la clé privée RACINE (geste hors ligne).
// La CA ainsi certifiée peut émettre des certificats d'appareil via IssueCertFrom (chaîne 2 niveaux).
func IssueCA(rootPriv ed25519.PrivateKey, caPub ed25519.PublicKey, subject string, notBefore, notAfter time.Time, serial string) Cert {
	return signCert(rootPriv, nil, caPub, subject, notBefore, notAfter, serial, true)
}

// IssueCertFrom signe un certificat d'APPAREIL avec une CA intermédiaire, en embarquant le cert de la CA
// (issuer) pour que la chaîne soit vérifiable hors ligne jusqu'à la racine épinglée.
func IssueCertFrom(caPriv ed25519.PrivateKey, issuer Cert, devicePub ed25519.PublicKey, subject string, notBefore, notAfter time.Time, serial string) Cert {
	return signCert(caPriv, &issuer, devicePub, subject, notBefore, notAfter, serial, false)
}

// verifyCert : le cert chaîne à la racine épinglée (directement ou via une CA intermédiaire), est dans
// sa fenêtre de validité et non révoqué. Fail-closed : toute rupture → erreur.
func verifyCert(c Cert, now time.Time, revoked map[string]bool) error {
	return verifyCertDepth(c, now, revoked, 0)
}

func verifyCertDepth(c Cert, now time.Time, revoked map[string]bool, depth int) error {
	if depth > maxChainDepth {
		return errors.New("chaîne de certificats trop longue")
	}
	if len(c.DevicePub) != ed25519.PublicKeySize {
		return errors.New("clé invalide")
	}
	// Déterminer la clé émettrice : la racine épinglée, ou la CA intermédiaire (elle-même vérifiée).
	var signer ed25519.PublicKey
	if c.Issuer == nil {
		root, err := Root()
		if err != nil {
			return err
		}
		signer = root
	} else {
		if !c.Issuer.IsCA {
			return errors.New("l'émetteur n'est pas une autorité (CA)")
		}
		if err := verifyCertDepth(*c.Issuer, now, revoked, depth+1); err != nil {
			return fmt.Errorf("émetteur invalide : %w", err)
		}
		signer = c.Issuer.DevicePub
	}
	if !ed25519.Verify(signer, certSigningBytes(c.DevicePub, c.Subject, c.NotBefore, c.NotAfter, c.Serial, c.IsCA), c.RootSig) {
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

// Attestation : preuve qu'un commit est né dans SituLab. Deux portages coexistent :
//   - v2 (défaut depuis 1.9.7) : portée par un TRAILER de commit « SituLab-Approved: » — IMMUABLE (dans
//     l'objet commit) et voyage AVEC le commit (plus de note à pousser/récupérer). Sig couvre le
//     pré-image du commit (CommitPreimage), pas le SHA : le trailer modifie le SHA, on ne peut donc pas
//     signer le SHA final (auto-référence).
//   - v1 (legacy + mode serveur) : portée par la note git refs/notes/situlab. Sig couvre le SHA.
//
// Verify (v1) et VerifyPreimage (v2) distinguent les deux ; V=2 marque une attestation de trailer. La
// chaîne de certificat et la racine épinglée sont INCHANGÉES entre les deux : seule la charge signée diffère.
type Attestation struct {
	Commit string `json:"commit,omitempty"` // v1 : SHA lié (vide en v2)
	Cert   Cert   `json:"cert"`
	Sig    []byte `json:"sig"`         // v1 : ed25519(priv, commitSigningBytes(SHA)) ; v2 : ed25519(priv, CommitPreimage(...))
	TS     string `json:"ts"`          // horodatage indicatif (affichage) — NON couvert par la signature, non fiable
	V      int    `json:"v,omitempty"` // portage : 2 = trailer/pré-image ; absent/0 = v1 note/SHA
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

// --- v2 (1.9.7) : attestation IMMUABLE portée par un trailer de commit ---------------------------
//
// Le trailer « SituLab-Approved: » fait partie de l'objet commit (il en modifie le SHA) : on ne peut
// donc pas signer le SHA final (auto-référence). On signe à la place un PRÉ-IMAGE canonique du commit —
// son arbre, ses parents, ses lignes auteur/committer et son message SANS le trailer. Toute altération
// du commit (arbre, parent, auteur, message) change le pré-image → la vérification échoue ; transplanter
// le trailer sur un autre commit échoue de même. La chaîne de certificat et la racine épinglée sont
// INCHANGÉES : seule la charge signée diffère de la v1.

// CommitPreimage construit la représentation canonique STABLE signée pour un commit v2. `author` et
// `committer` sont les lignes d'identité git complètes (« Nom <email> ts tz ») ; `body` est le message
// du commit SANS le trailer d'attestation, espaces de fin retirés. Préfixe de domaine anti-confusion.
func CommitPreimage(tree string, parents []string, author, committer, body string) []byte {
	payload, _ := json.Marshal(struct {
		Tree      string   `json:"tree"`
		Parents   []string `json:"parents"`
		Author    string   `json:"author"`
		Committer string   `json:"committer"`
		Body      string   `json:"body"`
	}{tree, parents, author, committer, body})
	return append([]byte("situlab-attest-v2\x00"), payload...)
}

// SignPreimage produit une attestation v2 (trailer) : la clé d'appareil signe le pré-image du commit.
func SignPreimage(devicePriv ed25519.PrivateKey, cert Cert, preimage []byte) Attestation {
	return Attestation{
		V:    2,
		Cert: cert,
		Sig:  ed25519.Sign(devicePriv, preimage),
		TS:   time.Now().UTC().Format(time.RFC3339),
	}
}

// VerifyPreimage vérifie une attestation v2 pour un pré-image de commit : cert → racine + fenêtre de
// validité + non révoqué + signature d'appareil sur le pré-image. Fail-closed : toute erreur → non vérifié.
func VerifyPreimage(att Attestation, preimage []byte, now time.Time, revoked map[string]bool) Verdict {
	if err := verifyCert(att.Cert, now, revoked); err != nil {
		return Verdict{Reason: err.Error()}
	}
	if !ed25519.Verify(att.Cert.DevicePub, preimage, att.Sig) {
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
