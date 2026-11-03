// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"fmt"
	"situlab/internal/config"
	"situlab/internal/gitx"
	"strings"
	"sync"
)

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
