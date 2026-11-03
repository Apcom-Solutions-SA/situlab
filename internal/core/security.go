// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"errors"
	"fmt"
	"os"
	"situlab/internal/config"
	"situlab/internal/keyring"
	"time"
)

// PINSet indique si un PIN est configuré (c.-à-d. si le trousseau est initialisé).
func (s *Service) PINSet() bool {
	return keyring.Initialized()
}

// isTrivialPIN rejette un code fait d'un seul caractère répété (000000, 111111…). La vraie résistance
// hors-ligne du trousseau volé tient à l'espace du PIN + au coût Argon2id (documenté) ; bannir des motifs
// n'y change presque rien — on écarte seulement le pire cas, sans contraindre l'usage clé-en-main.
func isTrivialPIN(code string) bool {
	if len(code) < 2 {
		return false
	}
	for i := 1; i < len(code); i++ {
		if code[i] != code[0] {
			return false
		}
	}
	return true
}

// SetPIN définit (1re fois) ou change le PIN.
//   - 1re fois : initialise le trousseau chiffré et OUVRE la session (la DEK naît ici).
//   - changement : l'ancien PIN est exigé et vérifié par le MÊME chemin verrouillé que
//     Push (sinon SetPIN serait un oracle de bruteforce), puis la DEK est ré-enveloppée
//     sous le nouveau PIN sans re-chiffrer les tokens.
func (s *Service) SetPIN(oldCode, newCode string) error {
	if len(newCode) < pinMinLen {
		return fmt.Errorf("le PIN doit comporter au moins %d caractères", pinMinLen)
	}
	if isTrivialPIN(newCode) {
		return errors.New("PIN trop prévisible : évitez un code fait d'un seul chiffre répété (ex. 000000)")
	}
	if !keyring.Initialized() {
		_, dek, err := keyring.Init(newCode)
		if err != nil {
			return err
		}
		s.openSession(dek)
		zero(dek) // openSession en a pris une copie ; on efface l'original (hygiène, comme Unlock)
		s.resetPINCounters()
		s.migrateVaultOnce()
		return nil
	}
	// Changement : vérifier l'ancien PIN (rate-limité) → récupérer la DEK → ré-envelopper.
	dek, ok, err := s.verifyPINGated(oldCode)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("ancien PIN incorrect")
	}
	defer zero(dek)
	// La ré-enveloppe mute le trousseau : elle doit passer par ioMu comme toute mutation
	// persistante (sinon course last-writer-wins avec AddInstanceLogin, etc.).
	s.ioMu.Lock()
	kr, err := keyring.Load()
	if err != nil {
		s.ioMu.Unlock()
		return err
	}
	if err := kr.Rewrap(dek, newCode); err != nil {
		s.ioMu.Unlock()
		return err
	}
	s.ioMu.Unlock()
	s.openSession(dek)
	s.migrateVaultOnce()
	return nil
}

// ResetKeyring efface le trousseau chiffré (cas « PIN oublié » : le PIN étant la seule clé,
// il est irrécupérable). Les instances sont CONSERVÉES dans la config mais perdent leur token
// (elles deviennent « déconnectées ») : l'utilisateur recrée un PIN puis les reconnecte une à
// une. Session fermée, compteurs remis à zéro. Les dépôts locaux ne sont PAS touchés.
func (s *Service) ResetKeyring() error {
	s.ioMu.Lock()
	if err := os.Remove(keyring.Path()); err != nil && !os.IsNotExist(err) {
		s.ioMu.Unlock()
		return err
	}
	s.ioMu.Unlock()
	s.Lock()
	s.mu.Lock()
	s.pinFails, s.pinLockTo, s.migrated = 0, 0, false
	s.mu.Unlock()
	return nil
}

// DisconnectedInstances renvoie les noms des instances sans token valide dans le trousseau
// (après un reset de PIN, ou si le trousseau n'est pas initialisé) — à reconnecter.
func (s *Service) DisconnectedInstances() []string {
	cfg, err := config.Load()
	if err != nil {
		return nil
	}
	kr, kerr := keyring.Load()
	var out []string
	for _, in := range cfg.Instances {
		if kerr != nil || !kr.Has(in.Name) {
			out = append(out, in.Name)
		}
	}
	return out
}

