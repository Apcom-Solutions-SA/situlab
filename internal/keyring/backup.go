// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

// Export / import chiffré de la configuration SituLab (P18, v1.18.2). Réutilise les primitives du
// trousseau (Argon2id → KEK → DEK → AES-256-GCM). Un fichier .slbak contient : des métadonnées en
// CLAIR non sensibles (catégories, date) ; la config (sans secret : Règle 17, aucun token n'y vit) ;
// et le TROUSSEAU ré-enveloppé sous le PIN de sauvegarde — les jetons restent chiffrés, seul le Wrap
// de la DEK change. Aucun credential ni PIN n'est jamais écrit en clair.

package keyring

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"time"
)

const (
	slbakMagic   = "SITULAB-BACKUP"
	slbakVersion = 1
)

// Bundle est le contenu d'un fichier .slbak.
type Bundle struct {
	Magic      string          `json:"magic"`
	Version    int             `json:"version"`
	When       string          `json:"when"`
	Categories []string        `json:"categories,omitempty"`
	Config     json.RawMessage `json:"config"`  // config SituLab (non secret)
	Keyring    File            `json:"keyring"` // trousseau ré-enveloppé sous le PIN de sauvegarde
}

// BuildExport fabrique EN MÉMOIRE un trousseau équivalent au trousseau local, ré-enveloppé sous
// exportPIN : les jetons restent chiffrés sous la MÊME DEK ; seul le Wrap change. keepIDs != nil
// filtre les entrées (export sélectif). Nécessite la DEK de session (pour re-wrapper).
func BuildExport(dek []byte, exportPIN string, keepIDs map[string]bool) (*File, error) {
	f, err := Load()
	if err != nil {
		return nil, err
	}
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	k := kdfParams{Algo: algoKDF, Salt: salt, Mem: defMemKiB, Time: defTime, Par: defPar}
	kek := deriveKEK(exportPIN, k)
	defer zero(kek)
	wrap, err := seal(kek, dek, []byte(aadWrap))
	if err != nil {
		return nil, err
	}
	out := &File{Version: Version, KDF: k, Wrap: wrap}
	for _, e := range f.Entries {
		if keepIDs == nil || keepIDs[e.ID] {
			out.Entries = append(out.Entries, e)
		}
	}
	return out, nil
}

// MakeBundle sérialise un .slbak (config non secrète + trousseau ré-enveloppé).
func MakeBundle(cfg []byte, categories []string, kr *File) ([]byte, error) {
	b := Bundle{
		Magic:      slbakMagic,
		Version:    slbakVersion,
		When:       time.Now().UTC().Format(time.RFC3339),
		Categories: categories,
		Config:     json.RawMessage(cfg),
		Keyring:    *kr,
	}
	return json.MarshalIndent(b, "", "  ")
}

// ParseBundle lit et valide l'en-tête d'un .slbak (sans le déchiffrer).
func ParseBundle(data []byte) (*Bundle, error) {
	var b Bundle
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, ErrFormat
	}
	if b.Magic != slbakMagic {
		return nil, errors.New("fichier de sauvegarde SituLab invalide")
	}
	if b.Version != slbakVersion || !validKDF(b.Keyring.KDF) {
		return nil, ErrFormat
	}
	return &b, nil
}

// InstallKeyring écrit un trousseau reçu d'un import sur un poste VIERGE comme trousseau local (il est
// déjà verrouillé par le PIN de sauvegarde, qui devient le PIN local). Échoue si un trousseau existe.
func InstallKeyring(f *File) error {
	if Initialized() {
		return ErrAlreadyInit
	}
	return f.save()
}

// MergeInto re-scelle chaque entrée du trousseau SOURCE (ouvert avec srcDEK) dans le trousseau LOCAL
// sous dstDEK. Pour un import-fusion sur un poste DÉJÀ configuré : le PIN local ne change pas.
func MergeInto(local *File, dstDEK []byte, src *File, srcDEK []byte) error {
	for _, e := range src.Entries {
		tok, err := src.GetToken(srcDEK, e.ID)
		if err != nil {
			continue
		}
		if err := local.SetToken(dstDEK, e.ID, e.InstanceURL, e.Scopes, tok); err != nil {
			return err
		}
		if e.Pwd != nil {
			if pw, err := src.GetPassword(srcDEK, e.ID); err == nil && pw != "" {
				_ = local.SetPassword(dstDEK, e.ID, pw)
			}
		}
		if e.Sign != nil {
			if sk, err := src.GetSignKey(srcDEK, e.ID); err == nil && len(sk) > 0 {
				_ = local.SetSignKey(dstDEK, e.ID, sk)
				zero(sk)
			}
		}
	}
	return nil
}
