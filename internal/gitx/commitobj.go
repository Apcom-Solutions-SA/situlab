// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package gitx

import "strings"

// CommitObj est un objet commit décomposé (en-têtes + message), tel que lu par `git cat-file commit`.
// Sert à l'attestation « SituLab approved » v2 : signer un pré-image stable du commit puis recréer le
// commit à l'identique avec un trailer, sans réinventer git.
type CommitObj struct {
	Tree      string   `json:"tree"`
	Parents   []string `json:"parents"`
	Author    string   `json:"author"`    // ligne d'identité complète : « Nom <email> ts tz »
	Committer string   `json:"committer"` // idem
	Message   string   `json:"message"`

	AuthorName, AuthorEmail, AuthorDate          string
	CommitterName, CommitterEmail, CommitterDate string
}

// CommitObject lit et décompose l'objet commit d'un SHA (en-têtes + message). Le SHA est validé.
func CommitObject(dir, sha string) (*CommitObj, error) {
	if err := safeRevision(sha); err != nil {
		return nil, err
	}
	out, err := run(dir, LocalTimeout, "cat-file", "commit", sha)
	if err != nil {
		return nil, err
	}
	return parseCommitObject(out), nil
}

// parseCommitObject décompose la sortie de `git cat-file commit` : des en-têtes (tree/parent/author/
// committer, plus d'éventuels gpgsig multi-lignes ignorés), une ligne vide, puis le message. Fonction
// pure (testable sans git).
func parseCommitObject(raw string) *CommitObj {
	o := &CommitObj{}
	lines := strings.Split(raw, "\n")
	i := 0
	for ; i < len(lines); i++ {
		l := lines[i]
		if l == "" { // fin des en-têtes
			i++
			break
		}
		if strings.HasPrefix(l, " ") { // continuation d'en-tête (ex. gpgsig) : ignorée
			continue
		}
		sp := strings.IndexByte(l, ' ')
		if sp < 0 {
			continue
		}
		switch k, v := l[:sp], l[sp+1:]; k {
		case "tree":
			o.Tree = v
		case "parent":
			o.Parents = append(o.Parents, v)
		case "author":
			o.Author = v
		case "committer":
			o.Committer = v
		}
	}
	o.Message = strings.Join(lines[i:], "\n")
	o.AuthorName, o.AuthorEmail, o.AuthorDate = parseIdent(o.Author)
	o.CommitterName, o.CommitterEmail, o.CommitterDate = parseIdent(o.Committer)
	return o
}

// parseIdent décompose une ligne d'identité git « Nom <email> ts tz » en (nom, email, « ts tz »).
// Robuste à un nom contenant des espaces (on se repère sur le dernier « < » et le « > » qui suit).
func parseIdent(ident string) (name, email, date string) {
	lt := strings.LastIndexByte(ident, '<')
	if lt < 0 {
		return strings.TrimSpace(ident), "", ""
	}
	gt := strings.IndexByte(ident[lt:], '>')
	if gt < 0 {
		return strings.TrimSpace(ident[:lt]), strings.TrimSpace(ident[lt+1:]), ""
	}
	gt += lt
	name = strings.TrimSpace(ident[:lt])
	email = ident[lt+1 : gt]
	date = strings.TrimSpace(ident[gt+1:])
	return name, email, date
}

// RecommitWithTrailer recrée le commit `sha` à l'identique (même arbre, mêmes parents, MÊMES identités
// et dates auteur/committer) en ajoutant un trailer « key: val » à son message, puis avance la branche
// courante vers le nouveau commit. L'arbre et l'index ne bougent pas (même arbre) : l'état reste propre.
//
// C'est le portage IMMUABLE de l'attestation : le trailer fait partie de l'objet commit. Comme on
// reproduit à l'identique auteur/committer, le seul changement de contenu est l'ajout du trailer — ce
// que la vérification v2 reconstruit en retirant ce trailer (cf. attest.CommitPreimage).
func RecommitWithTrailer(dir, sha, key, val string) (string, error) {
	obj, err := CommitObject(dir, sha)
	if err != nil {
		return "", err
	}
	newMsg := appendTrailer(obj.Message, key, val)
	args := []string{"commit-tree", obj.Tree}
	for _, p := range obj.Parents {
		args = append(args, "-p", p)
	}
	args = append(args, "-m", newMsg)
	env := []string{
		"GIT_AUTHOR_NAME=" + obj.AuthorName,
		"GIT_AUTHOR_EMAIL=" + obj.AuthorEmail,
		"GIT_AUTHOR_DATE=" + obj.AuthorDate,
		"GIT_COMMITTER_NAME=" + obj.CommitterName,
		"GIT_COMMITTER_EMAIL=" + obj.CommitterEmail,
		"GIT_COMMITTER_DATE=" + obj.CommitterDate,
	}
	out, err := runC(dir, LocalTimeout, env, args...)
	if err != nil {
		return "", err
	}
	newSha := strings.TrimSpace(out)
	// Avance la branche courante vers le nouveau commit, en exigeant que HEAD vaille encore l'ancien
	// (compare-and-swap) : jamais écraser un commit survenu entre-temps.
	if _, err := run(dir, LocalTimeout, "update-ref", "HEAD", newSha, sha); err != nil {
		return "", err
	}
	return newSha, nil
}

// appendTrailer place « key: val » dans un bloc de trailer final, séparé du corps par une ligne vide.
// Déterministe et réversible : la vérification retire exactement cette ligne puis retaille les fins de
// ligne pour retrouver le corps signé.
func appendTrailer(message, key, val string) string {
	body := strings.TrimRight(message, "\n")
	return body + "\n\n" + key + ": " + val + "\n"
}
