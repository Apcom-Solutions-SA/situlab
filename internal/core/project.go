// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"fmt"
	"strings"
	"time"

	"situlab/internal/config"
	"situlab/internal/gitx"
)

// --- Projets : liens atelier ↔ production (v1.7.1) ---
//
// Un projet relie UN dépôt atelier (source de vérité du dev, historique complet) à 1..N dépôts
// production (vitrines). La production n'est jamais poussée depuis le local : on y publie par
// PROMOTION (v1.7.2). Le « point d'équilibre » se lit par VERSION (tag/release) : quelle version
// d'atelier est en prod, et l'écart. L'appariement par versions (et non par commits) colle au
// modèle « micro-historique à l'atelier, versions propres en prod ».

// RepoLine décrit un dépôt d'un projet et sa version courante (dernier tag).
type RepoLine struct {
	Instance string `json:"instance"`
	Owner    string `json:"owner"`
	Repo     string `json:"repo"`
	Version  string `json:"version"` // dernier tag, ou "" si aucun
	Error    string `json:"error"`   // lecture impossible (connexion KO, droits…)
}

// ProdLine ajoute l'état d'équilibre d'une production vis-à-vis de l'atelier.
type ProdLine struct {
	RepoLine
	Balanced bool   `json:"balanced"` // même version que l'atelier
	Behind   int    `json:"behind"`   // nb de versions d'atelier plus récentes que la prod
	Promoted string `json:"promoted"` // dernière version promue par SituLab (marqueur), si connue
}

// ProjectView est l'état consolidé d'un projet pour l'affichage/agent.
type ProjectView struct {
	Name    string     `json:"name"`
	Atelier RepoLine   `json:"atelier"`
	Prods   []ProdLine `json:"productions"`
}

// Projects renvoie les projets configurés (liens atelier↔prod). Lecture, pas de PIN.
func (s *Service) Projects() []config.Project {
	s.mu.Lock()
	s.lockedReload()
	ps := append([]config.Project(nil), s.cfg.Projects...)
	s.mu.Unlock()
	return ps
}

// AddProject crée un projet (atelier obligatoire ; vitrines facultatives). Compat ancienne signature.
func (s *Service) AddProject(name string, atelier config.RepoRef, productions []config.RepoRef) error {
	return s.SaveProject("", name, "", atelier, productions)
}

