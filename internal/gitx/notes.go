// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package gitx

import "strings"

// HeadSHA renvoie le SHA du commit courant (HEAD), ou "" si indéterminé.
func HeadSHA(dir string) string {
	out, err := run(dir, LocalTimeout, "rev-parse", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// WriteNote attache (ou remplace) une note à un commit dans une ref de notes dédiée (ex. "situlab").
// Le contenu n'est PAS secret (attestation publique) : passage par argv accepté. Le SHA est validé.
func WriteNote(dir, ref, sha, content string) error {
	if err := safeRevision(sha); err != nil {
		return err
	}
	_, err := run(dir, LocalTimeout, "notes", "--ref="+ref, "add", "-f", "-m", content, "--", sha)
	return err
}

// ReadNote renvoie la note d'un commit dans la ref donnée, ou "" (sans erreur) si aucune note : un
// commit sans note = non attesté, pas une panne.
func ReadNote(dir, ref, sha string) string {
	out, err := run(dir, LocalTimeout, "notes", "--ref="+ref, "show", sha)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}
