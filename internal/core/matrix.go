// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

// La MATRICE des dépôts (v1.11) — le service que SituLab doit rendre : non pas « switcher d'instance
// et lire des états » (vue de gestionnaire), mais RELIER et ASSEMBLER les dépôts entre eux —
// clone local ↔ dépôt atelier ↔ dépôt vitrine. La matrice renvoie des LIGNES ALIGNÉES : une ligne par
// dépôt logique, trois cases (local, atelier, vitrine) qui se font face. Une case vide porte une
// action « créer » (plug-and-play, pour un public non-dev). Critère d'appairage déjà en vigueur :
// l'URL du distant normalisée (normURL) relie un clone local à son dépôt distant, et le nom du dépôt
// relie un atelier à ses vitrines. Lecture seule : la matrice propose, l'humain confirme.

package core

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"situlab/internal/config"
)

// MatrixCell : une case de la matrice (un emplacement local, atelier ou vitrine). Present=false = case
// vide, à proposer à la création.
type MatrixCell struct {
	Present   bool   `json:"present"`
	Name      string `json:"name,omitempty"`
	Org       string `json:"org,omitempty"`        // propriétaire / organisation (affiché en petit)
	Instance  string `json:"instance,omitempty"`   // connexion git (atelier/vitrine)
	FullName  string `json:"full_name,omitempty"`  // owner/repo (pour ouvrir le détail / cloner)
	LocalPath string `json:"local_path,omitempty"` // chemin du clone, si présent sur le poste
	// Suggested : vitrine PROPOSÉE (match par nom ou suggestion manuelle VitrineHint) mais PAS encore
	// confirmée par un appariement de projet. L'UI la distingue d'une vitrine confirmée (✓) pour ne pas
	// laisser croire que l'appairage est fait (1.19, #16). Toujours false pour local/atelier.
	Suggested bool `json:"suggested,omitempty"`
}

// MatrixRow : une ligne alignée local ↔ atelier ↔ vitrine(s).
//   - Pending   = pas encore rangée dans un projet → « à traiter » (compteur / facette / mise en route).
//     Inclut les dépôts SEULS dans une colonne (atelier seul, vitrine seule) : incomplets, à compléter
//     ou à archiver — ils ne doivent pas disparaître du compte.
//   - Alignable = il y a VRAIMENT de quoi apparier maintenant (atelier + au moins une contrepartie
//     locale/vitrine) → le bouton « Appairer » n'apparaît que dans ce cas.
type MatrixRow struct {
	Key       string       `json:"key"`
	Name      string       `json:"name"`
	Local     MatrixCell   `json:"local"`
	Atelier   MatrixCell   `json:"atelier"`
	Vitrines  []MatrixCell `json:"vitrines"`
	Project   string       `json:"project,omitempty"`
	Pending   bool         `json:"pending"`
	Alignable bool         `json:"alignable"`
}

// RepoMatrix : les lignes alignées + le nombre d'appariements en attente + les lignes archivées
// (suggestions rangées) + les connexions injoignables.
type RepoMatrix struct {
	Rows        []MatrixRow `json:"rows"`
	Pending     int         `json:"pending"`
	Archived    []MatrixRow `json:"archived,omitempty"`
	Unreachable []string    `json:"unreachable,omitempty"`
}

// matrixRepo : un dépôt distant intermédiaire (non exporté) le temps de bâtir les lignes.
type matrixRepo struct {
	instance, owner, repo, fullName, remote, localPath string
}

// dedupRepos retire les doublons de DÉPÔT (1.19) : un même dépôt distant vu par DEUX connexions du
// même hôte — par exemple deux jetons GitHub qui voient tous deux Apcom-Solutions-SA/situlab — ne doit
// produire qu'UNE ligne, pas deux. Clé = remote normalisé (host + chemin) ; un remote vide retombe sur
// instance|owner|repo pour ne jamais fusionner deux dépôts réellement distincts. La PREMIÈRE occurrence
// (ordre des connexions) est conservée.
func dedupRepos(in []matrixRepo) []matrixRepo {
	seen := make(map[string]bool, len(in))
	out := make([]matrixRepo, 0, len(in))
	for _, r := range in {
		key := r.remote
		if key == "" {
			key = r.instance + "|" + r.owner + "|" + r.repo
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, r)
	}
	return out
}