// lockTTL renvoie le délai d'auto-verrouillage réglé par l'utilisateur (config LockAfterMin) :
// 0 ⇒ défaut (defaultLockTTL) ; -1 ⇒ « jamais » (durée très longue, effectivement sans expiration).
func (s *Service) lockTTL() time.Duration {
	m := 0
	if c, err := config.Load(); err == nil {
		m = c.LockAfterMin
	}
	switch {
	case m < 0:
		return 100 * 365 * 24 * time.Hour // « jamais »
	case m == 0:
		return defaultLockTTL // défaut durci (1.16)
	default:
		return time.Duration(m) * time.Minute
	}
}

// LockDelay renvoie le réglage courant (minutes ; -1 = jamais ; 0 = défaut 5, durci en 1.16).
func (s *Service) LockDelay() int {
	if c, err := config.Load(); err == nil {
		return c.LockAfterMin
	}
	return 0
}

// SetLockDelay enregistre le délai d'auto-verrouillage (minutes ; -1 = jamais ; 0 = défaut)
// et le ré-applique à la session en cours.
func (s *Service) SetLockDelay(min int) error {
	s.ioMu.Lock()
	cfg, err := config.Load()
	if err != nil {
		s.ioMu.Unlock()
		return err
	}
	cfg.LockAfterMin = min
	if err := cfg.Save(); err != nil {
		s.ioMu.Unlock()
		return err
	}
	s.publish(cfg)
	s.ioMu.Unlock()
	// Ré-arme la session ouverte avec le nouveau délai.
	s.mu.Lock()
	if len(s.sessDEK) > 0 {
		s.sessTTL = s.lockTTL()
		s.sessUntil = time.Now().Add(s.sessTTL)
		if s.lockTimer != nil {
			s.lockTimer.Stop()
		}
		s.lockTimer = time.AfterFunc(s.sessTTL, s.lockIfExpired)
	}
	s.mu.Unlock()
	return nil
}

// LockAfterWrite indique si la session doit se reverrouiller après chaque écriture réussie.
func (s *Service) LockAfterWrite() bool {
	if c, err := config.Load(); err == nil {
		return c.LockAfterWrite
	}
	return false
}

// SetLockAfterWrite active/désactive le verrouillage après écriture.
func (s *Service) SetLockAfterWrite(on bool) error {
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.LockAfterWrite = on
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}

// ReadOnly indique si le mode lecture seule est actif (v1.4) : toute mutation est alors refusée.
// FAIL-CLOSED : si la config est illisible (erreur d'E/S, verrou antivirus…), on considère la
// lecture seule ACTIVE — un garde-fou de sûreté doit échouer en position fermée. (Une config
// absente renvoie une config vide SANS erreur, donc une install neuve n'est pas bloquée.)
func (s *Service) ReadOnly() bool {
	c, err := config.Load()
	if err != nil {
		return true
	}
	if !s.policyOK(c) {
		return true // politique falsifiée → position fermée (lecture seule), fermeture P3
	}
	return c.ReadOnly
}

// AgentNotes renvoie la fiche de collaboration avec l'IA (consignes libres de l'utilisateur,
// reprises dans AGENTS.md au moment du dépôt). v1.5.1.
func (s *Service) AgentNotes() string {
	if c, err := config.Load(); err == nil {
		return c.AgentNotes
	}
	return ""
}

