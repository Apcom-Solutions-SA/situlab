// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package gitx

// Bundle crée un bundle git de TOUT l'historique (`git bundle create <out> --all`) dans le fichier
// outPath. Un bundle est un fichier unique, autonome et restaurable par `git clone <bundle>` — idéal
// pour une sauvegarde miroir déposée dans un cloud de fichiers.
func Bundle(dir, outPath string) error {
	_, err := run(dir, LocalTimeout, "bundle", "create", outPath, "--all")
	return err
}
