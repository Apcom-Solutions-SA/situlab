// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"fmt"
	"strings"
	"sync"
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
	cfg.ClearVitrineHintsForAtelier(atelier) // la paire est confirmée : plus de suggestion fantôme
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}

func eqProjName(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

// ConfirmVitrine confirme une vitrine SUGGÉRÉE (match par nom ou VitrineHint) dans le projet de son
// atelier (#16, 1.19). Si un projet existe déjà pour cet atelier, on y AJOUTE la vitrine sans toucher
// au reste (nom, icône, tags, autres vitrines) ; sinon on crée le projet. Dans les deux cas la
// suggestion fantôme (hint) est effacée. Geste FENÊTRE, local, sans PIN.
func (s *Service) ConfirmVitrine(atelier, vitrine config.RepoRef) error {
	if err := s.assertWritable(); err != nil {
		return err
	}
	if atelier.Instance == "" || atelier.Owner == "" || atelier.Repo == "" {
		return fmt.Errorf("atelier incomplet (instance/owner/repo)")
	}
	if vitrine.Instance == "" || vitrine.Owner == "" || vitrine.Repo == "" {
		return fmt.Errorf("vitrine incomplète (instance/owner/repo)")
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ak, vk := refKey(atelier), refKey(vitrine)
	var proj *config.Project
	for i := range cfg.Projects {
		if refKey(cfg.Projects[i].Atelier) == ak {
			proj = &cfg.Projects[i]
			break
		}
	}
	if proj != nil {
		already := false
		for _, pr := range proj.Productions {
			if refKey(pr) == vk {
				already = true
				break
			}
		}
		if !already {
			proj.Productions = append(proj.Productions, vitrine)
		}
	} else {
		cfg.Projects = append(cfg.Projects, config.Project{Name: atelier.Repo, Atelier: atelier, Productions: []config.RepoRef{vitrine}})
	}
	cfg.ClearVitrineHintsForAtelier(atelier) // la paire est confirmée : plus de suggestion fantôme
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}

// SetProjectTag classe un projet dans un « tag projet » (groupe libre). tag vide = non classé. Ne
// touche à aucun dépôt — c'est un rangement de l'affichage.
func (s *Service) SetProjectTag(name, tag string) error {
	if err := s.assertWritable(); err != nil {
		return err
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	found := false
	for i := range cfg.Projects {
		if eqProjName(cfg.Projects[i].Name, name) {
			cfg.Projects[i].Tag = strings.TrimSpace(tag)
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("projet introuvable : %q", name)
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}

// normTags : trim, retrait des vides, déduplication en préservant l'ordre.
func normTags(tags []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t == "" || seen[strings.ToLower(t)] {
			continue
		}
		seen[strings.ToLower(t)] = true
		out = append(out, t)
	}
	return out
}

// SetProjectTags fixe les tags (1..N) d'un projet. Rangement d'affichage, ne touche à aucun dépôt.
// Geste FENÊTRE (assertWritable, sans PIN, jamais exposé au démon/CLI/MCP).
func (s *Service) SetProjectTags(name string, tags []string) error {
	if err := s.assertWritable(); err != nil {
		return err
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	found := false
	for i := range cfg.Projects {
		if eqProjName(cfg.Projects[i].Name, name) {
			cfg.Projects[i].Tags = normTags(tags)
			cfg.Projects[i].Tag = "" // on bascule sur le multi-tag, l'ancien mono-tag ne vit plus
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("projet introuvable : %q", name)
	}
	// Enregistre d'office les tags inconnus dans le registre (couleur par défaut).
	for _, t := range normTags(tags) {
		cfg.UpsertTagDef(t, "")
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}

// CreateProjectTag enregistre un tag au registre (avec une couleur de palette), sans l'assigner.
func (s *Service) CreateProjectTag(name, color string) error {
	return s.tagRegistryEdit(func(cfg *config.Config) error {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("nom de tag vide")
		}
		cfg.UpsertTagDef(name, color)
		return nil
	})
}

// SetProjectTagColor change la couleur (clé de palette) d'un tag du registre.
func (s *Service) SetProjectTagColor(name, color string) error {
	return s.tagRegistryEdit(func(cfg *config.Config) error {
		cfg.UpsertTagDef(name, color)
		return nil
	})
}

// RenameProjectTag renomme un tag partout (registre + tous les projets), fusionnant en cas de collision.
func (s *Service) RenameProjectTag(oldName, newName string) error {
	return s.tagRegistryEdit(func(cfg *config.Config) error {
		if strings.TrimSpace(newName) == "" {
			return fmt.Errorf("nouveau nom de tag vide")
		}
		cfg.RenameTag(oldName, newName)
		return nil
	})
}

// DeleteProjectTag retire un tag du registre et de tous les projets.
func (s *Service) DeleteProjectTag(name string) error {
	return s.tagRegistryEdit(func(cfg *config.Config) error {
		cfg.DeleteTag(name)
		return nil
	})
}

// TagDefs renvoie le registre des tags (nom + couleur). Lecture.
func (s *Service) TagDefs() []config.TagDef {
	s.mu.Lock()
	cfg := s.cfg
	s.mu.Unlock()
	return append([]config.TagDef(nil), cfg.TagDefs...)
}

func (s *Service) tagRegistryEdit(fn func(*config.Config) error) error {
	if err := s.assertWritable(); err != nil {
		return err
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := fn(cfg); err != nil {
		return err
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
}

// SetProjectCorpus définit les fichiers/dossiers CORPUS reliés à un projet (mapping, v1.18.2). Geste
// de désignation : FENÊTRE uniquement (assertWritable, sans PIN, jamais exposé au démon/CLI/MCP). Ne
// crée AUCUN droit (le mapping est un pointeur ; l'accès IA reste décidé par les droits du cloud).
func (s *Service) SetProjectCorpus(name string, links []config.CorpusLink) error {
	if err := s.assertWritable(); err != nil {
		return err
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	clean := make([]config.CorpusLink, 0, len(links))
	for _, l := range links {
		cloud := strings.TrimSpace(l.Cloud)
		if cloud == "" || cfg.ResolveCloud(cloud) == nil {
			continue // ignore les clouds inexistants (best-effort)
		}
		seen := map[string]bool{}
		paths := make([]string, 0, len(l.Paths))
		for _, p := range l.Paths {
			cp := config.CloudCleanPath(p)
			if seen[cp] {
				continue
			}
			seen[cp] = true
			paths = append(paths, cp)
		}
		if len(paths) == 0 {
			continue
		}
		clean = append(clean, config.CorpusLink{Cloud: cloud, Paths: paths})
	}
	found := false
	for i := range cfg.Projects {
		if eqProjName(cfg.Projects[i].Name, name) {
			cfg.Projects[i].Corpus = clean
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("projet introuvable : %q", name)
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	s.publish(cfg)
	return nil
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

// latestTag ne lit QUE le tag le plus récent d'un dépôt, sans paginer tout l'historique des versions
// (forgejo.LatestTag). À réserver aux cas où seul le dernier tag compte — vitrines, barre de version —
// par opposition à latestTags qui ramène la liste complète (nécessaire à l'atelier pour l'index de
// retard). Best-effort : une erreur remonte via err (perf 1.17, Point 2).
func (s *Service) latestTag(ref config.RepoRef) (string, error) {
	cl, _, err := s.clientFor(ref.Instance)
	if err != nil {
		return "", err
	}
	ctx, cancel := opCtx()
	defer cancel()
	t, ok, err := cl.LatestTag(ctx, ref.Owner, ref.Repo)
	if err != nil || !ok {
		return "", err
	}
	return t.Name, nil
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
	// Vitrine : seul son dernier tag compte (comparé à l'index de l'atelier) — on ne pagine donc pas
	// tout son historique de versions (latestTag, pas latestTags).
	if pv, err := s.latestTag(pr); err != nil {
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
	Name        string        `json:"name"`
	Icon        string        `json:"icon"`
	Atelier     RepoLine      `json:"atelier"`      // dépôt atelier + version distante (dernier tag)
	AtelierTags []string      `json:"atelier_tags"` // tags de l'atelier (du + récent au + ancien) : évite au front de les re-demander (dé-doublonnage du graphe)
	Local       *ProjectLocal `json:"local"`        // clone local de l'atelier ; nil = pas sur ce poste
	Prods       []ProdLine    `json:"productions"`  // productions + équilibre par versions
}

// ProjectBoards calcule l'état 3 colonnes de TOUS les projets. Best-effort : une connexion KO
// remonte son erreur par dépôt sans bloquer les autres. Lecture (pas de PIN).
func (s *Service) ProjectBoards() ([]ProjectBoard, error) {
	projs := s.Projects()
	out := make([]ProjectBoard, len(projs))
	// Perf (1.9.3) : chaque projet fait plusieurs appels API (tags atelier + prods) + une lecture du
	// clone local ; on les calcule EN PARALLÈLE (chaque goroutine écrit SON index → pas de course ;
	// les caches partagés sont protégés par s.mu). Concurrence bornée pour ne pas saturer le serveur.
	// Perf (1.9.7) : on scanne les clones locaux UNE SEULE FOIS (index remote→chemin) au lieu de
	// re-parcourir TOUS les dépôts suivis pour CHAQUE projet — l'ancienne boucle O(projets × dépôts)
	// de gitx.StatusOf était l'amplificateur principal de la lenteur de la page Projets.
	localIdx := s.localRemoteIndex()
	var wg sync.WaitGroup
	sem := make(chan struct{}, 6)
	for i := range projs {
		wg.Add(1)
		go func(i int, p config.Project) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			out[i] = s.boardFor(p, localIdx)
		}(i, projs[i])
	}
	wg.Wait()
	return out, nil
}

// boardFor calcule l'état « 3 colonnes » d'UN projet (atelier + clone local + vitrines). Appel RÉSEAU
// (tags d'atelier et de chaque vitrine, résolution du clone via ViewRepo). Source unique partagée par
// ProjectBoards (tous les projets, en parallèle) et ProjectBoardOne (un seul, au clic sur un classeur).
func (s *Service) boardFor(p config.Project, localIdx map[string]string) ProjectBoard {
	b := ProjectBoard{Name: p.Name, Icon: p.Icon}

	// Perf (1.16.4) : les tags de l'atelier et de CHAQUE vitrine sont des appels réseau INDÉPENDANTS.
	// On les récupère en parallèle au lieu d'enchaîner atelier puis vitrines en file (c'était une cause
	// directe de la lenteur d'ouverture d'un projet, surtout avec une vitrine GitHub lente). La lecture
	// du clone local (disque) se fait pendant que le réseau tourne. Chaque goroutine écrit SA case (pas
	// de course) ; les caches partagés restent protégés par s.mu — même modèle que ProjectBoards.
	type tagRes struct {
		tags   []string
		latest string
		err    error
	}
	atCh := make(chan tagRes, 1)
	go func() { t, l, e := s.latestTags(p.Atelier); atCh <- tagRes{t, l, e} }()
	prodCh := make([]chan tagRes, len(p.Productions))
	for i, pr := range p.Productions {
		prodCh[i] = make(chan tagRes, 1)
		// Vitrine : seul son dernier tag sert (comparé à l'index de l'atelier) — latestTag, sans paginer
		// tout son historique (le GitHub lent d'une vitrine était une cause directe de lenteur).
		go func(i int, pr config.RepoRef) { l, e := s.latestTag(pr); prodCh[i] <- tagRes{latest: l, err: e} }(i, pr)
	}

	// Clone local (lecture disque) en parallèle du réseau.
	if path := s.localCloneForIdx(p.Atelier, localIdx); path != "" {
		if sum, err := gitx.SummaryOf(path); err == nil && sum.IsRepo {
			b.Local = &ProjectLocal{
				Path: path, Branch: sum.Branch, Clean: sum.Clean, Ahead: sum.Ahead, Behind: sum.Behind,
				Changed: len(sum.Changed), Describe: sum.Describe, LastDate: sum.LastDate,
			}
		}
	}

	at := <-atCh
	b.Atelier = RepoLine{Instance: p.Atelier.Instance, Owner: p.Atelier.Owner, Repo: p.Atelier.Repo, Version: at.latest}
	b.AtelierTags = at.tags
	if at.err != nil {
		b.Atelier.Error = cleanClientErr(at.err)
	}
	idx := tagIndex(at.tags)
	for i, pr := range p.Productions {
		r := <-prodCh[i]
		line := ProdLine{RepoLine: RepoLine{Instance: pr.Instance, Owner: pr.Owner, Repo: pr.Repo}}
		if r.err != nil {
			line.Error = cleanClientErr(r.err)
		} else {
			line.Version = r.latest
			line.Balanced = r.latest != "" && r.latest == at.latest
			if j, ok := idx[r.latest]; ok {
				line.Behind = j
			}
		}
		if p.Promotions != nil {
			line.Promoted = p.Promotions[pr.Key()].Version
		}
		b.Prods = append(b.Prods, line)
	}
	return b
}

// ProjectBoardOne calcule l'état « 3 colonnes » d'UN SEUL projet (au clic sur son classeur), sans
// parcourir les autres. Réseau pour ce projet uniquement. Erreur si le projet est introuvable.
func (s *Service) ProjectBoardOne(name string) (*ProjectBoard, error) {
	s.mu.Lock()
	s.lockedReload()
	p := s.cfg.FindProject(name)
	if p == nil {
		s.mu.Unlock()
		return nil, fmt.Errorf("projet introuvable : %q", name)
	}
	proj := *p
	s.mu.Unlock()
	b := s.boardFor(proj, s.localRemoteIndex())
	return &b, nil
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

// localRemoteIndex scanne UNE SEULE FOIS tous les clones locaux suivis et renvoie un index
// remote(normalisé) → chemin. Il remplace, dans ProjectBoards, la boucle O(projets × dépôts) de
// gitx.StatusOf par un seul balayage O(dépôts). Le premier clone trouvé pour un remote gagne
// (ordre de cfg.LocalRepos — même choix que localCloneFor).
func (s *Service) localRemoteIndex() map[string]string {
	s.mu.Lock()
	s.lockedReload()
	paths := append([]string(nil), s.cfg.LocalRepos...)
	s.mu.Unlock()
	idx := make(map[string]string, len(paths))
	for _, p := range paths {
		st, err := gitx.StatusOf(p)
		if err != nil || !st.IsRepo {
			continue
		}
		k := normURL(st.Remote)
		if k == "" {
			continue
		}
		if _, exists := idx[k]; !exists {
			idx[k] = p
		}
	}
	return idx
}

// localCloneForIdx est localCloneFor appuyé sur un index pré-calculé (évite de re-balayer les dépôts
// locaux par projet). La résolution du remote du dépôt visé reste via ViewRepo (CloneURL faisant foi).
func (s *Service) localCloneForIdx(ref config.RepoRef, idx map[string]string) string {
	r, err := s.ViewRepo(ref.Instance, ref.Owner, ref.Repo)
	if err != nil {
		return ""
	}
	want := normURL(r.CloneURL)
	if want == "" {
		return ""
	}
	return idx[want]
}

// ProjectLightView : vue d'ACCUEIL légère d'un projet — AUCUN appel réseau. Identité + topologie
// (nombre de vitrines) + état du clone local (git status : lecture disque instantanée). La version
// d'atelier et l'alignement des vitrines (qui exigent le réseau) sont réservés au DÉTAIL du projet.
// C'est le socle de la page d'accueil « classeurs » (1.9.8) : instantanée, sans la lenteur réseau par carte.
type ProjectLightView struct {
	Name             string            `json:"name"`
	Icon             string            `json:"icon"`
	Atelier          RepoLine          `json:"atelier"`              // instance/owner/repo ; Version volontairement vide (réseau = détail)
	Vitrines         int               `json:"vitrines"`             // nombre de productions configurées
	Local            *ProjectLocal     `json:"local"`                // état du clone local (nil = pas sur ce poste)
	Kernel           bool              `json:"kernel"`               // ce dépôt est désigné comme KERNEL (mémoire) sur ce poste
	Spoke            bool              `json:"spoke"`                // ce dépôt est désigné comme SPOKE (projection de mission)
	Tag              string            `json:"tag"`                  // LEGACY : premier tag (compat lecture) ; "" = non classé
	Tags             []string          `json:"tags"`                 // tags multiples (v1.18.2)
	TagColors        map[string]string `json:"tag_colors,omitempty"` // clé de couleur par tag (v1.18.2)
	KernelRun        string            `json:"kernel_run"`           // fichier d'ouverture du KERNEL (si désigné), chemin relatif
	KernelSpokesFile string            `json:"kernel_spokes_file"`   // fichier de gestion des spokes du KERNEL (procédure de consolidation), chemin relatif
	SpokeRun         string            `json:"spoke_run"`            // fichier run du spoke (si désigné), chemin relatif
	Corpus           []CorpusLinkView  `json:"corpus,omitempty"`     // mapping CORPUS↔dépôt (v1.18.2), chemins filtrés à la frontière agent
}

// CorpusPathView : un chemin relié d'un cloud, avec l'état d'accès IA DÉRIVÉ (jamais stocké).
type CorpusPathView struct {
	Path string `json:"path"`
	AI   string `json:"ai"` // actif | partiel | inactif (dérivé du périmètre autorisé du cloud)
}

// CorpusLinkView : un cloud relié à un projet et ses chemins (vue de ProjectsLight).
type CorpusLinkView struct {
	Cloud string           `json:"cloud"`
	Kind  string           `json:"kind,omitempty"` // "" = cloud déconnecté/supprimé
	Paths []CorpusPathView `json:"paths,omitempty"`
}

// ProjectsLight renvoie l'état d'accueil de TOUS les projets SANS réseau : config + résolution hors-ligne
// du clone local (index remote→chemin + git status). Conçu pour une page d'accueil instantanée — la
// lenteur venait des appels réseau par carte (tags, branches distantes) ; ils partent désormais au clic.
func (s *Service) ProjectsLight() []ProjectLightView {
	projs := s.Projects()
	s.mu.Lock()
	s.lockedReload()
	paths := append([]string(nil), s.cfg.LocalRepos...)
	// Chemins locaux désignés comme KERNEL (pour marquer les classeurs correspondants).
	kernelPaths := make(map[string]bool, len(s.cfg.Kernels))
	kernelRun := make(map[string]string, len(s.cfg.Kernels))
	kernelSpokesFile := make(map[string]string, len(s.cfg.Kernels))
	for _, k := range s.cfg.Kernels {
		cp := config.CleanPath(k.LocalPath)
		kernelPaths[cp] = true
		if k.RunFile != "" {
			kernelRun[cp] = k.RunFile
		}
		if k.SpokesFile != "" {
			kernelSpokesFile[cp] = k.SpokesFile
		}
	}
	spokePaths := make(map[string]bool, len(s.cfg.Spokes))
	spokeRun := make(map[string]string, len(s.cfg.Spokes))
	for _, sp := range s.cfg.Spokes {
		cp := config.CleanPath(sp.LocalPath)
		spokePaths[cp] = true
		if sp.RunFile != "" {
			spokeRun[cp] = sp.RunFile
		}
	}
	tagColor := make(map[string]string, len(s.cfg.TagDefs))
	for _, t := range s.cfg.TagDefs {
		tagColor[strings.ToLower(t.Name)] = t.Color
	}
	cloudsSnap := append([]config.FileCloud(nil), s.cfg.FileClouds...)
	s.mu.Unlock()
	// UN SEUL balayage local : SummaryOf porte déjà le remote ET l'état complet, donc on bâtit l'index
	// remote→chemin et l'état du clone en une passe (plus de double `git status` index + SummaryOf).
	// Balayage local PARALLÈLE (v1.18.2) : un `git status` par dépôt menés de front (borné), au lieu
	// d'en série — la page « Dépôts git » passait plusieurs secondes sur une quinzaine de dépôts, elle
	// tient désormais dans la durée d'un seul balayage. On collecte dans l'ordre d'origine pour garder
	// la déduplication déterministe (premier chemin gagnant par remote).
	summaries := make([]*gitx.Sum, len(paths))
	if len(paths) > 0 {
		workers := 8
		if len(paths) < workers {
			workers = len(paths)
		}
		sem := make(chan struct{}, workers)
		var wg sync.WaitGroup
		for i, p := range paths {
			wg.Add(1)
			sem <- struct{}{}
			go func(i int, p string) {
				defer wg.Done()
				defer func() { <-sem }()
				if s, err := gitx.SummaryOf(p); err == nil && s.IsRepo {
					summaries[i] = s
				}
			}(i, p)
		}
		wg.Wait()
	}
	idx := make(map[string]string, len(paths))
	byPath := make(map[string]*ProjectLocal, len(paths))
	for i, p := range paths {
		sum := summaries[i]
		if sum == nil {
			continue
		}
		k := normURL(sum.Remote)
		if k == "" {
			continue
		}
		if _, exists := idx[k]; exists {
			continue
		}
		idx[k] = p
		byPath[p] = &ProjectLocal{
			Path: p, Branch: sum.Branch, Clean: sum.Clean, Ahead: sum.Ahead, Behind: sum.Behind,
			Changed: len(sum.Changed), Describe: sum.Describe, LastDate: sum.LastDate,
		}
	}
	out := make([]ProjectLightView, 0, len(projs))
	for _, p := range projs {
		tags := p.TagList()
		firstTag := ""
		if len(tags) > 0 {
			firstTag = tags[0]
		}
		v := ProjectLightView{
			Name:     p.Name,
			Icon:     p.Icon,
			Tag:      firstTag,
			Tags:     tags,
			Atelier:  RepoLine{Instance: p.Atelier.Instance, Owner: p.Atelier.Owner, Repo: p.Atelier.Repo},
			Vitrines: len(p.Productions),
		}
		if colors := tagColorsFor(tags, tagColor); len(colors) > 0 {
			v.TagColors = colors
		}
		v.Corpus = buildCorpusView(p.Corpus, cloudsSnap)
		if path := s.localCloneForRefOffline(p.Atelier, idx); path != "" {
			v.Local = byPath[path]
			cp := config.CleanPath(path)
			if kernelPaths[cp] {
				v.Kernel = true
				v.KernelRun = kernelRun[cp]
				v.KernelSpokesFile = kernelSpokesFile[cp]
			}
			if spokePaths[cp] {
				v.Spoke = true
				v.SpokeRun = spokeRun[cp]
			}
		}
		out = append(out, v)
	}
	return out
}

// tagColorsFor renvoie la map tag→clé de couleur (seulement les tags qui ont une couleur).
func tagColorsFor(tags []string, byLower map[string]string) map[string]string {
	out := map[string]string{}
	for _, t := range tags {
		if c := byLower[strings.ToLower(t)]; c != "" {
			out[t] = c
		}
	}
	return out
}

// buildCorpusView construit la vue du mapping d'un projet : par cloud relié, les chemins + l'état
// d'accès IA DÉRIVÉ (on/partial/off). 100% hors-réseau. Un cloud supprimé → Kind vide (déconnecté).
func buildCorpusView(links []config.CorpusLink, clouds []config.FileCloud) []CorpusLinkView {
	if len(links) == 0 {
		return nil
	}
	byName := make(map[string]*config.FileCloud, len(clouds))
	for i := range clouds {
		byName[strings.ToLower(clouds[i].Name)] = &clouds[i]
	}
	out := make([]CorpusLinkView, 0, len(links))
	for _, l := range links {
		fc := byName[strings.ToLower(l.Cloud)]
		lv := CorpusLinkView{Cloud: l.Cloud}
		if fc != nil {
			lv.Kind = fc.Kind
		}
		for _, p := range l.Paths {
			ai := "inactif"
			if fc != nil {
				if fc.PathAllowed(p) {
					ai = "actif"
				} else if anyChildAllowed(fc, p) {
					ai = "partiel"
				}
			}
			lv.Paths = append(lv.Paths, CorpusPathView{Path: p, AI: ai})
		}
		out = append(out, lv)
	}
	return out
}

// anyChildAllowed : un sous-chemin de parent est-il explicitement autorisé à l'IA ? (pour « partial »)
func anyChildAllowed(fc *config.FileCloud, parent string) bool {
	np := config.CloudCleanPath(parent)
	for _, d := range fc.AllowedDirs {
		nd := config.CloudCleanPath(d)
		if strings.HasPrefix(nd, np+"/") {
			return true
		}
	}
	return false
}

// localCloneForRefOffline résout le clone local d'un dépôt SANS réseau : il construit l'URL attendue à
// partir de l'URL d'instance (config, pas d'appel ViewRepo) et la cherche dans l'index ; à défaut, il
// retombe sur un appariement par suffixe owner/repo (insensible à la casse). "" si aucun clone.
func (s *Service) localCloneForRefOffline(ref config.RepoRef, idx map[string]string) string {
	if base := s.instanceBaseURL(ref.Instance); base != "" {
		// Instance connue : correspondance EXACTE d'hôte (comme le résolveur réseau faisant foi). Pas de
		// repli par suffixe — il rattacherait à tort un clone d'un AUTRE hôte de même owner/repo.
		return idx[normURL(strings.TrimRight(base, "/")+"/"+ref.Owner+"/"+ref.Repo)]
	}
	// Instance inconnue (non configurée) : repli best-effort par suffixe owner/repo.
	suffix := "/" + strings.ToLower(ref.Owner) + "/" + strings.ToLower(ref.Repo)
	for k, p := range idx {
		if strings.HasSuffix(k, suffix) {
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

// Modes de promotion (v1.9.2) — l'utilisateur choisit AU CAS PAR CAS la méthode. Clarifie la confusion
// « promouvoir ≠ publier une Release » : pousser du code (± un tag) sur la vitrine ne crée PAS, à lui
// seul, une Release GitHub ni ses installeurs. La Release et les binaires sont produits par la CI,
// déclenchée par le PUSH DU TAG (`release.yml`). Trois méthodes :
const (
	// PromoteModeCode : met à jour la branche de la vitrine SANS tag — aucune nouvelle version publiée
	// (utile pour aligner du contenu sans déclencher de release).
	PromoteModeCode = "code"
	// PromoteModeTag (défaut) : code + tag de version → sur GitHub, le tag DÉCLENCHE la CI qui bâtit les
	// installeurs et publie la Release. C'est le cas normal d'une mise en vitrine versionnée.
	PromoteModeTag = "tag"
	// PromoteModeRelease : code + tag + création EXPLICITE de l'objet Release via l'API (avec notes de
	// version). Garantit que la Release existe même sans attendre la CI — mais les INSTALLEURS restent
	// produits par la CI (l'app ne compile pas de binaires). À utiliser quand on veut la page Release
	// tout de suite, les assets suivant via la CI.
	PromoteModeRelease = "release"
)

// PromoteOptions précise CE qui est promu : l'utilisateur décide la version, la branche d'atelier
// source, la branche de production cible et la MÉTHODE (Mode). Champs vides = valeurs par défaut
// (dernier tag de la branche par défaut de l'atelier → branche par défaut de la prod ; Mode = tag).
type PromoteOptions struct {
	SourceBranch string `json:"source_branch"` // branche de l'atelier d'où provient la version
	Version      string `json:"version"`       // tag/ref exact publié (ex. v1.8.0) ; vide = dernier tag de la branche
	TargetBranch string `json:"target_branch"` // branche de la production où publier
	Mode         string `json:"mode"`          // code | tag (défaut) | release
	Notes        string `json:"notes"`         // notes de version (Mode release) ; vide = généré
}

// promotionPlan traduit un mode (+ cible GitHub ou non) en actions concrètes et en une explication
// lisible — fonction PURE, testable, et source unique de vérité du « qui fait quoi » de la promotion.
func promotionPlan(mode string, github bool) (pushTag, makeRelease bool, explain string) {
	switch mode {
	case PromoteModeCode:
		return false, false, "Code publié sur la vitrine (sans tag), aucune version ni Release créée."
	case PromoteModeRelease:
		if github {
			return true, true, "Code + tag poussés et Release GitHub créée (notes de version). Les INSTALLEURS sont bâtis par la CI (Actions) ; vérifiez qu'elle passe au vert."
		}
		return true, true, "Code + tag poussés et Release créée sur la vitrine."
	default: // PromoteModeTag
		if github {
			return true, false, "Code + tag poussés. Sur GitHub, le tag DÉCLENCHE la CI (release.yml) qui bâtit les installeurs et publie la Release, vérifiez Actions, la Release n'est pas automatique côté app."
		}
		return true, false, "Code + tag poussés sur la vitrine."
	}
}

// ProjectPromote publie une version choisie de l'atelier vers UNE production du projet, en UN commit
// propre (pas les micro-commits). L'utilisateur décide la version, la branche source et la branche
// cible (voir PromoteOptions). Geste DISTANT vers la production → PIN TOUJOURS requis (fail-closed).
// L'IA ne fait jamais cela (prod bridée). Enregistre le marqueur d'équilibre après succès.
// PublishTag publie un TAG LOCAL (version déjà créée sur le poste) vers l'ATELIER, pour aligner la
// version affichée côté atelier sans re-promouvoir. Le CONTENU doit déjà être poussé : le tag ne fait
// que nommer un commit existant. PIN selon la politique de la connexion (atelier autonome par défaut :
// aucun geste). Comble le trou « Synchroniser envoie les commits mais pas les tags ».
func (s *Service) PublishTag(instance, owner, repo, localPath, tag, pin string) error {
	if !gitx.Available() {
		return ErrNoGit
	}
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return fmt.Errorf("version (tag) manquante")
	}
	if err := s.authorize(instance, config.ActionPush, pin); err != nil {
		return err
	}
	sha := gitx.TagSHA(localPath, tag)
	if sha == "" {
		return fmt.Errorf("la version « %s » n'existe pas en local", tag)
	}
	pr, err := s.ViewRepo(instance, owner, repo)
	if err != nil {
		return err
	}
	token, github, base := s.instanceToken(instance), s.isGitHub(instance), s.instanceBaseURL(instance)
	if _, err := gitx.PushTagTo(localPath, pr.CloneURL, tag, sha, token, github, base); err != nil {
		return fmt.Errorf("publication de la version « %s » vers l'atelier échouée : %w", tag, err)
	}
	return nil
}

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
		return nil, fmt.Errorf("aucun clone local de l'atelier %s/%s suivi, clonez-le d'abord", proj.Atelier.Owner, proj.Atelier.Repo)
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
		return nil, fmt.Errorf("l'atelier a des modifications non enregistrées, enregistrez (ou mettez de côté) avant de promouvoir")
	}
	// Invariant de flux : on ne promeut JAMAIS depuis le local directement. La vitrine se nourrit de
	// l'ATELIER, pas du poste. Si le clone a des commits non encore publiés sur l'atelier, on refuse :
	// publiez d'abord l'atelier (local → atelier), puis promouvez (atelier → vitrine).
	if st.Ahead > 0 {
		return nil, fmt.Errorf("votre clone local a %d commit(s) non publiés sur l'atelier, publiez d'abord l'atelier (local → atelier), puis promouvez (atelier → vitrine)", st.Ahead)
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
		version, _ = s.latestTag(proj.Atelier) // dernier tag de l'atelier (seul le plus récent compte)
		srcRef = version
	}
	if version == "" || srcRef == "" {
		return nil, fmt.Errorf("aucune version TAGUÉE à promouvoir sur « %s », créez d'abord un tag de version (ex. v1.8.0) sur l'atelier, puis promouvez", srcBranch)
	}
	// La version doit être PUBLIÉE sur l'ATELIER (son commit est déjà sur la branche distante), pas
	// seulement en local — sinon la vitrine recevrait du contenu absent de la source de vérité.
	if srcBranch != "" && !gitx.TagPublished(path, version, "origin/"+srcBranch) {
		return nil, fmt.Errorf("la version « %s » n'est pas publiée sur l'atelier, publiez-la d'abord (local → atelier) avant de promouvoir", version)
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
	pushTag, makeRelease, explain := promotionPlan(opts.Mode, github) // méthode choisie (code | tag | release)

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
	// Signer le commit de promotion (trailer immuable « SituLab-Approved: ») AVANT publication : la
	// garantie d'origine voyage ainsi avec le commit poussé sur la vitrine et se lit dans l'historique.
	// Best-effort : si la signature n'est pas disponible, on publie le commit d'origine (non bloquant).
	sha = s.attestPromotionSHA(path, sha)

	// 5) Publier vers la prod (fast-forward sur la base récupérée ; échoue proprement si la prod a
	//    bougé entre-temps — on ne force pas la vitrine à l'aveugle).
	if _, err := gitx.PushCommitTo(path, pr.CloneURL, sha, branch, token, github, base, false); err != nil {
		return nil, fmt.Errorf("publication vers la production échouée : %w", err)
	}

	// 6) Étiqueter la version sur la vitrine — SELON LA MÉTHODE. Mode « code » : pas de tag (simple
	//    alignement de contenu). Sinon le tag accompagne la vitrine (et, sur GitHub, déclenche la CI).
	if pushTag {
		if _, err := gitx.PushTagTo(path, pr.CloneURL, version, sha, token, github, base); err != nil {
			return nil, fmt.Errorf("commit publié mais étiquetage « %s » de la vitrine échoué : %w", version, err)
		}
	}

	// 6b) Mode « release » : créer EXPLICITEMENT l'objet Release via l'API (notes de version). Les
	//     installeurs, eux, restent bâtis par la CI. Échec signalé sans défaire le code déjà publié.
	if makeRelease {
		notes := strings.TrimSpace(opts.Notes)
		if notes == "" {
			notes = fmt.Sprintf("Version %s promue depuis l'atelier %s.", version, srcLabel)
		}
		if _, err := s.CreateRelease(prod.Instance, prod.Owner, prod.Repo, version, version, notes, version, false, false, pin); err != nil {
			return &PromoteResult{
				Project: projectName, Production: prod.Owner + "/" + prod.Repo, Version: version, Branch: branch,
				Summary: fmt.Sprintf("Code + tag %s publiés sur %s/%s, mais la création de la Release a échoué : %s", version, prod.Owner, prod.Repo, cleanClientErr(err)),
			}, nil
		}
	}

	// 7) Marqueur d'équilibre : une version n'est « promue » que si elle a été taguée (sinon simple
	//    alignement de contenu sans version publiée).
	if pushTag {
		s.recordPromotion(projectName, prod.Key(), version)
	}

	res := &PromoteResult{
		Project: projectName, Production: prod.Owner + "/" + prod.Repo, Version: version, Branch: branch,
		Summary: fmt.Sprintf("%s/%s → %s : %s", prod.Owner, prod.Repo, branch, explain),
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