// SetAgentNotes enregistre la fiche de collaboration avec l'IA.
func (s *Service) SetAgentNotes(notes string) error {
	if err := s.assertWritable(); err != nil {
		return err
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.AgentNotes = notes
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}

// assertWritable renvoie ErrReadOnly si le mode lecture seule est actif. À appeler en tête de
// TOUTE mutation qui ne passe pas déjà par gatePIN (mutations git locales, mutations de config) :
// c'est le point unique qui garantit que « lecture seule » couvre vraiment toute modification,
// sur les trois surfaces (fenêtre, CLI, MCP).
func (s *Service) assertWritable() error {
	if s.ReadOnly() {
		return ErrReadOnly
	}
	return nil
}

// SetReadOnly active/désactive le mode lecture seule.
func (s *Service) SetReadOnly(on bool) error {
	if err := s.assertPolicyIntact(); err != nil { // refus de re-signer par-dessus une policy falsifiée
		return err
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.ReadOnly = on
	s.resignPolicy(cfg) // read_only fait partie de la policy signée (fermeture P3)
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}

// resetPINCounters remet à zéro le compteur anti-bruteforce (succès / init).
func (s *Service) resetPINCounters() {
	s.mu.Lock()
	s.pinFails = 0
	s.pinLockTo = 0
	s.mu.Unlock()
	s.savePINGuard() // persiste la remise à zéro (hors s.mu)
}

// verifyPINGated vérifie un code en tentant de déballer la DEK du trousseau, en RÉSERVANT
// un essai de façon atomique AVANT le calcul coûteux (Argon2id). Ainsi le verrou 5 essais
// / 60 s résiste à la concurrence intra-processus (TOCTOU éliminé) : au plus pinMaxFails
// dérivations par fenêtre, les appels en excès sont rejetés à la porte. Renvoie (dek, ok,
// err) ; err non nil signale un verrou actif, l'absence de trousseau, ou un fichier
// illisible. En cas de succès la DEK est renvoyée (l'appelant l'ouvre en session ou l'efface).
func (s *Service) verifyPINGated(code string) ([]byte, bool, error) {
	s.mu.Lock()
	now := time.Now().Unix()
	if s.pinLockTo > now {
		wait := s.pinLockTo - now
		s.mu.Unlock()
		return nil, false, fmt.Errorf("%w : réessayez dans %d s", ErrPINLocked, wait)
	}
	if !keyring.Initialized() {
		s.mu.Unlock()
		return nil, false, errors.New("aucun PIN configuré : définissez-en un d'abord")
	}
	// Réservation atomique d'un créneau d'essai, verrou tenu, AVANT la dérivation Argon2id.
	// Compteur EN MÉMOIRE (s.mu) : aucune écriture de config concurrente ne peut l'écraser.
	if s.pinFails >= pinMaxFails {
		s.pinLockTo = time.Now().Add(pinLockFor).Unix()
		s.pinFails = 0
		s.mu.Unlock()
		s.savePINGuard() // le verrou doit survivre à un redémarrage de l'app
		return nil, false, fmt.Errorf("%w : verrouillé %d s après %d essais", ErrPINLocked, int(pinLockFor.Seconds()), pinMaxFails)
	}
	s.pinFails++
	s.mu.Unlock()
	s.savePINGuard() // persiste le compteur d'essais (hors s.mu)

	kr, lerr := keyring.Load() // hors verrou
	if lerr != nil {
		return nil, false, errors.New("trousseau illisible : redéfinissez le PIN")
	}
	dek, derr := kr.Unlock(code) // CPU (Argon2id), hors verrou
	if derr != nil {
		return nil, false, nil // PIN faux : l'essai est déjà comptabilisé par la réservation
	}

	// Succès : on remet le compteur à zéro.
	s.resetPINCounters()
	return dek, true, nil
}

// VerifyPIN vérifie un PIN À BLANC (sans ouvrir de session), throttlé par le MÊME verrou anti-bruteforce
// que le déverrouillage. La fenêtre s'en sert pour signaler un PIN FAUX avant de soumettre la demande au
// démon — au lieu de refermer la modale en silence et de laisser l'action échouer sans explication.
// Renvoie (ok, err) ; err non nil = verrou actif, trousseau absent/illisible. La DEK n'est jamais retenue.
func (s *Service) VerifyPIN(code string) (bool, error) {
	dek, ok, err := s.verifyPINGated(code)
	if ok {
		zero(dek)
	}
	return ok, err
}