// SaveProject crée OU modifie un projet. oldName vide = création ; sinon on retrouve le projet par
// son ancien nom et on peut le renommer, changer son icône, son atelier et ses vitrines. Un atelier
// reste obligatoire ; les vitrines (production) sont facultatives. Les marqueurs de promotion des
// vitrines conservées sont préservés ; ceux des vitrines retirées sont nettoyés.
func (s *Service) SaveProject(oldName, name, icon string, atelier config.RepoRef, productions []config.RepoRef) error {
	if err := s.assertWritable(); err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	oldName = strings.TrimSpace(oldName)
	if name == "" {
		return fmt.Errorf("nom de projet vide")
	}
	if atelier.Instance == "" || atelier.Owner == "" || atelier.Repo == "" {
		return fmt.Errorf("dépôt atelier incomplet (instance/owner/repo)")
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	key := oldName
	if key == "" {
		key = name
	}
	// Collision : le nom cible ne doit pas appartenir à un AUTRE projet que celui édité.
	for i := range cfg.Projects {
		if eqProjName(cfg.Projects[i].Name, name) && !eqProjName(cfg.Projects[i].Name, key) {
			return fmt.Errorf("un projet « %s » existe déjà", name)
		}
	}
	existing := cfg.FindProject(key)
	if existing != nil {
		existing.Name = name
		existing.Icon = strings.TrimSpace(icon)
		existing.Atelier = atelier
		existing.Productions = productions
		existing.PruneDanglingPromotions()
	} else {
		cfg.Projects = append(cfg.Projects, config.Project{Name: name, Icon: strings.TrimSpace(icon), Atelier: atelier, Productions: productions})
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}

func eqProjName(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

// RemoveProject retire un projet par nom (ne touche à aucun dépôt).
func (s *Service) RemoveProject(name string) error {
	if err := s.assertWritable(); err != nil {
		return err
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	out := cfg.Projects[:0]
	for _, p := range cfg.Projects {
		if !strings.EqualFold(strings.TrimSpace(p.Name), strings.TrimSpace(name)) {
			out = append(out, p)
		}
	}
	cfg.Projects = out
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}

// latestTags renvoie les tags d'un dépôt, du plus récent au plus ancien (ordre de l'API), et le
// plus récent. Best-effort : une erreur remonte via err.
func (s *Service) latestTags(ref config.RepoRef) (tags []string, latest string, err error) {
	ts, e := s.RepoTags(ref.Instance, ref.Owner, ref.Repo)
	if e != nil {
		return nil, "", e
	}
	for _, t := range ts {
		tags = append(tags, t.Name)
	}
	if len(tags) > 0 {
		latest = tags[0]
	}
	return tags, latest, nil
}

// ProjectStatus calcule l'état d'équilibre d'un projet : version de l'atelier, version de chaque
// prod, « équilibré » ou « en retard de N versions », et la dernière version promue connue.
// Best-effort : chaque dépôt illisible renvoie son erreur sans bloquer les autres.
func (s *Service) ProjectStatus(name string) (*ProjectView, error) {
	s.mu.Lock()
	s.lockedReload()
	p := s.cfg.FindProject(name)
	if p == nil {
		s.mu.Unlock()
		return nil, fmt.Errorf("projet introuvable : %q", name)
	}
	proj := *p // copie
	s.mu.Unlock()

	v := &ProjectView{Name: proj.Name}
	atelierTags, atelierLatest, aerr := s.latestTags(proj.Atelier)
	v.Atelier = RepoLine{Instance: proj.Atelier.Instance, Owner: proj.Atelier.Owner, Repo: proj.Atelier.Repo, Version: atelierLatest}
	if aerr != nil {
		v.Atelier.Error = cleanClientErr(aerr)
	}
	idx := tagIndex(atelierTags)
	for _, pr := range proj.Productions {
		v.Prods = append(v.Prods, s.prodLine(pr, idx, atelierLatest, proj.Promotions))
	}
	return v, nil
}

// tagIndex associe chaque tag d'atelier à son rang (0 = le plus récent), pour mesurer le retard d'une
// vitrine par rapport à l'atelier.
func tagIndex(atelierTags []string) map[string]int {
	idx := make(map[string]int, len(atelierTags))
	for i, t := range atelierTags {
		idx[t] = i
	}
	return idx
}

// prodLine calcule l'état d'une vitrine face à l'atelier : version, « équilibré », retard (nombre de
// versions d'atelier plus récentes) et dernière version promue connue. Lecture illisible → Error.
func (s *Service) prodLine(pr config.RepoRef, idx map[string]int, atelierLatest string, promos map[string]config.Promo) ProdLine {
	line := ProdLine{RepoLine: RepoLine{Instance: pr.Instance, Owner: pr.Owner, Repo: pr.Repo}}
	if _, pv, err := s.latestTags(pr); err != nil {
		line.Error = cleanClientErr(err)
	} else {
		line.Version = pv
		line.Balanced = pv != "" && pv == atelierLatest
		if i, ok := idx[pv]; ok {
			line.Behind = i
		}
	}
	if promos != nil {
		line.Promoted = promos[pr.Key()].Version
	}
	return line
}

// ProjectLocal : état du clone local de l'atelier (colonne « Local » de la vue 3 colonnes v1.8).
type ProjectLocal struct {
	Path     string `json:"path"`
	Branch   string `json:"branch"`
	Clean    bool   `json:"clean"`
	Ahead    int    `json:"ahead"`
	Behind   int    `json:"behind"`
	Changed  int    `json:"changed"`
	Describe string `json:"describe"`
	LastDate string `json:"last_date"`
}

// ProjectBoard : état « 3 colonnes » d'un projet — Local (ce poste) · Atelier (dev) · Production(s).
// C'est le cœur du tableau de bord Projets (v1.8) : voir d'un coup l'alignement local↔atelier↔prod.
type ProjectBoard struct {
	Name    string        `json:"name"`
	Icon    string        `json:"icon"`
	Atelier RepoLine      `json:"atelier"`     // dépôt atelier + version distante (dernier tag)
	Local   *ProjectLocal `json:"local"`       // clone local de l'atelier ; nil = pas sur ce poste
	Prods   []ProdLine    `json:"productions"` // productions + équilibre par versions
}

// ProjectBoards calcule l'état 3 colonnes de TOUS les projets. Best-effort : une connexion KO
// remonte son erreur par dépôt sans bloquer les autres. Lecture (pas de PIN).
func (s *Service) ProjectBoards() ([]ProjectBoard, error) {
	projs := s.Projects()
	out := make([]ProjectBoard, 0, len(projs))
	for _, p := range projs {
		b := ProjectBoard{Name: p.Name, Icon: p.Icon}
		// Atelier (distant) : version = dernier tag.
		atelierTags, av, aerr := s.latestTags(p.Atelier)
		b.Atelier = RepoLine{Instance: p.Atelier.Instance, Owner: p.Atelier.Owner, Repo: p.Atelier.Repo, Version: av}
		if aerr != nil {
			b.Atelier.Error = cleanClientErr(aerr)
		}
		// Clone local de l'atelier (colonne Local).
		if path := s.localCloneFor(p.Atelier); path != "" {
			if sum, err := gitx.SummaryOf(path); err == nil && sum.IsRepo {
				b.Local = &ProjectLocal{
					Path: path, Branch: sum.Branch, Clean: sum.Clean, Ahead: sum.Ahead, Behind: sum.Behind,
					Changed: len(sum.Changed), Describe: sum.Describe, LastDate: sum.LastDate,
				}
			}
		}
		// Productions : version + « équilibré » / « en retard de N versions ».
		idx := tagIndex(atelierTags)
		for _, pr := range p.Productions {
			b.Prods = append(b.Prods, s.prodLine(pr, idx, av, p.Promotions))
		}
		out = append(out, b)
	}
	return out, nil
}

// localCloneFor trouve le clone LOCAL suivi dont le distant correspond au dépôt visé (par URL,
// sensible à l'hôte). Renvoie "" si aucun.
func (s *Service) localCloneFor(ref config.RepoRef) string {
	r, err := s.ViewRepo(ref.Instance, ref.Owner, ref.Repo)
	if err != nil {
		return ""
	}
	want := normURL(r.CloneURL)
	if want == "" {
		return ""
	}
	s.mu.Lock()
	s.lockedReload()
	paths := append([]string(nil), s.cfg.LocalRepos...)
	s.mu.Unlock()
	for _, p := range paths {
		if st, err := gitx.StatusOf(p); err == nil && st.IsRepo && normURL(st.Remote) == want {
			return p
		}
	}
	return ""
}

// PromoteResult décrit, en clair, ce qu'une promotion a publié.
type PromoteResult struct {
	Project    string `json:"project"`
	Production string `json:"production"` // owner/repo de la prod
	Version    string `json:"version"`
	Branch     string `json:"branch"`
	Summary    string `json:"summary"`
}

// PromoteOptions précise CE qui est promu : l'utilisateur décide la version, la branche d'atelier
// source et la branche de production cible. Champs vides = valeurs par défaut (dernier tag de la
// branche par défaut de l'atelier → branche par défaut de la prod), pour les appels non interactifs.
type PromoteOptions struct {
	SourceBranch string `json:"source_branch"` // branche de l'atelier d'où provient la version
	Version      string `json:"version"`       // tag/ref exact publié (ex. v1.8.0) ; vide = dernier tag de la branche
	TargetBranch string `json:"target_branch"` // branche de la production où publier
}

// ProjectPromote publie une version choisie de l'atelier vers UNE production du projet, en UN commit
// propre (pas les micro-commits). L'utilisateur décide la version, la branche source et la branche
// cible (voir PromoteOptions). Geste DISTANT vers la production → PIN TOUJOURS requis (fail-closed).
// L'IA ne fait jamais cela (prod bridée). Enregistre le marqueur d'équilibre après succès.
func (s *Service) ProjectPromote(projectName string, prodIndex int, opts PromoteOptions, pin string) (*PromoteResult, error) {
	if !gitx.Available() {
		return nil, ErrNoGit
	}
	s.mu.Lock()
	s.lockedReload()
	p := s.cfg.FindProject(projectName)
	if p == nil {
		s.mu.Unlock()
		return nil, fmt.Errorf("projet introuvable : %q", projectName)
	}
	proj := *p
	s.mu.Unlock()
	if prodIndex < 0 || prodIndex >= len(proj.Productions) {
		return nil, fmt.Errorf("production #%d inexistante dans le projet %q", prodIndex, projectName)
	}
	prod := proj.Productions[prodIndex]

	// Consentement : publier vers une PRODUCTION est par définition un geste protégé — PIN TOUJOURS
	// exigé (fail-closed), indépendamment du rôle/politique de la connexion cible (qui pourrait être
	// mal classée). gatePIN couvre aussi le mode lecture seule.
	if err := s.gatePIN(pin); err != nil {
		return nil, err
	}

	// 1) Clone local de l'atelier (source de la version à publier).
	path := s.localCloneFor(proj.Atelier)
	if path == "" {
		return nil, fmt.Errorf("aucun clone local de l'atelier %s/%s suivi — clonez-le d'abord", proj.Atelier.Owner, proj.Atelier.Repo)
	}
	// Resynchroniser l'atelier AVANT de lire la version/l'arbre : on promeut la version réelle, pas
	// un clone en retard. Best-effort pour le fetch ; on refuse en revanche de promouvoir un clone
	// en retard sur son serveur (le contenu promu ne correspondrait pas à la version nommée).
	atToken, atGitHub, atBase := s.instanceToken(proj.Atelier.Instance), s.isGitHub(proj.Atelier.Instance), s.instanceBaseURL(proj.Atelier.Instance)
	_, _ = gitx.Fetch(path, atToken, atGitHub, atBase)
	st, err := gitx.StatusOf(path)
	if err != nil {
		return nil, err
	}
	if !st.Clean {
		return nil, fmt.Errorf("l'atelier a des modifications non enregistrées — enregistrez (ou mettez de côté) avant de promouvoir")
	}
	// Invariant de flux : on ne promeut JAMAIS depuis le local directement. La vitrine se nourrit de
	// l'ATELIER, pas du poste. Si le clone a des commits non encore publiés sur l'atelier, on refuse :
	// publiez d'abord l'atelier (local → atelier), puis promouvez (atelier → vitrine).
	if st.Ahead > 0 {
		return nil, fmt.Errorf("votre clone local a %d commit(s) non publiés sur l'atelier — publiez d'abord l'atelier (local → atelier), puis promouvez (atelier → vitrine)", st.Ahead)
	}

	// 2) Source & version. On publie une version TAGUÉE de l'atelier (le tag est obligatoire : il
	//    accompagnera la vitrine). La version demandée, ou le dernier tag de la branche d'atelier.
	srcBranch := strings.TrimSpace(opts.SourceBranch)
	if srcBranch == "" {
		if ar, e := s.ViewRepo(proj.Atelier.Instance, proj.Atelier.Owner, proj.Atelier.Repo); e == nil {
			srcBranch = ar.DefaultBranch
		}
	}
	version := strings.TrimSpace(opts.Version)
	var srcRef string
	switch {
	case version != "":
		srcRef = version // on publie exactement la version (tag) demandée
	case srcBranch != "":
		version = gitx.NearestTagOf(path, "origin/"+srcBranch) // dernier tag joignable sur l'atelier
		srcRef = version
	default:
		_, version, _ = s.latestTags(proj.Atelier) // dernier tag de l'atelier
		srcRef = version
	}
	if version == "" || srcRef == "" {
		return nil, fmt.Errorf("aucune version TAGUÉE à promouvoir sur « %s » — créez d'abord un tag de version (ex. v1.8.0) sur l'atelier, puis promouvez", srcBranch)
	}
	// La version doit être PUBLIÉE sur l'ATELIER (son commit est déjà sur la branche distante), pas
	// seulement en local — sinon la vitrine recevrait du contenu absent de la source de vérité.
	if srcBranch != "" && !gitx.TagPublished(path, version, "origin/"+srcBranch) {
		return nil, fmt.Errorf("la version « %s » n'est pas publiée sur l'atelier — publiez-la d'abord (local → atelier) avant de promouvoir", version)
	}

	// 3) Infos prod : URL de clone + branche par défaut + jeton de la connexion prod.
	pr, err := s.ViewRepo(prod.Instance, prod.Owner, prod.Repo)
	if err != nil {
		return nil, err
	}
	branch := strings.TrimSpace(opts.TargetBranch)
	if branch == "" {
		branch = pr.DefaultBranch
	}
	if branch == "" {
		branch = "main"
	}
	token, github, base := s.instanceToken(prod.Instance), s.isGitHub(prod.Instance), s.instanceBaseURL(prod.Instance)

	// 4) Base = état actuel de la prod (vide à la 1re promotion) ; squash propre par-dessus.
	baseSHA, ferr := gitx.FetchURL(path, pr.CloneURL, branch, token, github, base)
	if ferr != nil {
		return nil, fmt.Errorf("impossible de lire l'état de la production avant promotion : %w", ferr)
	}
	srcLabel := proj.Atelier.Owner + "/" + proj.Atelier.Repo
	if srcBranch != "" {
		srcLabel += "@" + srcBranch
	}
	msg := fmt.Sprintf("Promotion %s (depuis l'atelier %s)", version, srcLabel)
	sha, err := gitx.SquashCommit(path, srcRef, baseSHA, msg)
	if err != nil {
		return nil, err
	}

	// 5) Publier vers la prod (fast-forward sur la base récupérée ; échoue proprement si la prod a
	//    bougé entre-temps — on ne force pas la vitrine à l'aveugle).
	if _, err := gitx.PushCommitTo(path, pr.CloneURL, sha, branch, token, github, base, false); err != nil {
		return nil, fmt.Errorf("publication vers la production échouée : %w", err)
	}

	// 6) Étiqueter la version sur la vitrine (OBLIGATOIRE) : une version promue porte toujours son tag,
	//    pour que la vitrine soit identifiable et que l'équilibre se lise par version.
	if _, err := gitx.PushTagTo(path, pr.CloneURL, version, sha, token, github, base); err != nil {
		return nil, fmt.Errorf("commit publié mais étiquetage « %s » de la vitrine échoué : %w", version, err)
	}

	// 7) Marqueur d'équilibre : version promue + date.
	s.recordPromotion(projectName, prod.Key(), version)

	res := &PromoteResult{
		Project: projectName, Production: prod.Owner + "/" + prod.Repo, Version: version, Branch: branch,
		Summary: fmt.Sprintf("Version %s (%s) publiée et étiquetée sur la production %s/%s → %s.", version, srcLabel, prod.Owner, prod.Repo, branch),
	}
	return res, nil
}

// BranchTags liste les versions (tags) joignables depuis une branche d'un clone local. On lit la
// branche distante (origin/<branche>) après un fetch best-effort, pour refléter le serveur.
func (s *Service) BranchTags(path, branch string) ([]string, error) {
	if !gitx.Available() {
		return nil, ErrNoGit
	}
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return nil, fmt.Errorf("branche manquante")
	}
	if t := gitx.TagsFor(path, "origin/"+branch); t != nil {
		return t, nil
	}
	return gitx.TagsFor(path, branch), nil // repli : branche locale
}

// recordPromotion enregistre le dernier état promu vers une production (clé RepoRef.Key()).
func (s *Service) recordPromotion(projectName, prodKey, version string) {
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return
	}
	pr := cfg.FindProject(projectName)
	if pr == nil {
		return
	}
	if pr.Promotions == nil {
		pr.Promotions = map[string]config.Promo{}
	}
	pr.Promotions[prodKey] = config.Promo{Version: version, Date: time.Now().Format(time.RFC3339)}
	if err := cfg.Save(); err == nil {
		s.publish(cfg)
	}
}

// cleanClientErr raccourcit une erreur de client pour l'affichage (évite les URLs à rallonge).
func cleanClientErr(err error) string {
	m := err.Error()
	if len(m) > 160 {
		m = m[:160] + "…"
	}
	return m
}
