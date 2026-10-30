// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	"situlab/internal/config"
	"situlab/internal/keyring"
	"situlab/internal/vault"
)

// Gestion de la SESSION : la DEK du trousseau, tenue en mémoire après déverrouillage par
// le PIN. Les lectures exigent une session déverrouillée (révision d'invariant v1.1.0) ;
// les écritures confirment le PIN à chaque fois (gatePIN) et rafraîchissent la session.
//
// Rappel d'architecture : la GUI (et le futur démon, Phase 4) est un process long qui
// porte cette session ; une commande CLI one-shot repart toujours verrouillée.

// Purge efface TOUTES les données locales de SituLab de ce poste, pour repartir à blanc ou céder une
// machine. IRRÉVERSIBLE. N'exige ni PIN ni session (c'est aussi l'issue de secours quand le PIN est
// oublié) ; la confirmation se fait côté interface/CLI. Vérification de complétude (1.17) — tout ce que
// SituLab garde est couvert :
//   - jetons des connexions GIT        → coffre OS (vault), effacés par connexion (boucle 1) ;
//   - secrets des connexions GIT & CLOUD → trousseau chiffré (keyring.json), dans le dossier de config ;
//   - fiche AGENTS.md administrée à la source, config.json, device.key (signature), pinguard.json
//     (anti-bruteforce), journal des sauvegardes, socket → dossier de config ;
//   - cache d'interface (1.18.3, sous config.Dir()/cache) et rapports d'analyse d'équipe
//     (config.Dir()/reports) → emportés par le RemoveAll du dossier de config ;
//   - données du navigateur embarqué (WebView2) → best-effort (verrouillées tant que la fenêtre tourne).
//
// Un test de régression (TestPurgeWipesData) sème une entrée de cache et la fiche, et vérifie qu'elles
// disparaissent : garde anti-dérive si l'un de ces états quittait un jour config.Dir() sans suivre Purge.
//
// Les connexions CLOUD n'utilisent PAS le coffre OS (uniquement le trousseau) — supprimer le dossier de
// config les efface donc toutes. L'interface vide en plus son localStorage et recharge.
func (s *Service) Purge() error {
	// 1) Connexions GIT : retirer du coffre OS le jeton de chaque connexion connue, AVANT d'effacer la
	//    config (après, on ne saurait plus lesquelles énumérer). Les clouds ne sont pas au coffre OS.
	if cfg, err := config.Load(); err == nil {
		for _, in := range cfg.Instances {
			_ = vault.DeleteToken(in.Name)
		}
	}
	// 2) Fermer la session en mémoire (DEK).
	s.Lock()
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	// 3) Fiche AGENTS.md administrée à la source : effacement EXPLICITE (elle vit dans le dossier de
	//    config, donc le RemoveAll ci-dessous la retire aussi — on l'enlève d'abord par clarté/robustesse).
	_ = os.Remove(config.AgentsDocPath())
	// 4) Effacer le dossier de configuration ENTIER : trousseau (tous les secrets git & cloud), config,
	//    device.key, pinguard.json, journal des sauvegardes, socket, .bak…
	err := os.RemoveAll(config.Dir())
	// 5) Best-effort : le dossier de données WebView2 de cet exécutable (%AppData%\<exe>).
	if exe, e := os.Executable(); e == nil {
		if base, be := os.UserConfigDir(); be == nil {
			_ = os.RemoveAll(filepath.Join(base, filepath.Base(exe)))
		}
	}
	return err
}

// Unlock ouvre la session de lecture : vérifie le PIN (chemin anti-bruteforce) et garde la
// DEK en mémoire jusqu'au timeout d'inactivité.
func (s *Service) Unlock(pinCode string) error {
	dek, ok, err := s.verifyPINGated(pinCode)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("PIN incorrect")
	}
	s.openSession(dek)
	zero(dek) // openSession en a pris une copie
	s.migrateVaultOnce()
	return nil
}

// migrateVaultOnce tente d'importer les tokens de l'ancien coffre OS (best-effort,
// silencieux). À appeler quand une session vient d'être ouverte. RETENTÉ à chaque ouverture
// tant que la migration n'a pas entièrement réussi : on ne marque `migrated` qu'au succès
// complet (sinon une migration partielle laisserait des tokens bloqués au coffre, jamais
// réimportés pour toute la vie du process).
func (s *Service) migrateVaultOnce() {
	s.mu.Lock()
	done := s.migrated
	s.mu.Unlock()
	if done {
		return
	}
	if _, err := s.MigrateFromVault(); err == nil {
		s.mu.Lock()
		s.migrated = true
		s.mu.Unlock()
	}
	// En cas d'erreur : `migrated` reste false -> nouvelle tentative à la prochaine ouverture.
}

// vaultResidual compte les instances ayant encore un token dans l'ancien coffre OS
// (doit valoir 0 après migration). Best-effort : une erreur de coffre compte pour 0.
func (s *Service) vaultResidual() int {
	s.mu.Lock()
	s.lockedReload()
	instances := append([]config.Instance(nil), s.cfg.Instances...)
	s.mu.Unlock()
	n := 0
	for _, in := range instances {
		if t, err := vault.GetToken(in.Name); err == nil && t != "" {
			n++
		}
	}
	return n
}

