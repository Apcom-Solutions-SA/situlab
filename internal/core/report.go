// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"situlab/internal/config"
	"situlab/internal/forgejo"
)

// --- Analyse d'équipe (v1.5.0) : qui a produit quoi, où, quand, combien ---

// AuthorStat agrège l'activité d'une personne sur la période.
type AuthorStat struct {
	Login     string `json:"login"` // compte Forgejo (si mappé)
	Name      string `json:"name"`  // nom inscrit dans les commits
	Commits   int    `json:"commits"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Repos     int    `json:"repos"` // nombre de dépôts touchés

	reposSet map[string]bool
}

// RepoStat agrège l'activité sur un dépôt.
type RepoStat struct {
	FullName  string `json:"full_name"`
	Commits   int    `json:"commits"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
}

// TeamReport est un rapport d'analyse, sauvegardé et rejouable (« état au… »).
type TeamReport struct {
	ID        string         `json:"id"`         // identifiant/clé fichier (horodatage)
	CreatedAt string         `json:"created_at"` // date de génération (ISO)
	Instance  string         `json:"instance"`
	Since     string         `json:"since"`
	Until     string         `json:"until"`
	Scope     []string       `json:"scope"` // logins ciblés (vide = équipe entière)
	Authors   []AuthorStat   `json:"authors"`
	Repos     []RepoStat     `json:"repos"`
	ByHour    []int          `json:"by_hour"` // 24 cases : commits par heure locale (0-23)
	ByDate    map[string]int `json:"by_date"` // "AAAA-MM-JJ" -> commits (courbe d'activité)
	Totals    struct {
		Commits   int `json:"commits"`
		Additions int `json:"additions"`
		Deletions int `json:"deletions"`
		Authors   int `json:"authors"`
		Repos     int `json:"repos"`
	} `json:"totals"`
	ReposScanned int  `json:"repos_scanned"`
	ReposFailed  int  `json:"repos_failed,omitempty"` // dépôts ignorés (inaccessibles) : chiffres sous-estimés
	Partial      bool `json:"partial,omitempty"`      // chiffres INCOMPLETS : pagination tronquée OU dépôt(s) ignoré(s)
}

// reportsDir est le dossier où les rapports sont enregistrés.
func reportsDir() string { return filepath.Join(config.Dir(), "reports") }

