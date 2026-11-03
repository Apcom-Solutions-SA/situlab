// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"situlab/internal/config"
	"situlab/internal/forgejo"
	"situlab/internal/gitx"
)

// pruneMissingLocalRepos cesse de suivre les chemins locaux dont le DOSSIER N'EXISTE PLUS (clone
// supprimé). Auto-nettoyage : un dossier effacé ne doit pas rester en « à vérifier / illisible » —
// il disparaît simplement. On ne retire QUE sur « n'existe pas » (os.IsNotExist), pas sur une autre
// erreur (droits, volume déconnecté), pour ne pas oublier un clone temporairement indisponible.
func (s *Service) pruneMissingLocalRepos() {
	s.mu.Lock()
	s.lockedReload()
	paths := append([]string(nil), s.cfg.LocalRepos...)
	s.mu.Unlock()

	gone := map[string]bool{}
	for _, p := range paths {
		if _, err := os.Stat(p); err != nil && os.IsNotExist(err) {
			gone[config.CleanPath(p)] = true
		}
	}
	if len(gone) == 0 {
		return
	}
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	cfg, err := config.Load()
	if err != nil {
		return
	}
	out := cfg.LocalRepos[:0]
	for _, p := range cfg.LocalRepos {
		if !gone[config.CleanPath(p)] {
			out = append(out, p)
		}
	}
	cfg.LocalRepos = out
	if cfg.Save() == nil {
		s.publish(cfg)
	}
}

