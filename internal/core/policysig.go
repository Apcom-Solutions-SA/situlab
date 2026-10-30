// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"

	"situlab/internal/config"
)

// ErrPolicyTampered : la politique sur disque porte une signature PRÉSENTE mais INVALIDE (réécrite hors
// SituLab). On refuse alors toute mutation de politique : re-signer par-dessus « bénirait » la
// falsification (laundering). La récupération est un geste EXPLICITE (ReanchorPolicy), jamais implicite.
var ErrPolicyTampered = errors.New("politique de sécurité non validée sur ce poste (réécrite hors SituLab ou clé d'appareil renouvelée) : re-validez-la dans Réglages")

// Intégrité de la POLITIQUE de sécurité (v1.16, fermeture P3).
//
// config.json porte la politique (rôle + pin_policy par instance, allowed/denied_dirs + pin_policy par
// cloud, read_only) EN CLAIR et INSCRIPTIBLE par le propriétaire du compte. Le cœur relit cette policy
// pour décider QUAND exiger le PIN et quel périmètre l'IA peut toucher. Sans garde, un process local
// pouvait réécrire config.json à l'exécution pour désactiver l'exigence de PIN ou élargir le périmètre,
// SANS PIN.
//
// Garde : on SIGNE la projection de politique (SecurityDigest) avec une CLÉ D'APPAREIL (fichier dédié,
// 0600, propre au poste), et on VÉRIFIE la signature avant chaque décision. Une réécriture directe de
// config.json (hors SituLab) n'emporte pas de signature valide → le cœur retombe en POSITION FERMÉE
// (PIN exigé, lecture seule). SituLab re-signe à chaque changement légitime de politique.
//
// Pourquoi une clé d'appareil et non le PIN : la décision de politique (et le mode lecture seule) doit
// pouvoir être évaluée même SESSION VERROUILLÉE et par plusieurs process (fenêtre + démon lisent tous
// la même config). La clé est donc indépendante du PIN. Honnêteté : un process du MÊME compte OS peut
// lire le fichier de clé et forger une signature — comme il peut déjà dumper la mémoire du démon
// déverrouillé (modèle de menace déjà assumé). C'est une défense en profondeur qui ferme la réécriture
// TRIVIALE de la politique, pas une frontière contre un attaquant même-compte déterminé.

// deviceKeyPath : fichier de la clé d'appareil (même dossier restreint que le reste de l'état).
func deviceKeyPath() string { return filepath.Join(config.Dir(), "device.key") }

// ensureDeviceKey lit la clé d'appareil (32 octets) ou la crée si absente/corrompue.
//   - Fichier valide (32 o) → on l'utilise.
//   - Fichier ABSENT ou PRÉSENT MAIS INVALIDE (crash pendant l'écriture, troncature antivirus…) → on
//     (re)crée une clé fraîche par remplacement ATOMIQUE. Regénérer invalide toute PolicySig existante,
//     donc la politique retombe en position FERMÉE (direction sûre) — au lieu de l'ancien fail-OPEN où
//     un device.key à 0 octet rendait policySigValid toujours vrai (intégrité désactivée en silence).
//   - Erreur de lecture TRANSITOIRE (verrou/permission) → nil (dégradation gracieuse, sans clobber).
func ensureDeviceKey() []byte {
	p := deviceKeyPath()
	b, err := os.ReadFile(p)
	switch {
	case err == nil && len(b) == 32:
		return b
	case err == nil || os.IsNotExist(err):
		// Présent-mais-invalide, ou absent : on pose une clé fraîche de façon atomique.
		return writeDeviceKeyAtomic(p)
	default:
		return nil // erreur de lecture transitoire : ne pas écraser une clé peut-être bonne
	}
}

// writeDeviceKeyAtomic écrit une nouvelle clé d'appareil par temp+fsync+rename (jamais de fichier
// partiel même en cas de crash), puis RELIT le fichier pour adopter la clé réellement persistée (course
// multi-process au tout premier lancement). Renvoie la clé effective, ou nil sur échec d'E/S.
func writeDeviceKeyAtomic(p string) []byte {
	if err := os.MkdirAll(config.Dir(), 0o700); err != nil {
		return nil
	}
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		return nil
	}
	tmp, err := os.CreateTemp(config.Dir(), "device-*.tmp")
	if err != nil {
		return nil
	}
	tn := tmp.Name()
	defer os.Remove(tn)
	if _, err := tmp.Write(k); err != nil {
		tmp.Close()
		return nil
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return nil
	}
	if err := tmp.Close(); err != nil {
		return nil
	}
	_ = os.Chmod(tn, 0o600)
	if err := os.Rename(tn, p); err != nil {
		return nil
	}
	if fb, e := os.ReadFile(p); e == nil && len(fb) == 32 {
		return fb // adopter la clé réellement sur disque (une autre instance a pu gagner la course)
	}
	return k
}