// AnalyzeTeam parcourt les dépôts de l'instance, agrège l'activité (commits, lignes +/-, horaires)
// sur [since, until], filtrée aux logins donnés (vide = tout le monde), et renvoie un rapport
// SAUVEGARDÉ. progress (optionnel) est appelé à chaque dépôt pour l'attente côté interface.
// Lecture seule côté serveur (aucune mutation). since/until au format ISO 8601.
func (s *Service) AnalyzeTeam(instance string, logins []string, since, until string, progress func(done, total int, label string)) (*TeamReport, error) {
	cl, inst, err := s.clientFor(instance)
	if err != nil {
		return nil, err
	}
	// Réservé à Forgejo/Gitea : l'API GitHub ne renvoie pas les volumes de lignes par commit sur la
	// liste des commits (ils seraient tous à zéro) — on refuse plutôt que de produire des chiffres faux.
	if s.isGitHub(instance) {
		return nil, fmt.Errorf("l'analyse d'équipe n'est disponible que sur une connexion Forgejo/Gitea")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	repos, _, err := s.cachedRepos(ctx, inst.Name, cl)
	if err != nil {
		return nil, err
	}

	// Ensemble des logins ciblés (insensible à la casse) ; vide = aucun filtre.
	want := map[string]bool{}
	for _, l := range logins {
		if l = strings.ToLower(strings.TrimSpace(l)); l != "" {
			want[l] = true
		}
	}
	inScope := func(cs forgejo.CommitStat) bool {
		if len(want) == 0 {
			return true
		}
		return want[strings.ToLower(cs.AuthorUser)] || want[strings.ToLower(cs.AuthorName)]
	}

	rep := &TeamReport{
		ID: time.Now().Format("20060102-150405"), CreatedAt: time.Now().Format(time.RFC3339),
		Instance: inst.Name, Since: since, Until: until, Scope: logins,
		ByHour: make([]int, 24), ByDate: map[string]int{},
	}
	authors := map[string]*AuthorStat{} // clé = login si présent, sinon "name:"+nom
	repoStats := map[string]*RepoStat{}

	for i, r := range repos {
		if progress != nil {
			progress(i, len(repos), r.FullName)
		}
		commits, partial, err := cl.ListCommitsRange(ctx, repoOwner(r.FullName), r.Name, "", since, until)
		if err != nil {
			// Un dépôt inaccessible ne fait pas échouer l'ensemble, mais on le COMPTE et on marque le
			// rapport comme incomplet : sinon des chiffres sous-estimés passeraient pour exhaustifs.
			rep.ReposFailed++
			rep.Partial = true
			continue
		}
		if partial {
			rep.Partial = true // ce dépôt dépassait la borne de pagination : chiffres incomplets
		}
		rep.ReposScanned++
		for _, cm := range commits {
			if !inScope(cm) {
				continue
			}
			// Auteur
			key := strings.ToLower(cm.AuthorUser)
			if key == "" {
				key = "name:" + strings.ToLower(cm.AuthorName)
			}
			a := authors[key]
			if a == nil {
				a = &AuthorStat{Login: cm.AuthorUser, Name: cm.AuthorName, reposSet: map[string]bool{}}
				authors[key] = a
			}
			a.Commits++
			a.Additions += cm.Additions
			a.Deletions += cm.Deletions
			a.reposSet[r.FullName] = true
			// Dépôt
			rs := repoStats[r.FullName]
			if rs == nil {
				rs = &RepoStat{FullName: r.FullName}
				repoStats[r.FullName] = rs
			}
			rs.Commits++
			rs.Additions += cm.Additions
			rs.Deletions += cm.Deletions
			// Totaux + horaires + courbe
			rep.Totals.Commits++
			rep.Totals.Additions += cm.Additions
			rep.Totals.Deletions += cm.Deletions
			if t, perr := time.Parse(time.RFC3339, cm.Date); perr == nil {
				rep.ByHour[t.Hour()]++
				rep.ByDate[t.Format("2006-01-02")]++
			}
		}
	}
	if progress != nil {
		progress(len(repos), len(repos), "")
	}

	for _, a := range authors {
		a.Repos = len(a.reposSet)
		rep.Authors = append(rep.Authors, *a)
	}
	for _, rs := range repoStats {
		rep.Repos = append(rep.Repos, *rs)
	}
	// Tri décroissant par commits (le plus actif en tête).
	sort.Slice(rep.Authors, func(i, j int) bool { return rep.Authors[i].Commits > rep.Authors[j].Commits })
	sort.Slice(rep.Repos, func(i, j int) bool { return rep.Repos[i].Commits > rep.Repos[j].Commits })
	rep.Totals.Authors = len(rep.Authors)
	rep.Totals.Repos = len(rep.Repos)

	if err := s.saveReport(rep); err != nil {
		return nil, err
	}
	return rep, nil
}

func repoOwner(full string) string {
	if i := strings.IndexByte(full, '/'); i >= 0 {
		return full[:i]
	}
	return full
}

// saveReport écrit le rapport en JSON sous reports/<id>.json.
func (s *Service) saveReport(rep *TeamReport) error {
	dir := reportsDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, rep.ID+".json"), b, 0o600)
}

// TeamReportMeta est l'entrée d'historique (sans le détail), pour la liste des rapports.
type TeamReportMeta struct {
	ID        string `json:"id"`
	CreatedAt string `json:"created_at"`
	Since     string `json:"since"`
	Until     string `json:"until"`
	Commits   int    `json:"commits"`
	Authors   int    `json:"authors"`
}

// ListTeamReports renvoie l'historique des rapports, du plus récent au plus ancien.
func (s *Service) ListTeamReports() ([]TeamReportMeta, error) {
	ents, err := os.ReadDir(reportsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return []TeamReportMeta{}, nil
		}
		return nil, err
	}
	var out []TeamReportMeta
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		rep, err := s.LoadTeamReport(strings.TrimSuffix(e.Name(), ".json"))
		if err != nil {
			continue
		}
		out = append(out, TeamReportMeta{ID: rep.ID, CreatedAt: rep.CreatedAt, Since: rep.Since,
			Until: rep.Until, Commits: rep.Totals.Commits, Authors: rep.Totals.Authors})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out, nil
}

// LoadTeamReport relit un rapport enregistré par son identifiant.
func (s *Service) LoadTeamReport(id string) (*TeamReport, error) {
	// id = horodatage sûr (chiffres + tiret) ; on refuse tout séparateur de chemin.
	if id == "" || strings.ContainsAny(id, "/\\.") {
		return nil, os.ErrNotExist
	}
	b, err := os.ReadFile(filepath.Join(reportsDir(), id+".json"))
	if err != nil {
		return nil, err
	}
	var rep TeamReport
	if err := json.Unmarshal(b, &rep); err != nil {
		return nil, err
	}
	return &rep, nil
}