// Lock referme la session et efface la DEK en mémoire.
func (s *Service) Lock() {
	s.mu.Lock()
	s.clearSessionLocked()
	s.mu.Unlock()
}

// clearSessionLocked zéroise la DEK et arrête la minuterie (l'appelant tient s.mu).
func (s *Service) clearSessionLocked() {
	if s.lockTimer != nil {
		s.lockTimer.Stop()
		s.lockTimer = nil
	}
	zero(s.sessDEK)
	s.sessDEK = nil
	s.sessUntil = time.Time{}
}

// Unlocked indique si une session de lecture est active (non expirée).
func (s *Service) Unlocked() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sessDEK) > 0 && s.sessUntil.After(time.Now())
}

// openSession installe une COPIE de la DEK en session et (ré)arme le timeout + la minuterie
// de zéroisation.
func (s *Service) openSession(dek []byte) {
	ttl := s.lockTTL()
	cp := make([]byte, len(dek))
	copy(cp, dek)
	s.mu.Lock()
	zero(s.sessDEK)
	s.sessDEK = cp
	s.sessTTL = ttl
	s.sessUntil = time.Now().Add(ttl)
	if s.lockTimer != nil {
		s.lockTimer.Stop()
	}
	s.lockTimer = time.AfterFunc(ttl, s.lockIfExpired)
	s.mu.Unlock()
}

// lockIfExpired est appelée par la minuterie : efface la DEK si le TTL est écoulé, sinon
// ré-arme pour le temps restant (la session a glissé suite à un usage récent). Garantit que
// la clé est zéroisée APRÈS le re-verrouillage même en l'absence de tout appel ultérieur.
func (s *Service) lockIfExpired() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.sessDEK) == 0 {
		return
	}
	if s.sessUntil.After(time.Now()) {
		s.lockTimer = time.AfterFunc(time.Until(s.sessUntil), s.lockIfExpired)
		return
	}
	s.clearSessionLocked()
}

// sessionDEK renvoie une COPIE de la DEK si la session est déverrouillée (et fait glisser
// le timeout d'inactivité), sinon ErrLocked. À l'expiration, la DEK est effacée sur-le-champ.
// L'appelant efface la copie après usage.
func (s *Service) sessionDEK() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.sessDEK) == 0 {
		return nil, ErrLocked
	}
	if !s.sessUntil.After(time.Now()) {
		s.clearSessionLocked() // expirée : on zéroise immédiatement
		return nil, ErrLocked
	}
	ttl := s.sessTTL
	if ttl <= 0 {
		ttl = defaultLockTTL
	}
	s.sessUntil = time.Now().Add(ttl) // glissement sur usage
	cp := make([]byte, len(s.sessDEK))
	copy(cp, s.sessDEK)
	return cp, nil
}

// tokenFor renvoie le token déchiffré d'une instance via la session déverrouillée.
func (s *Service) tokenFor(instance string) (string, error) {
	dek, err := s.sessionDEK()
	if err != nil {
		return "", err
	}
	defer zero(dek)
	kr, err := keyring.Load()
	if err != nil {
		return "", err
	}
	return kr.GetToken(dek, instance)
}

// MigrateFromVault importe, une fois, les tokens de l'ancien coffre OS (≤ v1.0) vers le
// trousseau chiffré, puis les retire du coffre. Nécessite une session déverrouillée
// (donc un PIN déjà défini). Renvoie le nombre de tokens migrés. Idempotent : les
// instances sans token au coffre sont simplement ignorées.
func (s *Service) MigrateFromVault() (int, error) {
	dek, err := s.sessionDEK()
	if err != nil {
		return 0, err
	}
	defer zero(dek)

	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return 0, err
	}
	kr, err := keyring.Load()
	if err != nil {
		return 0, err
	}
	migrated := 0
	var failed error
	for _, in := range cfg.Instances {
		tok, verr := vault.GetToken(in.Name)
		if verr != nil || tok == "" {
			continue // rien au coffre pour cette instance
		}
		// Le token existe encore au coffre : s'assurer qu'il est dans le trousseau...
		if !kr.Has(in.Name) {
			if err := kr.SetToken(dek, in.Name, in.URL, nil, tok); err != nil {
				failed = err
				continue // on NE purge PAS le coffre si l'écriture chiffrée a échoué
			}
			migrated++
		}
		// ...puis PURGER le coffre (toujours tenté, même si l'entrée y était déjà chiffrée :
		// une purge ratée précédemment ne doit pas rester bloquée par le court-circuit kr.Has).
		if err := vault.DeleteToken(in.Name); err != nil {
			failed = err
		}
	}
	return migrated, failed // failed non nil -> migrated reste false -> retenté plus tard
}

// zero efface un tampon sensible en place.
func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
