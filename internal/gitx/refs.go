// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package gitx

import (
	"crypto/sha256"
	"encoding/hex"
)

// RefsDigest renvoie une empreinte stable de TOUTES les refs (objet + nom) d'un dépôt. Deux états
// identiques donnent la même empreinte ; le moindre commit/branche/tag la change. Sert au backup
// incrémental (v1.18.2) : si l'empreinte n'a pas bougé depuis la dernière sauvegarde, le dépôt est
// inchangé et peut être ignoré (sous réserve qu'une sauvegarde existe bien sur le cloud).
func RefsDigest(dir string) (string, error) {
	out, err := run(dir, LocalTimeout, "for-each-ref", "--format=%(objectname) %(refname)")
	if err != nil {
		return "", err
	}
	h := sha256.Sum256([]byte(out))
	return hex.EncodeToString(h[:]), nil
}
