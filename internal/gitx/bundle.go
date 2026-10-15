// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package gitx

import "strings"

// Bundle crée un bundle git de TOUT l'historique (`git bundle create <out> --all`) dans le fichier
// outPath. Un bundle est un fichier unique, autonome et restaurable par `git clone <bundle>` — idéal
// pour une sauvegarde miroir déposée dans un cloud de fichiers.
func Bundle(dir, outPath string) error {
	_, err := run(dir, LocalTimeout, "bundle", "create", outPath, "--all")
	return err
}

// TrackedFiles liste les fichiers SUIVIS du dépôt (git ls-files) — chemins relatifs à la racine,
// séparateurs « / ». Respecte .gitignore (seuls les fichiers versionnés). Pour le miroir « lisible ».
func TrackedFiles(dir string) ([]string, error) {
	out, err := run(dir, LocalTimeout, "ls-files", "-z")
	if err != nil {
		return nil, err
	}
	var files []string
	for _, p := range strings.Split(out, "\x00") {
		if p = strings.TrimSpace(p); p != "" {
			files = append(files, p)
		}
	}
	return files, nil
}