// RepoMatrix construit la matrice alignée. Action délibérée de l'humain (réseau) : à présenter avec un
// indicateur de progression. Best-effort : une connexion injoignable est signalée, pas bloquante.
func (s *Service) RepoMatrix() (*RepoMatrix, error) {
	insts := s.Instances()
	localIdx := s.localRemoteIndex() // distant normalisé → chemin du clone
	projects := s.Projects()

	projByAtelier := map[string]string{}
	prodByAtelier := map[string][]config.RepoRef{} // vitrines DÉCLARÉES par projet (noms ≠ atelier)
	prodKeys := map[string]bool{}                  // refKey de chaque vitrine déjà rattachée à un atelier
	for _, p := range projects {
		projByAtelier[refKey(p.Atelier)] = p.Name
		prodByAtelier[refKey(p.Atelier)] = p.Productions
		for _, pr := range p.Productions {
			prodKeys[refKey(pr)] = true
		}
	}

	// Clés d'appariement archivées (suggestions rangées) + suggestions MANUELLES atelier→vitrine
	// (assemblées mais pas encore confirmées — bouton « Appairer » à cliquer).
	hintByAtelier := map[string][]config.RepoRef{}
	s.mu.Lock()
	s.lockedReload()
	archived := map[string]bool{}
	for _, k := range s.cfg.ArchivedPairings {
		archived[k] = true
	}
	for _, h := range s.cfg.VitrineHints {
		ak := refKey(h.Atelier)
		hintByAtelier[ak] = append(hintByAtelier[ak], h.Vitrine)
		prodKeys[refKey(h.Vitrine)] = true // rattachée → ne pas la relister en orpheline
	}
	s.mu.Unlock()

	out := &RepoMatrix{}
	var ateliers, vitrines []matrixRepo
	for _, inst := range insts {
		repos, err := s.Repos(inst.Name)
		if err != nil {
			out.Unreachable = append(out.Unreachable, inst.Name)
			continue
		}
		for _, r := range repos {
			owner, repo := splitFull(r.FullName, r.Name)
			mr := matrixRepo{instance: inst.Name, owner: owner, repo: repo, fullName: r.FullName, remote: normURL(r.CloneURL)}
			mr.localPath = localIdx[mr.remote]
			if inst.NormalizedRole() == config.RoleProduction {
				vitrines = append(vitrines, mr)
			} else {
				ateliers = append(ateliers, mr)
			}
		}
	}
	// Dédoublonnage par dépôt AVANT de bâtir les lignes : deux connexions du même hôte voyant le même
	// dépôt ne doivent pas produire de doublon (ni en vitrine orpheline, ni dans les vitrines d'une ligne).
	vitrines = dedupRepos(vitrines)
	ateliers = dedupRepos(ateliers)

	atelierRemotes := map[string]bool{}
	atelierNames := map[string]bool{}
	for _, a := range ateliers {
		if a.remote != "" {
			atelierRemotes[a.remote] = true
		}
		atelierNames[strings.ToLower(a.repo)] = true
	}
	vitrineRemotes := map[string]bool{}
	for _, v := range vitrines {
		if v.remote != "" {
			vitrineRemotes[v.remote] = true
		}
	}

	var rows []MatrixRow
	// Lignes ancrées sur un atelier : local (même distant) + vitrines (même nom).
	for _, a := range ateliers {
		row := MatrixRow{
			Key:     refKey(config.RepoRef{Instance: a.instance, Owner: a.owner, Repo: a.repo}),
			Name:    a.repo,
			Atelier: MatrixCell{Present: true, Name: a.repo, Org: a.owner, Instance: a.instance, FullName: a.fullName, LocalPath: a.localPath},
		}
		if a.localPath != "" {
			row.Local = MatrixCell{Present: true, Name: baseName(a.localPath), LocalPath: a.localPath}
		}
		// Vitrines CONFIRMÉES de ce projet (si appairé) : toute vitrine hors de cet ensemble n'est que
		// PROPOSÉE (match par nom ou suggestion VitrineHint) → Suggested=true pour l'UI (#16, 1.19).
		confirmed := map[string]bool{}
		for _, pr := range prodByAtelier[row.Key] {
			confirmed[refKey(pr)] = true
		}
		// Une vitrine hors de l'ensemble confirmé n'est que PROPOSÉE (Suggested) : on l'affiche quand même
		// (même sur une ligne déjà appairée), mais marquée « à confirmer » et CLIQUABLE pour la confirmer
		// dans le projet (ConfirmVitrine) — jamais masquée en silence (#16, 1.19).
		for _, v := range vitrines {
			if strings.EqualFold(v.repo, a.repo) {
				vk := refKey(config.RepoRef{Instance: v.instance, Owner: v.owner, Repo: v.repo})
				row.Vitrines = append(row.Vitrines, MatrixCell{Present: true, Name: v.repo, Org: v.owner, Instance: v.instance, FullName: v.fullName, LocalPath: v.localPath, Suggested: !confirmed[vk]})
			}
		}
		// Vitrines rattachées explicitement (nom ≠ atelier — ex. glim↔glim_v2), en plus du match par
		// nom et sans doublon : celles du PROJET (confirmées) ET les SUGGESTIONS manuelles (assemblées,
		// pas encore confirmées — elles rendent la ligne « alignable » → bouton « Appairer »).
		declared := append(append([]config.RepoRef{}, prodByAtelier[row.Key]...), hintByAtelier[row.Key]...)
		for _, pr := range declared {
			prk := refKey(pr)
			dup := false
			for _, ex := range row.Vitrines {
				if refKey(config.RepoRef{Instance: ex.Instance, Owner: ex.Org, Repo: ex.Name}) == prk {
					dup = true
					break
				}
			}
			if dup {
				continue
			}
			for _, v := range vitrines {
				if refKey(config.RepoRef{Instance: v.instance, Owner: v.owner, Repo: v.repo}) == prk {
					row.Vitrines = append(row.Vitrines, MatrixCell{Present: true, Name: v.repo, Org: v.owner, Instance: v.instance, FullName: v.fullName, LocalPath: v.localPath, Suggested: !confirmed[prk]})
					break
				}
			}
		}
		row.Project = projByAtelier[row.Key]
		// Non-projet = à traiter (compté). Alignable seulement s'il y a une contrepartie à relier : le
		// bouton « Appairer » n'a de sens que là ; un atelier seul reste compté mais sans bouton (il se
		// complète par « Créer le clone / la vitrine », ou s'archive). SOURCE UNIQUE du pending (1.18.1).
		row.Pending = row.Project == ""
		row.Alignable = row.Pending && (row.Local.Present || len(row.Vitrines) > 0)
		rows = append(rows, row)
	}
	// Orphelins : clones locaux sans atelier ni vitrine connus (dépôts pas encore reliés en ligne).
	for rem, path := range localIdx {
		if atelierRemotes[rem] || vitrineRemotes[rem] {
			continue
		}
		rows = append(rows, MatrixRow{
			Key:     "local:" + rem,
			Name:    baseName(path),
			Local:   MatrixCell{Present: true, Name: baseName(path), LocalPath: path},
			Pending: true, // clone local pas encore relié en ligne → à traiter (compléter ou archiver)
		})
	}
	// Orphelins : vitrines sans atelier de même nom ET non déjà rattachées à un atelier via un projet
	// (appairage explicite nom ≠ atelier) — sinon la vitrine appairée réapparaîtrait en double.
	for _, v := range vitrines {
		if atelierNames[strings.ToLower(v.repo)] {
			continue
		}
		if prodKeys[refKey(config.RepoRef{Instance: v.instance, Owner: v.owner, Repo: v.repo})] {
			continue
		}
		rows = append(rows, MatrixRow{
			Key:      "vit:" + refKey(config.RepoRef{Instance: v.instance, Owner: v.owner, Repo: v.repo}),
			Name:     v.repo,
			Vitrines: []MatrixCell{{Present: true, Name: v.repo, Org: v.owner, Instance: v.instance, FullName: v.fullName, LocalPath: v.localPath}},
			Pending:  true, // vitrine sans atelier → incomplète, à traiter (créer l'atelier/clone ou archiver)
		})
	}

	// Séparer les lignes archivées (rangées) des lignes vives. Une archive ne compte pas « en attente ».
	live := rows[:0]
	for _, r := range rows {
		if archived[r.Key] {
			out.Archived = append(out.Archived, r)
			continue
		}
		if r.Pending {
			out.Pending++
		}
		live = append(live, r)
	}
	rows = live

	bykey := func(a []MatrixRow) {
		sort.SliceStable(a, func(i, j int) bool {
			ri, rj := rowScore(a[i]), rowScore(a[j])
			if ri != rj {
				return ri > rj // à approuver d'abord, puis complets, puis orphelins
			}
			return strings.ToLower(a[i].Name) < strings.ToLower(a[j].Name)
		})
	}
	bykey(rows)
	bykey(out.Archived)
	out.Rows = rows
	return out, nil
}