// policyMAC calcule le HMAC-SHA256 de l'empreinte de politique avec la clé d'appareil.
func policyMAC(key []byte, c *config.Config) string {
	m := hmac.New(sha256.New, key)
	m.Write(c.SecurityDigest())
	return hex.EncodeToString(m.Sum(nil))
}

// signPolicy (re)calcule et pose la signature d'intégrité de la politique sur c.
func signPolicy(key []byte, c *config.Config) {
	if key == nil {
		return
	}
	c.PolicySig = policyMAC(key, c)
}

// policySigValid vérifie la signature d'intégrité de la politique (comparaison à temps constant).
func policySigValid(key []byte, c *config.Config) bool {
	if key == nil {
		return true // clé indisponible : dégradation gracieuse (comportement d'avant 1.16)
	}
	if c.PolicySig == "" {
		return false // jamais signée après ancrage = position fermée
	}
	return hmac.Equal([]byte(c.PolicySig), []byte(policyMAC(key, c)))
}

// policyOK dit si la politique portée par c est INTÈGRE (signature valide, ou clé indisponible).
// Utilisé par les décisions de sécurité ; false ⇒ position fermée (PIN exigé, lecture seule).
func (s *Service) policyOK(c *config.Config) bool {
	return policySigValid(s.devKey, c)
}

// resignPolicy re-signe la policy après un changement LÉGITIME (setters window-only). À appeler juste
// avant cfg.Save() dans toute mutation qui touche un champ de politique.
func (s *Service) resignPolicy(c *config.Config) {
	signPolicy(s.devKey, c)
}

// assertPolicyIntact refuse une mutation de politique si la config SUR DISQUE est signée MAIS falsifiée
// (signature présente et invalide). Sans ce garde, un setter re-signerait la config entière — y compris
// un champ voisin empoisonné — et « blanchirait » la falsification (laundering). À appeler en tête des
// setters de politique NON gardés par assertWritable (les autres ferment déjà via ReadOnly fail-closed).
func (s *Service) assertPolicyIntact() error {
	if s.devKey == nil {
		return nil
	}
	cfg, err := config.Load()
	if err != nil {
		return nil // illisible : traité en fail-closed par les décisions de lecture
	}
	if cfg.PolicySig != "" && !policySigValid(s.devKey, cfg) {
		return ErrPolicyTampered
	}
	return nil
}

// SecurityPolicyTampered dit si la politique sur disque est signée MAIS invalide (ErrPolicyTampered) :
// l'UI s'en sert pour proposer la RE-VALIDATION explicite (RevalidateSecurityPolicy). Toute autre
// situation — intègre, jamais signée, clé indisponible, config illisible — renvoie false : on ne signale
// « à re-valider » que le cas réel (clé d'appareil renouvelée, mise à niveau, édition hors SituLab).
func (s *Service) SecurityPolicyTampered() bool {
	return errors.Is(s.assertPolicyIntact(), ErrPolicyTampered)
}

// ReanchorPolicy re-pose la signature d'intégrité sur la policy COURANTE du disque. Usage LOCAL/FENÊTRE
// uniquement (jamais exposé au socket/CLI/MCP — sinon un appelant pourrait « bénir » une falsification) :
// après une modification de config autorisée hors des setters (migration, import, réparation).
func (s *Service) ReanchorPolicy() { s.retrustPolicy() }

// retrustPolicy re-signe la policy TELLE QUELLE sur le disque : à utiliser après un changement de
// politique effectué hors des setters window-only (notamment les tests qui préparent un scénario en
// écrivant config.json directement — en production, tout passe par les setters qui re-signent).
func (s *Service) retrustPolicy() {
	if s.devKey == nil {
		return
	}
	cfg, err := config.Load()
	if err != nil {
		return
	}
	signPolicy(s.devKey, cfg)
	s.ioMu.Lock()
	_ = cfg.Save()
	s.ioMu.Unlock()
}

// anchorPolicy pose la signature au démarrage si la policy n'a JAMAIS été signée (install fraîche ou
// mise à niveau depuis une version < 1.16) — « trust on first use ». Une signature présente mais
// INVALIDE n'est PAS re-signée (on ne bénit pas une falsification : les décisions fermeront la porte).
func (s *Service) anchorPolicy() {
	if s.devKey == nil {
		return
	}
	cfg, err := config.Load()
	if err != nil {
		return
	}
	if cfg.PolicySig == "" {
		signPolicy(s.devKey, cfg)
		s.ioMu.Lock()
		_ = cfg.Save()
		s.ioMu.Unlock()
	}
}