// RepoView est la vue unifiée d'un dépôt : son état distant ET son clone local
// rattaché. C'est l'objet central de SituLab : « ce que j'ai en ligne et en local, et
// dans quel état ».
type RepoView struct {
	// Distant
	FullName      string `json:"full_name"`
	Owner         string `json:"owner"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	Private       bool   `json:"private"`
	DefaultBranch string `json:"default_branch"`
	HTMLURL       string `json:"html_url"`
	CloneURL      string `json:"clone_url"`
	HasRemote     bool   `json:"has_remote"`
	// Local rattaché
	Matched      bool   `json:"matched"`
	LocalPath    string `json:"local_path"`
	LocalBranch  string `json:"local_branch"`
	Detached     bool   `json:"detached"`
	Describe     string `json:"describe"`
	Ahead        int    `json:"ahead"`
	Behind       int    `json:"behind"`
	Dirty        bool   `json:"dirty"`
	ChangedCount int    `json:"changed_count"`
	LastDate     string `json:"last_date"`   // date ISO du dernier commit local
	LocalError   string `json:"local_error"` // non vide = clone local illisible
	LocalKind    string `json:"local_kind"`  // "clone" | "worktree" | "" (indéterminé)
	ViaAtelier   bool   `json:"via_atelier"` // vitrine présente localement VIA le clone d'atelier du projet (1.9.11)
	// État synthétique pour l'UI (source unique de vérité)
	State string `json:"state"` // uptodate|ahead|behind|diverged|dirty|detached|unreadable|nolocal
}

// DashboardData est le retour du tableau de bord : les vues + des méta d'état.
type DashboardData struct {
	Repos     []RepoView `json:"repos"`
	Truncated bool       `json:"truncated"` // liste distante potentiellement incomplète
	Instance  string     `json:"instance"`
	Role      string     `json:"role"` // rôle de la connexion : "atelier" | "production" (v1.7)
}

// localScan est le résultat du résumé git d'un dépôt local suivi.
type localScan struct {
	sum  *gitx.Sum
	err  error
	path string
}

// Dashboard renvoie la vue unifiée : dépôts distants enrichis de leur clone local,
// plus les dépôts locaux non rattachés. Les clones illisibles sont AFFICHÉS (avec leur
// cause), jamais masqués — c'est le cœur de la promesse « montrer l'état réel ».
func (s *Service) Dashboard(instance string) (*DashboardData, error) {
	cl, inst, err := s.clientFor(instance)
	if err != nil {
		return nil, err
	}
	s.pruneMissingLocalRepos() // auto-nettoyage : oublie les clones dont le dossier a disparu

	// 1. Snapshot des chemins locaux suivis (sous verrou), puis scan EN PARALLÈLE.
	s.mu.Lock()
	s.lockedReload()
	raw := append([]string(nil), s.cfg.LocalRepos...)
	projects := append([]config.Project(nil), s.cfg.Projects...)
	instances := append([]config.Instance(nil), s.cfg.Instances...)
	s.mu.Unlock()
	// Dédoublonnage défensif : un même chemin présent deux fois (ajouté sous deux formes) ne
	// doit JAMAIS produire deux cartes du même dépôt.
	paths := make([]string, 0, len(raw))
	seenPath := map[string]bool{}
	for _, p := range raw {
		k := config.CleanPath(p)
		if seenPath[k] {
			continue
		}
		seenPath[k] = true
		paths = append(paths, p)
	}

	scans := make([]localScan, len(paths))
	if gitx.Available() {
		sem := make(chan struct{}, 8)
		var wg sync.WaitGroup
		for i, p := range paths {
			wg.Add(1)
			go func(i int, p string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				sum, err := gitx.SummaryOf(p)
				scans[i] = localScan{sum: sum, err: err, path: p}
			}(i, p)
		}
		wg.Wait()
	}

	// Map 1:N : plusieurs clones locaux peuvent partager la même URL distante
	// (ex. deux copies d'un dépôt). On les conserve TOUS — jamais de clone masqué.
	byURL := map[string][]localScan{}
	var orphans []localScan
	for _, sc := range scans {
		if sc.sum == nil {
			continue
		}
		if sc.err != nil || !sc.sum.IsRepo {
			orphans = append(orphans, sc) // illisible ou disparu : affiché à part
			continue
		}
		if key := normURL(sc.sum.Remote); key != "" {
			byURL[key] = append(byURL[key], sc)
		} else {
			orphans = append(orphans, sc)
		}
	}

	// 2. Dépôts distants (cache court), rattachés à leur local.
	ctx, cancel := opCtx()
	defer cancel()
	repos, trunc, err := s.cachedRepos(ctx, inst.Name, cl)
	if err != nil {
		return nil, err
	}

	// Appariement PAR PROJET (1.9.11) : sur une vitrine, une production dont l'atelier est cloné ici
	// n'a pas à être « récupérée » — on l'a déjà, via le clone d'atelier. Le lien atelier↔vitrine est
	// déclaré par le projet (hôtes et propriétaires diffèrent, l'URL seule ne peut pas les rapprocher).
	instHost := map[string]string{}
	for _, in := range instances {
		instHost[in.Name] = hostOf(in.URL)
	}
	prodClone := map[string]string{}      // lower(owner/repo) de la vitrine → chemin du clone d'atelier
	prodAtelierKey := map[string]string{} // lower(owner/repo) de la vitrine → clé URL de l'atelier (pour marquer « utilisé »)
	for _, p := range projects {
		ak := strings.ToLower(instHost[p.Atelier.Instance] + "/" + p.Atelier.Owner + "/" + p.Atelier.Repo)
		locals, ok := byURL[ak]
		if !ok || len(locals) == 0 {
			continue // atelier du projet non cloné ici : rien à rapprocher
		}
		for _, pr := range p.Productions {
			if !strings.EqualFold(pr.Instance, inst.Name) {
				continue // production sur une AUTRE connexion que celle affichée
			}
			k := strings.ToLower(pr.Owner + "/" + pr.Repo)
			prodClone[k] = locals[0].path
			prodAtelierKey[k] = ak
		}
	}

	used := map[string]bool{}
	views := make([]RepoView, 0, len(repos)+len(orphans))
	for _, r := range repos {
		owner := r.FullName
		if i := strings.IndexByte(owner, '/'); i >= 0 {
			owner = owner[:i]
		}
		v := RepoView{
			FullName: r.FullName, Owner: owner, Name: r.Name,
			Description: r.Description, Private: r.Private,
			DefaultBranch: r.DefaultBranch, HTMLURL: r.HTMLURL, CloneURL: r.CloneURL,
			HasRemote: true,
		}
		// Appariement par URL normalisée (host/owner/repo) — sensible à l'hôte, donc sûr en
		// multi-serveurs (Forgejo + GitHub) : un clone dont l'origine ne correspond pas à CE dépôt
		// s'affiche en « local isolé » plutôt que d'être rattaché à tort. Le dédoublonnage des
		// chemins (plus haut) empêche, lui, qu'un même clone produise deux cartes.
		var locals []localScan
		if l, ok := byURL[normURL(r.CloneURL)]; ok {
			locals, used[normURL(r.CloneURL)] = l, true
		} else if l, ok := byURL[normURL(r.SSHURL)]; ok {
			locals, used[normURL(r.SSHURL)] = l, true
		}
		if len(locals) == 0 {
			// Pas de clone direct de CETTE URL ? Dernière chance : c'est une vitrine dont l'atelier
			// (même projet) est cloné ici → on la marque « présente via l'atelier » et on ne proposera
			// pas de la récupérer. On marque aussi le clone d'atelier « utilisé » pour éviter qu'il
			// réapparaisse en double comme carte locale isolée (étape 3).
			if path, ok := prodClone[strings.ToLower(r.FullName)]; ok {
				v.LocalPath = path
				v.LocalKind = detectLocalKind(path)
				v.ViaAtelier = true
				v.State = "uptodate"
				used[prodAtelierKey[strings.ToLower(r.FullName)]] = true
				views = append(views, v)
				continue
			}
			v.State = computeState(&v)
			views = append(views, v)
			continue
		}
		for i := range locals { // une carte par clone local rattaché
			vv := v
			fillLocal(&vv, &locals[i])
			vv.State = computeState(&vv)
			views = append(views, vv)
		}
	}

	// 3. Clones locaux non rattachés (hors instance, illisibles, ou disparus).
	for key, locals := range byURL {
		if used[key] {
			continue
		}
		for i := range locals {
			l := locals[i]
			v := RepoView{Name: lastSeg(l.path), FullName: l.sum.Remote}
			fillLocal(&v, &l)
			v.State = computeState(&v)
			views = append(views, v)
		}
	}
	for _, l := range orphans {
		v := RepoView{Name: lastSeg(l.path)}
		fillLocal(&v, &l)
		v.State = computeState(&v)
		views = append(views, v)
	}

	// Tri : rattachés au distant d'abord, puis par nom.
	sort.SliceStable(views, func(i, j int) bool {
		if views[i].HasRemote != views[j].HasRemote {
			return views[i].HasRemote
		}
		return strings.ToLower(views[i].FullName+views[i].Name) < strings.ToLower(views[j].FullName+views[j].Name)
	})
	return &DashboardData{Repos: views, Truncated: trunc, Instance: inst.Name, Role: inst.NormalizedRole()}, nil
}

func fillLocal(v *RepoView, sc *localScan) {
	v.LocalPath = sc.path
	v.LocalKind = detectLocalKind(sc.path)
	if sc.err != nil {
		v.LocalError = sc.err.Error()
		return
	}
	sum := sc.sum
	v.Matched = v.HasRemote
	v.LocalBranch = sum.Branch
	v.Detached = sum.Detached
	v.Describe = sum.Describe
	v.Ahead = sum.Ahead
	v.Behind = sum.Behind
	v.Dirty = !sum.Clean
	v.ChangedCount = len(sum.Changed)
	v.LastDate = sum.LastDate
}

// detectLocalKind distingue un worktree git (`.git` est un FICHIER pointeur vers le dépôt
// principal) d'un clone autonome (`.git` est un DOSSIER). Renvoie "" si indéterminable
// (dossier absent, droits) — l'UI n'affiche alors aucun badge de type.
func detectLocalKind(path string) string {
	fi, err := os.Stat(filepath.Join(path, ".git"))
	if err != nil {
		return ""
	}
	if fi.IsDir() {
		return "clone"
	}
	return "worktree"
}

// computeState réduit l'état d'un dépôt à un code unique, consommé par l'UI pour
// choisir libellé/icône/couleur. Source unique de vérité du modèle d'état.
func computeState(v *RepoView) string {
	if v.LocalError != "" {
		return "unreadable"
	}
	if v.LocalPath == "" {
		return "nolocal"
	}
	if v.Detached {
		return "detached"
	}
	if v.Dirty {
		return "dirty"
	}
	switch {
	case v.Ahead > 0 && v.Behind > 0:
		return "diverged"
	case v.Ahead > 0:
		return "ahead"
	case v.Behind > 0:
		return "behind"
	}
	return "uptodate"
}

// LocalRepos renvoie l'état des dépôts locaux suivis (pour la CLI `local list`).
func (s *Service) LocalRepos() ([]*gitx.Status, error) {
	if !gitx.Available() {
		return nil, ErrNoGit
	}
	s.pruneMissingLocalRepos() // oublie les clones supprimés avant de lister
	s.mu.Lock()
	s.lockedReload()
	paths := append([]string(nil), s.cfg.LocalRepos...)
	s.mu.Unlock()

	out := make([]*gitx.Status, 0, len(paths))
	for _, p := range paths {
		st, err := gitx.StatusOf(p)
		if err != nil {
			st = &gitx.Status{Path: p, IsRepo: false}
		}
		out = append(out, st)
	}
	return out, nil
}

// SuggestCloneBase propose un dossier PARENT logique pour un nouveau clone : le répertoire qui
// héberge déjà le plus de clones suivis sur ce poste (« range le nouveau près des autres »). Rend
// une chaîne vide si aucun clone n'est suivi — l'appelant retombe alors sur le choix manuel. Lecture
// pure, aucun accès réseau. Le dossier final (parent + nom) est composé côté fenêtre.
func (s *Service) SuggestCloneBase() string {
	s.mu.Lock()
	s.lockedReload()
	paths := append([]string(nil), s.cfg.LocalRepos...)
	s.mu.Unlock()

	counts := map[string]int{}
	order := []string{} // préserve l'ordre de 1re rencontre pour un départage stable
	for _, p := range paths {
		if p == "" {
			continue
		}
		d := filepath.Dir(p)
		if d == "" || d == "." {
			continue
		}
		if _, seen := counts[d]; !seen {
			order = append(order, d)
		}
		counts[d]++
	}
	best, bestN := "", 0
	for _, d := range order {
		if counts[d] > bestN {
			best, bestN = d, counts[d]
		}
	}
	return best
}

// FetchAllLocal récupère en arrière-plan (best-effort) les nouveautés distantes de chaque dépôt
// local suivi, pour que l'état « en retard / nouveautés en ligne » reste à jour sans action
// manuelle du non-développeur. L'instance est choisie par correspondance d'hôte du distant (à
// défaut l'instance par défaut). LECTURE : aucun PIN, et rien ne modifie l'arbre de travail
// (fetch seul). Les erreurs par dépôt sont ignorées (réseau coupé, dépôt sans distant, hôte
// inconnu…) : une synchro de fond ne doit jamais interrompre l'utilisateur. Renvoie le nombre de
// dépôts récupérés avec succès.
func (s *Service) FetchAllLocal() (int, error) {
	if !gitx.Available() {
		return 0, ErrNoGit
	}
	if !s.Unlocked() {
		return 0, nil // session verrouillée : on ne tente rien (les tokens ne sont pas déchiffrés)
	}
	s.RefreshRevokedServers() // révocation des services de signature serveur (best-effort, 1.9.6)
	// Anti-chevauchement : un seul fetch de fond à la fois. Si un précédent tourne encore (dépôts
	// lents/injoignables), on saute ce tour au lieu d'empiler des `git fetch` concurrents sur les
	// mêmes dépôts (contention sur les verrous de refs .git).
	if !s.fetchMu.TryLock() {
		return 0, nil
	}
	defer s.fetchMu.Unlock()

	s.mu.Lock()
	s.lockedReload()
	paths := append([]string(nil), s.cfg.LocalRepos...)
	s.mu.Unlock()

	n := 0
	for _, p := range paths {
		// Un seul StatusOf par dépôt : il donne déjà IsRepo et l'URL du distant (évite un
		// `git remote` + un second StatusOf par l'ancien instanceForRepo).
		st, err := gitx.StatusOf(p)
		if err != nil || !st.IsRepo || st.Remote == "" {
			continue
		}
		// Ne jamais lancer un fetch de fond sur un dépôt qu'une action au premier plan est en train
		// d'utiliser : on saute ce tour pour ce dépôt (il sera repris au prochain cycle). Évite deux
		// processus git concurrents sur le même .git et une info lue pendant une écriture de refs.
		release, ok := s.tryLockRepo(p)
		if !ok {
			continue
		}
		inst := s.instanceForHost(hostOf(st.Remote))
		token, github, baseURL := s.connFor(inst) // une seule résolution de config par dépôt
		if _, err := gitx.Fetch(p, token, github, baseURL); err == nil {
			n++
		}
		release()
	}
	return n, nil
}

// instanceForHost renvoie l'instance correspondant à un hôte de distant, UNIQUEMENT si une seule
// instance configurée partage cet hôte. Avec zéro ou plusieurs correspondances (deux comptes sur
// le même serveur), on renvoie "" (instance par défaut) plutôt que de deviner : cela évite
// d'utiliser le jeton d'un compte pour un dépôt d'un autre compte du même hôte.
func (s *Service) instanceForHost(host string) string {
	if host == "" {
		return ""
	}
	match, count := "", 0
	for _, in := range s.Instances() {
		if hostOf(in.URL) == host {
			match = in.Name
			count++
		}
	}
	if count == 1 {
		return match
	}
	return ""
}

// hostOf extrait l'hôte (minuscule) d'une URL de dépôt OU d'une URL d'instance (sans chemin),
// en gérant https, ssh/scp et une forme nue. Complète normURL, qui exige un chemin.
func hostOf(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	switch {
	case strings.Contains(raw, "://"):
		if u, err := url.Parse(raw); err == nil {
			return strings.ToLower(u.Hostname())
		}
	case strings.Contains(raw, "@") && strings.Contains(raw, ":"):
		rest := strings.SplitN(raw, "@", 2)[1]
		return strings.ToLower(strings.SplitN(rest, ":", 2)[0])
	}
	seg := strings.SplitN(strings.TrimPrefix(raw, "/"), "/", 2)[0]
	return strings.ToLower(seg)
}

// --- Détail distant (branches, tags, commits, arborescence, fichier) ---

func (s *Service) RepoBranches(instance, owner, repo string) ([]forgejo.Branch, error) {
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return nil, err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.ListBranches(ctx, owner, repo)
}

func (s *Service) RepoTags(instance, owner, repo string) ([]forgejo.Tag, error) {
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return nil, err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.ListTags(ctx, owner, repo)
}

func (s *Service) RepoCommits(instance, owner, repo, ref string, limit int) ([]forgejo.Commit, error) {
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return nil, err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.ListCommits(ctx, owner, repo, ref, limit)
}

func (s *Service) RepoContents(instance, owner, repo, path, ref string) ([]forgejo.Entry, error) {
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return nil, err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.ListContents(ctx, owner, repo, path, ref)
}

func (s *Service) RepoFile(instance, owner, repo, path, ref string) (string, error) {
	cl, _, err := s.clientFor(instance)
	if err != nil {
		return "", err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.GetFile(ctx, owner, repo, path, ref)
}

// --- Appariement local <-> distant par URL ---

// normURL réduit une URL de dépôt (https, ssh, scp) à « host/owner/repo » minuscules.
func normURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var host, path string
	switch {
	case strings.Contains(raw, "://"):
		if u, err := url.Parse(raw); err == nil {
			host = u.Hostname()
			path = u.Path
		}
	case strings.Contains(raw, "@") && strings.Contains(raw, ":"):
		rest := strings.SplitN(raw, "@", 2)[1]
		c := strings.SplitN(rest, ":", 2)
		host = c[0]
		if len(c) > 1 {
			path = c[1]
		}
	default:
		return ""
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	if host == "" || path == "" {
		return ""
	}
	return strings.ToLower(host + "/" + path)
}

func lastSeg(p string) string {
	p = strings.TrimRight(p, "/\\")
	if i := strings.LastIndexAny(p, "/\\"); i >= 0 {
		return p[i+1:]
	}
	return p
}