// AddVitrineHint enregistre une suggestion manuelle atelier→vitrine (noms différents). La paire est
// alors ASSEMBLÉE (la vitrine se rattache à la ligne de l'atelier, le bouton « Appairer » apparaît),
// mais PAS confirmée : l'utilisateur garde la main (geste local, réversible, sans PIN).
func (s *Service) AddVitrineHint(atelier, vitrine config.RepoRef) error {
	if err := s.assertWritable(); err != nil {
		return err
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if !cfg.AddVitrineHint(atelier, vitrine) {
		return nil // déjà suggérée : idempotent
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}

// ArchivePairing range une suggestion d'appariement (par sa clé de ligne) : elle quitte « en attente »
// pour l'archive, sans rien supprimer. Geste de préférence local (réversible) — pas de PIN.
func (s *Service) ArchivePairing(key string) error {
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if !cfg.ArchivePairing(key) {
		return nil // déjà archivé : idempotent
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}

// UnarchivePairing repasse une suggestion de l'archive vers « en attente » d'appairage.
func (s *Service) UnarchivePairing(key string) error {
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if !cfg.UnarchivePairing(key) {
		return nil
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}

// PairingDelete SUPPRIME les dépôts d'une ligne : chaque dépôt distant présent (atelier, vitrines) est
// supprimé définitivement du serveur (DeleteRepo, sous PIN). Pour le clone local :
//   - wipeLocal = false : retiré du suivi SituLab seulement (fichiers conservés sur le disque) ;
//   - wipeLocal = true  : le DOSSIER est EFFACÉ du disque (irréversible) puis retiré du suivi.
//
// Action destructive : chaque suppression distante exige le PIN (porté par pin). L'effacement du
// dossier n'est permis que si le chemin existe et porte bien un dépôt git (présence de .git) — jamais
// un dossier arbitraire.
func (s *Service) PairingDelete(localPath string, wipeLocal bool, atelier config.RepoRef, productions []config.RepoRef, pin string) error {
	if err := s.assertWritable(); err != nil {
		return err
	}
	// ORDRE CRITIQUE : on supprime d'abord les VITRINES (dérivées, republiables par promotion depuis
	// l'atelier), puis l'ATELIER en DERNIER. L'atelier est la source de vérité (historique complet) et
	// n'est PAS reconstructible depuis une vitrine (promotion = écrasement/filtrage). Si une suppression
	// échoue en cours de route (PIN, réseau, droits), la source survit ainsi le plus longtemps possible —
	// jamais « source détruite, vitrines orphelines ».
	refs := make([]config.RepoRef, 0, 1+len(productions))
	for _, p := range productions {
		if p.Owner != "" && p.Repo != "" {
			refs = append(refs, p)
		}
	}
	if atelier.Owner != "" && atelier.Repo != "" {
		refs = append(refs, atelier)
	}
	for _, r := range refs {
		if err := s.DeleteRepo(r.Instance, r.Owner, r.Repo, pin); err != nil {
			return err
		}
	}
	localPath = strings.TrimSpace(localPath)
	if localPath == "" {
		return nil
	}
	if wipeLocal {
		if err := wipeLocalRepo(localPath); err != nil {
			return err
		}
	}
	// Retirer du suivi dans les deux cas (le dossier a disparu, ou on cesse simplement de le suivre).
	return s.RemoveLocalRepo(localPath)
}

// wipeLocalRepo efface le DOSSIER d'un dépôt git du disque — garde-fou : refuse si le chemin n'existe
// pas, n'est pas un dossier, ou ne contient pas de .git (on n'efface jamais un dossier qui n'est pas
// un dépôt). Irréversible.
func wipeLocalRepo(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // déjà disparu : rien à effacer
		}
		return err
	}
	if !fi.IsDir() {
		return errors.New("le chemin n'est pas un dossier")
	}
	if _, err := os.Stat(filepath.Join(path, ".git")); err != nil {
		return errors.New("refus : ce dossier n'est pas un dépôt git (.git introuvable)")
	}
	return os.RemoveAll(path)
}

// rowScore classe les lignes : appariements à approuver en tête, puis ateliers rangés, puis orphelins.
func rowScore(r MatrixRow) int {
	n := 0
	if r.Atelier.Present && r.Pending {
		n += 100
	}
	if r.Atelier.Present {
		n += 10
	}
	if r.Local.Present {
		n += 2
	}
	n += len(r.Vitrines)
	return n
}

// refKey : clé canonique d'un RepoRef (insensible à la casse), pour l'appariement.
func refKey(r config.RepoRef) string {
	return strings.ToLower(r.Instance + "/" + r.Owner + "/" + r.Repo)
}

// splitFull découpe « owner/repo » ; à défaut de « / », l'owner est vide et le repo prend name.
func splitFull(full, name string) (owner, repo string) {
	if i := strings.LastIndex(full, "/"); i >= 0 {
		return full[:i], full[i+1:]
	}
	return "", name
}

// baseName : dernier segment d'un chemin (nom du dossier), séparateurs \ et / confondus — quel que
// soit l'OS. On NE passe PAS par filepath.Base : sous Linux, « \ » n'y est pas un séparateur, donc un
// chemin Windows (C:\tmp\KERNEL) n'y serait pas découpé. On coupe donc sur \ ET / nous-mêmes.
func baseName(p string) string {
	p = strings.TrimRight(p, `\/`)
	if i := strings.LastIndexAny(p, `\/`); i >= 0 {
		return p[i+1:]
	}
	return p
}
