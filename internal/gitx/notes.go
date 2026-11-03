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

// PushNotes publie la ref de notes <ref> (ex. "situlab") vers le distant, pour que la signature de
// garantie « SituLab approved » soit VISIBLE en ligne et par les autres clones — sans quoi l'attestation
// resterait purement locale. L'appelant l'invoque en best-effort après l'envoi du code : un échec
// (aucune note encore écrite, distant sans la ref) ne doit jamais faire échouer le push de code.
func PushNotes(dir, ref, token string, github bool, baseURL string) (string, error) {
	full := "refs/notes/" + ref
	return runC(dir, NetTimeout, authCfg(token, github, baseURL), "push", remoteName(dir), full+":"+full)
}

// FetchNotes récupère la ref de notes <ref> depuis le distant pour voir les attestations des autres
// (force « + » : les notes peuvent avoir été réécrites côté serveur). Best-effort chez l'appelant.
func FetchNotes(dir, ref, token string, github bool, baseURL string) (string, error) {
	full := "refs/notes/" + ref
	return runC(dir, NetTimeout, authCfg(token, github, baseURL), "fetch", remoteName(dir), "+"+full+":"+full)
}
