// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package gitx

import "strings"

// Bundle crée un bundle git de TOUT l'historique (`git bundle create <out> --all`) dans le fichier
// outPath. Un bundle est un fichier unique, autonome et restaurable par `git clone <bundle>` — idéal
// pour une sauvegarde miroir déposée dans un cloud de fichiers.
func Bundle(dir, outPath string) error {
	// BundleTimeout (genereux) et non LocalTimeout : empaqueter tout l'historique d'un vrai depot depasse
	// 20 s (pack-objects complet, d'autant plus avec un antivirus scannant le .bundle).
	_, err := run(dir, BundleTimeout, "bundle", "create", outPath, "--all")
	return err
}

// BundleRemotes empaquette les refs d'UN remote (refs/remotes/<remote>/*) plutôt que --all : sert à
// sauvegarder l'état de l'ATELIER (origin) et non les branches locales de travail (1.19, #25). Le
// bundle reste autonome et restaurable par `git clone <bundle>`.
func BundleRemotes(dir, outPath, remote string) error {
	_, err := run(dir, BundleTimeout, "bundle", "create", outPath, "--remotes="+remote)
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
