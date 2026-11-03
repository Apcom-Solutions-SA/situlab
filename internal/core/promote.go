// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"fmt"
	"situlab/internal/config"
	"situlab/internal/gitx"
	"strings"
	"time"
)

// PromoteResult décrit, en clair, ce qu'une promotion a publié.
type PromoteResult struct {
	Project    string `json:"project"`
	Production string `json:"production"` // owner/repo de la prod
	Version    string `json:"version"`
	Branch     string `json:"branch"`
	Summary    string `json:"summary"`
}

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
