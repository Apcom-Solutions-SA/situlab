// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

// Package cli est le frontal ligne de commande de SituLab (binaire `situlab`).
// Il traduit des commandes en appels au noyau et présente le retour en texte ou JSON.
// Conçu pour être piloté par un agent IA : voir docs/AGENTS.md.
//
// Contrat agent : en --output json, les SUCCÈS comme les ERREURS sont du JSON, et les
// codes de sortie sont stables (0 ok, 1 générique, 2 instance, 3 PIN, 4 git absent).
package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"situlab/internal/attest"
	"situlab/internal/core"
	"situlab/internal/ipc"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	flagOutput   string
	flagInstance string
	flagIntent   string // Point 4 : phrase « pourquoi » de l'agent, affichée dans la fenêtre PIN
	version      = "dev"
	svc          *remote // client du démon résident (connexion paresseuse)
)

// Execute lance la CLI et renvoie un code de sortie. La CLI ne détient pas de session :
// elle relaie tout au démon (qui porte la session et affiche la modale PIN sur le poste).
func Execute(ver string) int {
	version = ver
	svc = newRemote()
	if err := root().Execute(); err != nil {
		renderError(os.Stderr, err)
		return codeFor(err)
	}
	return 0
}

func codeFor(err error) int {
	switch {
	case errors.Is(err, core.ErrNoInstance):
		return 2
	case errors.Is(err, core.ErrPINNeeded), errors.Is(err, core.ErrPINLocked), errors.Is(err, core.ErrLocked), errors.Is(err, core.ErrPINTimeout):
		return 3
	case errors.Is(err, core.ErrNoGit):
		return 4
	default:
		return 1
	}
}

// renderError émet une erreur structurée en JSON (ou un texte lisible) sur w.
func renderError(w *os.File, err error) {
	if flagOutput == "json" {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(map[string]any{"error": map[string]string{"message": err.Error()}})
		return
	}
	fmt.Fprintln(w, "erreur : "+err.Error())
}

// emit affiche un résultat en JSON ou en texte (via textFn). Renvoie l'erreur
// d'encodage éventuelle pour qu'un échec de sérialisation ne passe pas en code 0.
func emit(cmd *cobra.Command, data any, textFn func()) error {
	if flagOutput == "json" {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(data)
	}
	textFn()
	return nil
}

func root() *cobra.Command {
	root := &cobra.Command{
		Use:           "situlab",
		Short:         "SituLab — client Forgejo de bureau, l'utilitaire git pour tous",
		Long:          "SituLab connecte une ou plusieurs instances Forgejo via URL + token,\nexpose dépôts/profil/tokens selon vos droits et pilote git en local.\nSans argument : ouvre la fenêtre. Avec une commande : mode agent (voir docs/AGENTS.md).",
		SilenceUsage:  true,
		SilenceErrors: true, // le rendu d'erreur (texte/JSON) est géré par Execute
		Version:       version,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if flagOutput != "text" && flagOutput != "json" {
				return fmt.Errorf("format de sortie inconnu : %q (text|json)", flagOutput)
			}
			return nil
		},
	}
	root.PersistentFlags().StringVar(&flagOutput, "output", "text", "format de sortie : text|json")
	root.PersistentFlags().StringVar(&flagInstance, "instance", "", "instance ciblée (défaut : instance par défaut)")
	root.PersistentFlags().StringVar(&flagIntent, "intent", "", "phrase expliquant POURQUOI (affichée dans la fenêtre PIN sur le poste, à côté des actions constatées par SituLab)")

	root.AddCommand(cmdVersion(), cmdDoctor(), cmdInstance(), cmdWhoami(), cmdDashboard(),
		cmdRepo(), cmdIssue(), cmdPull(), cmdRelease(), cmdOrg(), cmdToken(), cmdLocal(), cmdProject(), cmdAttest(), cmdAdmin(), cmdPIN(), cmdMCP(),
		cmdCloud(), cmdBackup(), cmdKernel(), cmdSpoke(), cmdMap(),
		cmdUnlock(), cmdLock(), cmdStatus(), cmdPurge())
	return root
}

// cmdPurge efface toutes les données locales de SituLab (connexions, trousseau, réglages, jetons
// du coffre OS). IRRÉVERSIBLE. Tourne EN-PROCESS (pas via le démon, qui ouvrirait la fenêtre) et
// refuse tant que le démon répond sur le socket (fenêtre ouverte — elle réécrirait la config).
// Exige --confirm.
func cmdPurge() *cobra.Command {
	var confirm bool
	c := &cobra.Command{
		Use:   "purge",
		Short: "Efface TOUTES les données SituLab de ce poste (IRRÉVERSIBLE ; --confirm requis)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !confirm {
				return errors.New("ajoutez --confirm pour confirmer cet effacement irréversible")
			}
			// Vivacité par sonde BRUTE (sans handshake jeton) : le garde doit tenir même face à un démon
			// d'une version antérieure qui n'a pas écrit daemon.token (sinon Dial échouerait et on
			// effacerait tout pendant qu'une fenêtre vivante tourne — fail-open corrigé en 1.16).
			if ipc.Alive(ipc.SocketPath()) {
				return errors.New("fermez d'abord la fenêtre SituLab, puis relancez « situlab purge --confirm »")
			}
			svc0, err := core.New(version)
			if err != nil {
				return err
			}
			if err := svc0.Purge(); err != nil {
				return err
			}
			return emit(cmd, map[string]string{"status": "ok"}, func() {
				fmt.Fprintln(cmd.OutOrStdout(), "Données SituLab effacées de ce poste.")
			})
		},
	}
	c.Flags().BoolVar(&confirm, "confirm", false, "confirme l'effacement irréversible")
	return c
}

func cmdVersion() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Affiche la version de SituLab",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return emit(cmd, map[string]string{"version": version}, func() {
				fmt.Fprintln(cmd.OutOrStdout(), "SituLab "+version)
			})
		},
	}
}

func cmdDoctor() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Diagnostique l'environnement (git, config, trousseau, session)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			d, err := svc.Doctor()
			if err != nil {
				return err
			}
			return emit(cmd, d, func() {
				out := cmd.OutOrStdout()
				okko := map[bool]string{true: "OK", false: "ABSENT"}
				fmt.Fprintf(out, "git        : %s %s\n", okko[d.GitAvailable], d.GitVersion)
				fmt.Fprintf(out, "config     : %s\n", d.ConfigPath)
				fmt.Fprintf(out, "trousseau  : %s\n", d.KeyringPath)
				fmt.Fprintf(out, "instances  : %d\n", d.Instances)
				fmt.Fprintf(out, "PIN        : %s\n", map[bool]string{true: "défini", false: "non défini"}[d.PINSet])
				fmt.Fprintf(out, "session    : %s\n", map[bool]string{true: "déverrouillée", false: "verrouillée"}[d.Unlocked])
				if d.VaultResidual > 0 {
					fmt.Fprintf(out, "⚠ coffre OS: %d token(s) résiduel(s) à migrer\n", d.VaultResidual)
				}
				if d.SignTestRoot {
					fmt.Fprintln(out, "⚠ signature de garantie : racine de TEST embarquée (à remplacer par la vraie racine de gouvernance avant mise en prod)")
				}
			})
		},
	}
}

func cmdInstance() *cobra.Command {
	c := &cobra.Command{Use: "instance", Short: "Gère les instances Forgejo"}

	var instURL, instToken, instProvider string
	add := &cobra.Command{
		Use:   "add <nom>",
		Short: "Ajoute une instance avec un token déjà en main (chiffré par le PIN)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if instToken != "" {
				fmt.Fprintln(os.Stderr, "⚠ token fourni en ligne de commande (visible dans l'historique) ; préférez la saisie interactive.")
			} else {
				t, err := promptSecret("Token pour " + args[0] + " : ")
				if err != nil {
					return err
				}
				instToken = t
			}
			if err := svc.AddInstanceToken(args[0], instProvider, instURL, instToken); err != nil {
				return err
			}
			return emit(cmd, map[string]string{"status": "ok", "instance": args[0]}, func() {
				fmt.Fprintf(cmd.OutOrStdout(), "Instance « %s » ajoutée.\n", args[0])
			})
		},
	}
	add.Flags().StringVar(&instURL, "url", "", "URL de l'instance (Forgejo ; ex. https://git.apcom.app)")
	add.Flags().StringVar(&instToken, "token", "", "token personnel (sinon demandé)")
	add.Flags().StringVar(&instProvider, "provider", "forgejo", "fournisseur : forgejo | github")

	// login : onboarding « PIN-only » — SituLab crée le token lui-même à partir des
	// identifiants (le mot de passe et l'OTP ne sont jamais stockés).
	var logURL, logUser, logPass, logOTP string
	var logAdmin, logRemember bool
	login := &cobra.Command{
		Use:   "login <nom>",
		Short: "Connecte une instance par identifiant/mot de passe (SituLab crée le token)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if logPass == "" {
				p, err := promptSecret("Mot de passe pour " + logUser + " : ")
				if err != nil {
					return err
				}
				logPass = p
			}
			err := svc.AddInstanceLogin(args[0], logURL, logUser, logPass, logOTP, logAdmin, logRemember)
			// Compte avec 2FA : réessayer en demandant le code — mais SEULEMENT si l'échec
			// ressemble à un refus d'authentification, pas à une erreur locale (doublon
			// d'instance, session verrouillée, URL invalide) pour laquelle un code 2FA serait
			// trompeur et inutile.
			if err != nil && logOTP == "" && looksLikeAuthFailure(err) && term.IsTerminal(int(os.Stdin.Fd())) {
				o, e := promptSecret("Code 2FA (laisser vide si aucun) : ")
				if e == nil && o != "" {
					logOTP = o
					err = svc.AddInstanceLogin(args[0], logURL, logUser, logPass, logOTP, logAdmin, logRemember)
				}
			}
			if err != nil {
				return err
			}
			return emit(cmd, map[string]string{"status": "ok", "instance": args[0]}, func() {
				fmt.Fprintf(cmd.OutOrStdout(), "Instance « %s » connectée (token créé par SituLab).\n", args[0])
			})
		},
	}
	login.Flags().StringVar(&logURL, "url", "", "URL de l'instance")
	login.Flags().StringVar(&logUser, "user", "", "identifiant du compte")
	login.Flags().StringVar(&logPass, "password", "", "mot de passe (sinon demandé)")
	login.Flags().StringVar(&logOTP, "otp", "", "code 2FA (si le compte en a un)")
	login.Flags().BoolVar(&logAdmin, "admin", false, "compte administrateur : inclure la portée de gestion des utilisateurs")
	login.Flags().BoolVar(&logRemember, "remember", true, "mémoriser le mot de passe (chiffré par le PIN) pour ne plus le redemander")
	_ = login.MarkFlagRequired("url")
	_ = login.MarkFlagRequired("user")

	list := &cobra.Command{
		Use:   "list",
		Short: "Liste les instances configurées",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ins, err := svc.Instances()
			if err != nil {
				return err
			}
			return emit(cmd, ins, func() {
				out := cmd.OutOrStdout()
				if len(ins) == 0 {
					fmt.Fprintln(out, "Aucune instance. Ajoutez-en une : situlab instance add <nom> --url <url>")
					return
				}
				for _, in := range ins {
					mark := "  "
					if in.Default {
						mark = "* "
					}
					fmt.Fprintf(out, "%s%-14s %s\n", mark, in.Name, in.URL)
				}
			})
		},
	}

	test := &cobra.Command{
		Use:   "test <nom>",
		Short: "Teste la connexion à une instance",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := flagInstance
			if len(args) == 1 {
				name = args[0]
			}
			u, err := svc.TestInstance(name)
			if err != nil {
				return err
			}
			return emit(cmd, u, func() {
				fmt.Fprintf(cmd.OutOrStdout(), "Connexion OK — %s (%s)\n", u.Login, u.FullName)
			})
		},
	}

	remove := &cobra.Command{
		Use:   "remove <nom>",
		Short: "Supprime une instance et son token",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := svc.RemoveInstance(args[0]); err != nil {
				return err
			}
			return emit(cmd, map[string]string{"status": "ok"}, func() {
				fmt.Fprintf(cmd.OutOrStdout(), "Instance « %s » supprimée.\n", args[0])
			})
		},
	}

	def := &cobra.Command{
		Use:   "default <nom>",
		Short: "Définit l'instance par défaut",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := svc.SetDefaultInstance(args[0]); err != nil {
				return err
			}
			return emit(cmd, map[string]string{"status": "ok", "default": args[0]}, func() {
				fmt.Fprintf(cmd.OutOrStdout(), "Instance par défaut : « %s ».\n", args[0])
			})
		},
	}

	c.AddCommand(add, login, list, test, remove, def)
	return c
}

func cmdWhoami() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Affiche l'utilisateur courant",
		RunE: func(cmd *cobra.Command, _ []string) error {
			u, err := svc.Whoami(flagInstance)
			if err != nil {
				return err
			}
			return emit(cmd, u, func() {
				admin := ""
				if u.IsAdmin {
					admin = " [admin]"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s — %s <%s>%s\n", u.Login, u.FullName, u.Email, admin)
			})
		},
	}
}

func cmdDashboard() *cobra.Command {
	return &cobra.Command{
		Use:   "dashboard",
		Short: "Vue unifiée distant ⇄ local (le cœur de SituLab)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			data, err := svc.Dashboard(flagInstance)
			if err != nil {
				return err
			}
			return emit(cmd, data, func() {
				out := cmd.OutOrStdout()
				for _, v := range data.Repos {
					name := v.FullName
					if !v.HasRemote {
						name = v.Name + " (local)"
					}
					loc := "non cloné"
					switch {
					case v.LocalError != "":
						loc = "illisible : " + v.LocalError
					case v.LocalPath != "":
						st := map[bool]string{true: "modifié", false: "propre"}[v.Dirty]
						loc = fmt.Sprintf("%s @%s %s ↑%d/↓%d", v.Describe, v.LocalBranch, st, v.Ahead, v.Behind)
					}
					fmt.Fprintf(out, "%-42s [%-9s] dist:%-14s loc:%s\n", name, v.State, v.DefaultBranch, loc)
				}
				if data.Truncated {
					fmt.Fprintln(out, "\n(liste distante partielle : beaucoup de dépôts)")
				}
			})
		},
	}
}

func cmdRepo() *cobra.Command {
	c := &cobra.Command{Use: "repo", Short: "Dépôts (distant + git local)"}

	list := &cobra.Command{
		Use:   "list",
		Short: "Liste tes dépôts accessibles",
		RunE: func(cmd *cobra.Command, _ []string) error {
			repos, err := svc.Repos(flagInstance)
			if err != nil {
				return err
			}
			return emit(cmd, repos, func() {
				out := cmd.OutOrStdout()
				for _, r := range repos {
					vis := map[bool]string{true: "privé", false: "public"}[r.Private]
					fmt.Fprintf(out, "%-40s %-7s %s\n", r.FullName, vis, r.Description)
				}
				fmt.Fprintf(out, "\n%d dépôt(s).\n", len(repos))
			})
		},
	}

	view := &cobra.Command{
		Use:   "view <owner>/<repo>",
		Short: "Détail d'un dépôt",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			owner, repo, err := splitRepo(args[0])
			if err != nil {
				return err
			}
			r, err := svc.ViewRepo(flagInstance, owner, repo)
			if err != nil {
				return err
			}
			return emit(cmd, r, func() {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\n%s\nclone : %s\nweb   : %s\ndroits: pull=%v push=%v admin=%v\n",
					r.FullName, r.Description, r.CloneURL, r.HTMLURL,
					r.Permissions.Pull, r.Permissions.Push, r.Permissions.Admin)
			})
		},
	}

	branches := &cobra.Command{
		Use:   "branches <owner>/<repo>",
		Short: "Branches distantes",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			owner, repo, err := splitRepo(args[0])
			if err != nil {
				return err
			}
			b, err := svc.RepoBranches(flagInstance, owner, repo)
			if err != nil {
				return err
			}
			return emit(cmd, b, func() {
				for _, x := range b {
					fmt.Fprintln(cmd.OutOrStdout(), x.Name)
				}
			})
		},
	}

	tags := &cobra.Command{
		Use:   "tags <owner>/<repo>",
		Short: "Tags (versions) distants",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			owner, repo, err := splitRepo(args[0])
			if err != nil {
				return err
			}
			t, err := svc.RepoTags(flagInstance, owner, repo)
			if err != nil {
				return err
			}
			return emit(cmd, t, func() {
				for _, x := range t {
					fmt.Fprintln(cmd.OutOrStdout(), x.Name)
				}
			})
		},
	}

	var ref string
	var lim int
	commits := &cobra.Command{
		Use:   "commits <owner>/<repo>",
		Short: "Commits commentés d'une réf",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			owner, repo, err := splitRepo(args[0])
			if err != nil {
				return err
			}
			cs, err := svc.RepoCommits(flagInstance, owner, repo, ref, lim)
			if err != nil {
				return err
			}
			return emit(cmd, cs, func() {
				for _, x := range cs {
					fmt.Fprintf(cmd.OutOrStdout(), "%s  %s\n", x.Short, x.Message)
				}
			})
		},
	}
	commits.Flags().StringVar(&ref, "ref", "", "branche/tag/sha")
	commits.Flags().IntVar(&lim, "limit", 20, "nombre de commits")

	var treeRef string
	tree := &cobra.Command{
		Use:   "tree <owner>/<repo> [chemin]",
		Short: "Arborescence distante",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			owner, repo, err := splitRepo(args[0])
			if err != nil {
				return err
			}
			path := ""
			if len(args) == 2 {
				path = args[1]
			}
			ents, err := svc.RepoContents(flagInstance, owner, repo, path, treeRef)
			if err != nil {
				return err
			}
			return emit(cmd, ents, func() {
				for _, e := range ents {
					fmt.Fprintf(cmd.OutOrStdout(), "%-4s %s\n", e.Type, e.Path)
				}
			})
		},
	}
	tree.Flags().StringVar(&treeRef, "ref", "", "branche/tag/sha")

	var fileRef string
	file := &cobra.Command{
		Use:   "file <owner>/<repo> <chemin>",
		Short: "Contenu d'un fichier distant",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			owner, repo, err := splitRepo(args[0])
			if err != nil {
				return err
			}
			content, err := svc.RepoFile(flagInstance, owner, repo, args[1], fileRef)
			if err != nil {
				return err
			}
			return emit(cmd, map[string]string{"path": args[1], "content": content}, func() {
				fmt.Fprint(cmd.OutOrStdout(), content)
			})
		},
	}
	file.Flags().StringVar(&fileRef, "ref", "", "branche/tag/sha")

	clone := &cobra.Command{
		Use:   "clone <owner>/<repo> <dossier>",
		Short: "Clone un dépôt distant en local",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			owner, repo, err := splitRepo(args[0])
			if err != nil {
				return err
			}
			out, err := svc.Clone(flagInstance, owner, repo, args[1])
			if err != nil {
				return err
			}
			return emit(cmd, map[string]string{"status": "ok", "dir": args[1]}, func() {
				fmt.Fprintf(cmd.OutOrStdout(), "Cloné dans %s\n%s", args[1], out)
			})
		},
	}

	push := &cobra.Command{
		Use:   "push <dossier>",
		Short: "Pousse la branche locale vers le distant (PIN demandé sur le poste)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Le PIN n'est jamais fourni ici : c'est le démon qui le demande dans une
			// fenêtre sur le poste (l'agent ne voit jamais le PIN passer).
			out, err := svc.Push(flagInstance, args[0])
			if err != nil {
				return err
			}
			return emit(cmd, map[string]string{"status": "ok"}, func() {
				fmt.Fprintf(cmd.OutOrStdout(), "Poussé.\n%s", out)
			})
		},
	}

	var syncMsg string
	sync := &cobra.Command{
		Use:   "sync <dossier>",
		Short: "Synchronise : enregistre, récupère, envoie (PIN demandé sur le poste)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := svc.Synchronize(flagInstance, args[0], syncMsg)
			if err != nil {
				return err
			}
			return emit(cmd, res, func() { fmt.Fprintln(cmd.OutOrStdout(), res.Summary) })
		},
	}
	sync.Flags().StringVar(&syncMsg, "message", "", "description de la version enregistrée (sinon libellé daté)")

	simple := func(use, short string, fn func(path string) (string, error)) *cobra.Command {
		return &cobra.Command{
			Use: use, Short: short, Args: cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				out, err := fn(args[0])
				if err != nil {
					return err
				}
				return emit(cmd, map[string]string{"status": "ok", "output": out}, func() {
					fmt.Fprint(cmd.OutOrStdout(), out)
					if out != "" {
						fmt.Fprintln(cmd.OutOrStdout())
					}
				})
			},
		}
	}
	fetch := simple("fetch <dossier>", "Vérifie les nouveautés du serveur", func(p string) (string, error) { return svc.Fetch(flagInstance, p) })
	pull := simple("pull <dossier>", "Récupère les nouveautés (fast-forward)", func(p string) (string, error) { return svc.Pull(flagInstance, p) })
	diff := simple("diff <dossier>", "Modifications non enregistrées", func(p string) (string, error) { return svc.Diff(p) })

	var msg string
	commit := &cobra.Command{
		Use: "commit <dossier>", Short: "Enregistre une version locale", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := svc.Commit(args[0], msg)
			if err != nil {
				return err
			}
			return emit(cmd, map[string]string{"status": "ok"}, func() { fmt.Fprintln(cmd.OutOrStdout(), out) })
		},
	}
	commit.Flags().StringVar(&msg, "message", "", "description de la version")
	_ = commit.MarkFlagRequired("message")

	var crDesc string
	var crPriv bool
	var crOrg string
	create := &cobra.Command{
		Use: "create <nom>", Short: "Crée un dépôt distant (PIN demandé sur le poste)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := svc.CreateRepo(flagInstance, crOrg, args[0], crDesc, crPriv)
			if err != nil {
				return err
			}
			return emit(cmd, r, func() { fmt.Fprintf(cmd.OutOrStdout(), "Dépôt créé : %s\n", r.FullName) })
		},
	}
	create.Flags().StringVar(&crDesc, "description", "", "description")
	create.Flags().BoolVar(&crPriv, "private", true, "dépôt privé")
	create.Flags().StringVar(&crOrg, "org", "", "organisation propriétaire (vide = espace personnel)")

	var sdDesc string
	setdesc := &cobra.Command{
		Use: "setdesc <owner>/<repo>", Short: "Modifie la description d'un dépôt distant", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			owner, repo, err := splitRepo(args[0])
			if err != nil {
				return err
			}
			if err := svc.SetRepoDescription(flagInstance, owner, repo, sdDesc); err != nil {
				return err
			}
			return emit(cmd, map[string]string{"status": "ok"}, func() { fmt.Fprintf(cmd.OutOrStdout(), "Description de %s mise à jour.\n", args[0]) })
		},
	}
	setdesc.Flags().StringVar(&sdDesc, "desc", "", "nouvelle description")
	_ = setdesc.MarkFlagRequired("desc")

	del := &cobra.Command{
		Use: "delete <owner>/<repo>", Short: "Supprime définitivement un dépôt distant (PIN demandé sur le poste)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			owner, repo, err := splitRepo(args[0])
			if err != nil {
				return err
			}
			if err := svc.DeleteRepo(flagInstance, owner, repo); err != nil {
				return err
			}
			return emit(cmd, map[string]string{"status": "ok"}, func() { fmt.Fprintf(cmd.OutOrStdout(), "Dépôt supprimé : %s\n", args[0]) })
		},
	}

	rename := &cobra.Command{Use: "rename <owner>/<repo> <nouveau-nom>", Short: "Renomme un dépôt (PIN demandé)", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, rp, err := splitRepo(args[0])
			if err != nil {
				return err
			}
			nr, err := svc.RenameRepo(flagInstance, o, rp, args[1])
			if err != nil {
				return err
			}
			return emit(cmd, nr, func() { fmt.Fprintf(cmd.OutOrStdout(), "Renommé : %s\n", nr.FullName) })
		}}
	var visPrivate bool
	visibility := &cobra.Command{Use: "visibility <owner>/<repo>", Short: "Change la visibilité (PIN demandé)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, rp, err := splitRepo(args[0])
			if err != nil {
				return err
			}
			nr, err := svc.SetRepoVisibility(flagInstance, o, rp, visPrivate)
			if err != nil {
				return err
			}
			return emit(cmd, nr, func() {
				fmt.Fprintf(cmd.OutOrStdout(), "%s est maintenant %s\n", nr.FullName, map[bool]string{true: "privé", false: "public"}[visPrivate])
			})
		}}
	visibility.Flags().BoolVar(&visPrivate, "private", true, "privé (true) ou public (false)")
	var archState bool
	archive := &cobra.Command{Use: "archive <owner>/<repo>", Short: "Archive/désarchive un dépôt (PIN demandé)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, rp, err := splitRepo(args[0])
			if err != nil {
				return err
			}
			nr, err := svc.ArchiveRepo(flagInstance, o, rp, archState)
			if err != nil {
				return err
			}
			return emit(cmd, nr, func() { fmt.Fprintf(cmd.OutOrStdout(), "%s archivé=%v\n", nr.FullName, archState) })
		}}
	archive.Flags().BoolVar(&archState, "archived", true, "archiver (true) ou désarchiver (false)")
	defbranch := &cobra.Command{Use: "default-branch <owner>/<repo> <branche>", Short: "Change la branche par défaut (PIN demandé)", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, rp, err := splitRepo(args[0])
			if err != nil {
				return err
			}
			nr, err := svc.SetDefaultBranch(flagInstance, o, rp, args[1])
			if err != nil {
				return err
			}
			return emit(cmd, nr, func() { fmt.Fprintf(cmd.OutOrStdout(), "Branche par défaut de %s : %s\n", nr.FullName, args[1]) })
		}}
	delBranch := &cobra.Command{Use: "delete-branch <owner>/<repo> <branche>", Short: "Supprime une branche distante (PIN demandé ; refuse la branche par défaut)", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, rp, err := splitRepo(args[0])
			if err != nil {
				return err
			}
			if err := svc.DeleteRemoteBranch(flagInstance, o, rp, args[1]); err != nil {
				return err
			}
			return emit(cmd, map[string]string{"status": "ok"}, func() { fmt.Fprintf(cmd.OutOrStdout(), "Branche distante %s supprimée\n", args[1]) })
		}}

	c.AddCommand(list, view, branches, tags, commits, tree, file, clone, push, sync, fetch, pull, diff, commit, create, setdesc, del, rename, visibility, archive, defbranch, delBranch)
	return c
}

func cmdIssue() *cobra.Command {
	c := &cobra.Command{Use: "issue", Short: "Issues d'un dépôt"}
	var state string
	list := &cobra.Command{
		Use: "list <owner>/<repo>", Short: "Liste les issues ouvertes", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, r, err := splitRepo(args[0])
			if err != nil {
				return err
			}
			items, err := svc.RepoIssues(flagInstance, o, r, state)
			if err != nil {
				return err
			}
			return emit(cmd, items, func() {
				for _, i := range items {
					fmt.Fprintf(cmd.OutOrStdout(), "#%d  %s\n", i.Number, i.Title)
				}
			})
		},
	}
	list.Flags().StringVar(&state, "state", "open", "open|closed|all")

	var title, body string
	create := &cobra.Command{
		Use: "create <owner>/<repo>", Short: "Crée une issue (PIN demandé sur le poste)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, r, err := splitRepo(args[0])
			if err != nil {
				return err
			}
			i, err := svc.CreateIssue(flagInstance, o, r, title, body)
			if err != nil {
				return err
			}
			return emit(cmd, i, func() { fmt.Fprintf(cmd.OutOrStdout(), "Issue #%d créée\n", i.Number) })
		},
	}
	create.Flags().StringVar(&title, "title", "", "titre")
	create.Flags().StringVar(&body, "body", "", "description")
	_ = create.MarkFlagRequired("title")

	var cmtBody string
	comment := &cobra.Command{
		Use: "comment <owner>/<repo> <numero>", Short: "Commente une issue/PR (PIN demandé)", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, r, n, err := splitRepoNum(args)
			if err != nil {
				return err
			}
			if err := svc.CommentIssue(flagInstance, o, r, n, cmtBody); err != nil {
				return err
			}
			return emit(cmd, map[string]string{"status": "ok"}, func() { fmt.Fprintf(cmd.OutOrStdout(), "Commentaire ajouté à #%d\n", n) })
		},
	}
	comment.Flags().StringVar(&cmtBody, "body", "", "contenu du commentaire")
	_ = comment.MarkFlagRequired("body")

	mkState := func(use, short, st string) *cobra.Command {
		return &cobra.Command{
			Use: use, Short: short, Args: cobra.ExactArgs(2),
			RunE: func(cmd *cobra.Command, args []string) error {
				o, r, n, err := splitRepoNum(args)
				if err != nil {
					return err
				}
				if err := svc.SetIssueState(flagInstance, o, r, n, st); err != nil {
					return err
				}
				return emit(cmd, map[string]string{"status": "ok"}, func() { fmt.Fprintf(cmd.OutOrStdout(), "Issue #%d → %s\n", n, st) })
			},
		}
	}
	closeCmd := mkState("close <owner>/<repo> <numero>", "Ferme une issue (PIN demandé)", "closed")
	reopen := mkState("reopen <owner>/<repo> <numero>", "Rouvre une issue (PIN demandé)", "open")

	var edTitle, edBody string
	edit := &cobra.Command{
		Use: "edit <owner>/<repo> <numero>", Short: "Modifie titre/corps d'une issue (PIN demandé)", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, r, n, err := splitRepoNum(args)
			if err != nil {
				return err
			}
			if err := svc.EditIssue(flagInstance, o, r, n, edTitle, edBody); err != nil {
				return err
			}
			return emit(cmd, map[string]string{"status": "ok"}, func() { fmt.Fprintf(cmd.OutOrStdout(), "Issue #%d modifiée\n", n) })
		},
	}
	edit.Flags().StringVar(&edTitle, "title", "", "nouveau titre")
	edit.Flags().StringVar(&edBody, "body", "", "nouveau corps")

	var lblNames []string
	labels := &cobra.Command{Use: "labels <owner>/<repo> <numero>", Short: "Remplace les étiquettes (par noms, PIN demandé)", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, r, n, err := splitRepoNum(args)
			if err != nil {
				return err
			}
			if err := svc.SetIssueLabels(flagInstance, o, r, n, lblNames); err != nil {
				return err
			}
			return emit(cmd, map[string]string{"status": "ok"}, func() { fmt.Fprintf(cmd.OutOrStdout(), "Étiquettes de #%d mises à jour\n", n) })
		}}
	labels.Flags().StringSliceVar(&lblNames, "label", nil, "nom d'étiquette (répétable)")
	var assignees []string
	assign := &cobra.Command{Use: "assign <owner>/<repo> <numero>", Short: "Remplace les assignés (par logins, PIN demandé)", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, r, n, err := splitRepoNum(args)
			if err != nil {
				return err
			}
			if err := svc.SetIssueAssignees(flagInstance, o, r, n, assignees); err != nil {
				return err
			}
			return emit(cmd, map[string]string{"status": "ok"}, func() { fmt.Fprintf(cmd.OutOrStdout(), "Assignés de #%d mis à jour\n", n) })
		}}
	assign.Flags().StringSliceVar(&assignees, "user", nil, "identifiant d'assigné (répétable)")
	var msID int64
	milestone := &cobra.Command{Use: "milestone <owner>/<repo> <numero>", Short: "Fixe le jalon (--id, 0 = retirer ; PIN demandé)", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, r, n, err := splitRepoNum(args)
			if err != nil {
				return err
			}
			if err := svc.SetIssueMilestone(flagInstance, o, r, n, msID); err != nil {
				return err
			}
			return emit(cmd, map[string]string{"status": "ok"}, func() { fmt.Fprintf(cmd.OutOrStdout(), "Jalon de #%d mis à jour\n", n) })
		}}
	milestone.Flags().Int64Var(&msID, "id", 0, "identifiant du jalon (0 = retirer)")
	labelList := &cobra.Command{Use: "label-list <owner>/<repo>", Short: "Liste les étiquettes du dépôt", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, r, err := splitRepo(args[0])
			if err != nil {
				return err
			}
			ls, err := svc.RepoLabels(flagInstance, o, r)
			if err != nil {
				return err
			}
			return emit(cmd, ls, func() {
				for _, l := range ls {
					fmt.Fprintf(cmd.OutOrStdout(), "%s\n", l.Name)
				}
			})
		}}
	milestoneList := &cobra.Command{Use: "milestone-list <owner>/<repo>", Short: "Liste les jalons du dépôt", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, r, err := splitRepo(args[0])
			if err != nil {
				return err
			}
			ms, err := svc.RepoMilestones(flagInstance, o, r)
			if err != nil {
				return err
			}
			return emit(cmd, ms, func() {
				for _, m := range ms {
					fmt.Fprintf(cmd.OutOrStdout(), "#%d %s [%s]\n", m.ID, m.Title, m.State)
				}
			})
		}}

	c.AddCommand(list, create, comment, closeCmd, reopen, edit, labels, assign, milestone, labelList, milestoneList)
	return c
}

func cmdPull() *cobra.Command {
	c := &cobra.Command{Use: "pull-request", Aliases: []string{"pr"}, Short: "Demandes de fusion (pull requests)"}
	var state string
	list := &cobra.Command{
		Use: "list <owner>/<repo>", Short: "Liste les demandes de fusion", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, r, err := splitRepo(args[0])
			if err != nil {
				return err
			}
			items, err := svc.RepoPulls(flagInstance, o, r, state)
			if err != nil {
				return err
			}
			return emit(cmd, items, func() {
				for _, i := range items {
					fmt.Fprintf(cmd.OutOrStdout(), "#%d  %s  [%s]\n", i.Number, i.Title, i.State)
				}
			})
		},
	}
	list.Flags().StringVar(&state, "state", "open", "open|closed|all")

	var prTitle, prBody, prHead, prBase string
	create := &cobra.Command{
		Use: "create <owner>/<repo>", Short: "Ouvre une demande de fusion (PIN demandé)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, r, err := splitRepo(args[0])
			if err != nil {
				return err
			}
			pr, err := svc.CreatePull(flagInstance, o, r, prTitle, prHead, prBase, prBody)
			if err != nil {
				return err
			}
			return emit(cmd, pr, func() { fmt.Fprintf(cmd.OutOrStdout(), "Demande de fusion #%d ouverte\n", pr.Number) })
		},
	}
	create.Flags().StringVar(&prTitle, "title", "", "titre")
	create.Flags().StringVar(&prHead, "head", "", "branche source")
	create.Flags().StringVar(&prBase, "base", "", "branche cible")
	create.Flags().StringVar(&prBody, "body", "", "description")
	_ = create.MarkFlagRequired("title")
	_ = create.MarkFlagRequired("head")
	_ = create.MarkFlagRequired("base")

	var mergeMethod string
	merge := &cobra.Command{
		Use: "merge <owner>/<repo> <numero>", Short: "Fusionne une demande de fusion (PIN demandé)", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, r, n, err := splitRepoNum(args)
			if err != nil {
				return err
			}
			if err := svc.MergePull(flagInstance, o, r, n, mergeMethod); err != nil {
				return err
			}
			return emit(cmd, map[string]string{"status": "ok"}, func() { fmt.Fprintf(cmd.OutOrStdout(), "Demande de fusion #%d fusionnée\n", n) })
		},
	}
	merge.Flags().StringVar(&mergeMethod, "method", "merge", "merge|rebase|squash")

	detail := &cobra.Command{Use: "detail <owner>/<repo> <numero>", Short: "Détail d'une PR", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, r, n, err := splitRepoNum(args)
			if err != nil {
				return err
			}
			pi, err := svc.PullDetail(flagInstance, o, r, n)
			if err != nil {
				return err
			}
			return emit(cmd, pi, func() {
				fmt.Fprintf(cmd.OutOrStdout(), "#%d %s [%s] %s→%s\n", pi.Number, pi.Title, pi.State, pi.Head.Ref, pi.Base.Ref)
			})
		}}
	diff := &cobra.Command{Use: "diff <owner>/<repo> <numero>", Short: "Diff d'une PR", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, r, n, err := splitRepoNum(args)
			if err != nil {
				return err
			}
			df, err := svc.PullDiff(flagInstance, o, r, n)
			if err != nil {
				return err
			}
			return emit(cmd, map[string]string{"diff": df}, func() { fmt.Fprint(cmd.OutOrStdout(), df) })
		}}
	files := &cobra.Command{Use: "files <owner>/<repo> <numero>", Short: "Fichiers modifiés d'une PR", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, r, n, err := splitRepoNum(args)
			if err != nil {
				return err
			}
			fl, err := svc.PullFiles(flagInstance, o, r, n)
			if err != nil {
				return err
			}
			return emit(cmd, fl, func() {
				for _, f := range fl {
					fmt.Fprintf(cmd.OutOrStdout(), "%s  (+%d -%d)\n", f.Filename, f.Additions, f.Deletions)
				}
			})
		}}
	var reviewEvent, reviewBody string
	review := &cobra.Command{Use: "review <owner>/<repo> <numero>", Short: "Revue d'une PR (PIN demandé)", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, r, n, err := splitRepoNum(args)
			if err != nil {
				return err
			}
			if err := svc.ReviewPull(flagInstance, o, r, n, reviewEvent, reviewBody); err != nil {
				return err
			}
			return emit(cmd, map[string]string{"status": "ok"}, func() { fmt.Fprintf(cmd.OutOrStdout(), "Revue (%s) déposée sur #%d\n", reviewEvent, n) })
		}}
	review.Flags().StringVar(&reviewEvent, "event", "COMMENT", "APPROVED|REQUEST_CHANGES|COMMENT")
	review.Flags().StringVar(&reviewBody, "body", "", "commentaire de revue")
	closePR := &cobra.Command{Use: "close <owner>/<repo> <numero>", Short: "Ferme une PR sans fusionner (PIN demandé)", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, r, n, err := splitRepoNum(args)
			if err != nil {
				return err
			}
			if err := svc.SetPullState(flagInstance, o, r, n, "closed"); err != nil {
				return err
			}
			return emit(cmd, map[string]string{"status": "ok"}, func() { fmt.Fprintf(cmd.OutOrStdout(), "PR #%d fermée\n", n) })
		}}
	var pedTitle, pedBody string
	pullEdit := &cobra.Command{Use: "edit <owner>/<repo> <numero>", Short: "Modifie titre/corps d'une PR (PIN demandé)", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, r, n, err := splitRepoNum(args)
			if err != nil {
				return err
			}
			if err := svc.EditPull(flagInstance, o, r, n, pedTitle, pedBody); err != nil {
				return err
			}
			return emit(cmd, map[string]string{"status": "ok"}, func() { fmt.Fprintf(cmd.OutOrStdout(), "PR #%d modifiée\n", n) })
		}}
	pullEdit.Flags().StringVar(&pedTitle, "title", "", "nouveau titre")
	pullEdit.Flags().StringVar(&pedBody, "body", "", "nouveau corps")

	c.AddCommand(list, create, merge, detail, diff, files, review, closePR, pullEdit)
	return c
}

func cmdRelease() *cobra.Command {
	c := &cobra.Command{Use: "release", Short: "Versions (releases) et tags"}
	var relName, relBody, relTarget string
	var relDraft, relPre bool
	list := &cobra.Command{Use: "list <owner>/<repo>", Short: "Liste les versions (id, tag, titre)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, r, err := splitRepo(args[0])
			if err != nil {
				return err
			}
			rels, err := svc.RepoReleases(flagInstance, o, r)
			if err != nil {
				return err
			}
			return emit(cmd, rels, func() {
				for _, rel := range rels {
					fmt.Fprintf(cmd.OutOrStdout(), "%d\t%s\t%s\n", rel.ID, rel.TagName, rel.Name)
				}
			})
		}}
	create := &cobra.Command{Use: "create <owner>/<repo> <tag>", Short: "Publie une version (PIN demandé)", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, r, err := splitRepo(args[0])
			if err != nil {
				return err
			}
			rel, err := svc.CreateRelease(flagInstance, o, r, args[1], relName, relBody, relTarget, relDraft, relPre)
			if err != nil {
				return err
			}
			return emit(cmd, rel, func() { fmt.Fprintf(cmd.OutOrStdout(), "Version publiée : %s\n", rel.TagName) })
		}}
	create.Flags().StringVar(&relName, "name", "", "titre")
	create.Flags().StringVar(&relBody, "body", "", "notes")
	create.Flags().StringVar(&relTarget, "target", "", "branche/commit cible")
	create.Flags().BoolVar(&relDraft, "draft", false, "brouillon")
	create.Flags().BoolVar(&relPre, "prerelease", false, "pré-version")
	del := &cobra.Command{Use: "delete <owner>/<repo> <id>", Short: "Supprime une version par id (PIN demandé)", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, r, err := splitRepo(args[0])
			if err != nil {
				return err
			}
			id, err := strconv.ParseInt(args[1], 10, 64)
			if err != nil {
				return fmt.Errorf("id invalide : %s", args[1])
			}
			if err := svc.DeleteRelease(flagInstance, o, r, id); err != nil {
				return err
			}
			return emit(cmd, map[string]string{"status": "ok"}, func() { fmt.Fprintf(cmd.OutOrStdout(), "Version %d supprimée\n", id) })
		}}
	var tagTarget string
	tagCreate := &cobra.Command{Use: "tag-create <owner>/<repo> <tag>", Short: "Crée un tag (PIN demandé)", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, r, err := splitRepo(args[0])
			if err != nil {
				return err
			}
			if err := svc.CreateTag(flagInstance, o, r, args[1], tagTarget); err != nil {
				return err
			}
			return emit(cmd, map[string]string{"status": "ok"}, func() { fmt.Fprintf(cmd.OutOrStdout(), "Tag %s créé\n", args[1]) })
		}}
	tagCreate.Flags().StringVar(&tagTarget, "target", "", "branche/commit cible")
	tagDel := &cobra.Command{Use: "tag-delete <owner>/<repo> <tag>", Short: "Supprime un tag (PIN demandé)", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, r, err := splitRepo(args[0])
			if err != nil {
				return err
			}
			if err := svc.DeleteTag(flagInstance, o, r, args[1]); err != nil {
				return err
			}
			return emit(cmd, map[string]string{"status": "ok"}, func() { fmt.Fprintf(cmd.OutOrStdout(), "Tag %s supprimé\n", args[1]) })
		}}
	c.AddCommand(list, create, del, tagCreate, tagDel)
	return c
}

func cmdOrg() *cobra.Command {
	c := &cobra.Command{Use: "org", Short: "Organisations"}
	c.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "Liste tes organisations",
		RunE: func(cmd *cobra.Command, _ []string) error {
			orgs, err := svc.Orgs(flagInstance)
			if err != nil {
				return err
			}
			return emit(cmd, orgs, func() {
				for _, o := range orgs {
					fmt.Fprintf(cmd.OutOrStdout(), "%-20s %s\n", o.Username, o.FullName)
				}
			})
		},
	})
	return c
}

func cmdToken() *cobra.Command {
	c := &cobra.Command{Use: "token", Short: "Gestion des tokens d'accès"}
	var lUser, lPass, lOTP string
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "Liste tes tokens d'accès (noms et portées ; identifiants requis)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Forgejo n'autorise pas la gestion des jetons PAR jeton : il faut une auth basique
			// (identifiant + mot de passe + 2FA au besoin), comme pour create/delete.
			p, err := secretOrPrompt(lPass, "le mot de passe (--password)", "Mot de passe pour "+lUser+" : ")
			if err != nil {
				return err
			}
			lPass = p
			if lOTP != "" {
				warnSecretFlag("le code 2FA (--otp)")
			}
			tokens, err := svc.Tokens(flagInstance, lUser, lPass, lOTP)
			if err != nil && lOTP == "" && looksLikeAuthFailure(err) && term.IsTerminal(int(os.Stdin.Fd())) {
				if o, e := promptSecret("Code 2FA (laisser vide si aucun) : "); e == nil && o != "" {
					lOTP = o
					tokens, err = svc.Tokens(flagInstance, lUser, lPass, lOTP)
				}
			}
			if err != nil {
				return err
			}
			return emit(cmd, tokens, func() {
				for _, t := range tokens {
					fmt.Fprintf(cmd.OutOrStdout(), "%-24s %s\n", t.Name, strings.Join(t.Scopes, ","))
				}
			})
		},
	}
	listCmd.Flags().StringVar(&lUser, "user", "", "identifiant du compte")
	listCmd.Flags().StringVar(&lPass, "password", "", "mot de passe du compte (sinon demandé, masqué)")
	listCmd.Flags().StringVar(&lOTP, "otp", "", "code 2FA (si le compte en a un)")
	c.AddCommand(listCmd)

	var user, pass, otp, name, scopes, repos string
	splitCSV := func(s string) []string {
		var out []string
		for _, x := range strings.Split(s, ",") {
			if x = strings.TrimSpace(x); x != "" {
				out = append(out, x)
			}
		}
		return out
	}
	create := &cobra.Command{
		Use: "create", Short: "Crée un token (auth basique user+mot de passe ; PIN demandé sur le poste)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := secretOrPrompt(pass, "le mot de passe (--password)", "Mot de passe pour "+user+" : ")
			if err != nil {
				return err
			}
			pass = p
			if otp != "" {
				warnSecretFlag("le code 2FA (--otp)")
			}
			tok, err := svc.TokenCreate(flagInstance, user, pass, otp, name, splitCSV(scopes), splitCSV(repos))
			// Compte avec 2FA : réessayer en demandant le code (masqué), seulement sur un refus
			// d'authentification et au terminal — pour ne jamais exiger --otp en clair.
			if err != nil && otp == "" && looksLikeAuthFailure(err) && term.IsTerminal(int(os.Stdin.Fd())) {
				if o, e := promptSecret("Code 2FA (laisser vide si aucun) : "); e == nil && o != "" {
					otp = o
					tok, err = svc.TokenCreate(flagInstance, user, pass, otp, name, splitCSV(scopes), splitCSV(repos))
				}
			}
			if err != nil {
				return err
			}
			return emit(cmd, map[string]string{"token": tok}, func() { fmt.Fprintln(cmd.OutOrStdout(), tok) })
		},
	}
	create.Flags().StringVar(&user, "user", "", "identifiant du compte")
	create.Flags().StringVar(&pass, "password", "", "mot de passe du compte (sinon demandé, masqué)")
	create.Flags().StringVar(&name, "name", "situlab", "nom du token")
	create.Flags().StringVar(&scopes, "scopes", "read:repository,write:repository", "portées (séparées par des virgules)")
	create.Flags().StringVar(&repos, "repos", "", "restreindre à ces dépôts owner/repo (virgules) ; vide = selon les droits")
	create.Flags().StringVar(&otp, "otp", "", "code 2FA (si le compte en a un)")

	var dUser, dPass, dOTP string
	del := &cobra.Command{
		Use: "delete <id>", Short: "Supprime un token par id (auth basique ; PIN demandé sur le poste)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("id invalide")
			}
			p, err := secretOrPrompt(dPass, "le mot de passe (--password)", "Mot de passe pour "+dUser+" : ")
			if err != nil {
				return err
			}
			dPass = p
			if dOTP != "" {
				warnSecretFlag("le code 2FA (--otp)")
			}
			err = svc.TokenDelete(flagInstance, dUser, dPass, dOTP, id)
			if err != nil && dOTP == "" && looksLikeAuthFailure(err) && term.IsTerminal(int(os.Stdin.Fd())) {
				if o, e := promptSecret("Code 2FA (laisser vide si aucun) : "); e == nil && o != "" {
					dOTP = o
					err = svc.TokenDelete(flagInstance, dUser, dPass, dOTP, id)
				}
			}
			if err != nil {
				return err
			}
			return emit(cmd, map[string]string{"status": "ok"}, func() { fmt.Fprintln(cmd.OutOrStdout(), "Token supprimé.") })
		},
	}
	del.Flags().StringVar(&dUser, "user", "", "identifiant du compte")
	del.Flags().StringVar(&dPass, "password", "", "mot de passe du compte (sinon demandé, masqué)")
	del.Flags().StringVar(&dOTP, "otp", "", "code 2FA (si le compte en a un)")

	c.AddCommand(create, del)
	return c
}

func cmdLocal() *cobra.Command {
	c := &cobra.Command{Use: "local", Short: "Dépôts git locaux"}

	add := &cobra.Command{
		Use:   "add <chemin>",
		Short: "Enregistre un dépôt local à suivre",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := svc.AddLocalRepo(args[0]); err != nil {
				return err
			}
			return emit(cmd, map[string]string{"status": "ok"}, func() {
				fmt.Fprintf(cmd.OutOrStdout(), "Dépôt local suivi : %s\n", args[0])
			})
		},
	}

	list := &cobra.Command{
		Use:   "list",
		Short: "Liste les dépôts locaux suivis et leur état",
		RunE: func(cmd *cobra.Command, _ []string) error {
			repos, err := svc.LocalRepos()
			if err != nil {
				return err
			}
			return emit(cmd, repos, func() {
				out := cmd.OutOrStdout()
				for _, st := range repos {
					state := map[bool]string{true: "propre", false: "modifié"}[st.Clean]
					fmt.Fprintf(out, "%-40s %-10s %s (+%d/-%d)\n", st.Path, st.Branch, state, st.Ahead, st.Behind)
				}
			})
		},
	}

	status := &cobra.Command{
		Use:   "status <chemin>",
		Short: "Statut git détaillé d'un dossier",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := svc.LocalStatus(args[0])
			if err != nil {
				return err
			}
			return emit(cmd, st, func() {
				out := cmd.OutOrStdout()
				if !st.IsRepo {
					fmt.Fprintln(out, "Pas un dépôt git.")
					return
				}
				fmt.Fprintf(out, "branche : %s\nsuivi   : %s\navance  : +%d / -%d\nétat    : %s\nremote  : %s\n",
					st.Branch, st.Upstream, st.Ahead, st.Behind,
					map[bool]string{true: "propre", false: "modifié"}[st.Clean], st.Remote)
				for _, f := range st.Changed {
					fmt.Fprintf(out, "  ~ %s\n", f)
				}
			})
		},
	}

	remove := &cobra.Command{
		Use: "remove <chemin>", Short: "Cesse de suivre un dépôt local (ne supprime rien)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := svc.RemoveLocalRepo(args[0]); err != nil {
				return err
			}
			return emit(cmd, map[string]string{"status": "ok"}, func() { fmt.Fprintln(cmd.OutOrStdout(), "Dépôt retiré du suivi.") })
		},
	}

	branch := &cobra.Command{Use: "branch <chemin> <nom>", Short: "Crée une branche locale et bascule dessus", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := svc.GitCreateBranch(args[0], args[1])
			if err != nil {
				return err
			}
			return emit(cmd, map[string]string{"output": out}, func() { fmt.Fprintf(cmd.OutOrStdout(), "Branche « %s » créée.\n", args[1]) })
		}}
	switchCmd := &cobra.Command{Use: "switch <chemin> <branche>", Short: "Bascule sur une branche existante", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := svc.GitSwitchBranch(args[0], args[1])
			if err != nil {
				return err
			}
			return emit(cmd, map[string]string{"output": out}, func() { fmt.Fprintf(cmd.OutOrStdout(), "Sur la branche « %s ».\n", args[1]) })
		}}
	var discFile string
	var discConfirm bool
	discard := &cobra.Command{Use: "discard <chemin>", Short: "Annule les modifications locales non enregistrées (DESTRUCTIF)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !discConfirm {
				return fmt.Errorf("action destructive : relancez avec --confirm pour annuler les modifications (fichier : %s)", valOr(discFile, "tout le dépôt"))
			}
			out, err := svc.GitDiscard(args[0], discFile)
			if err != nil {
				return err
			}
			return emit(cmd, map[string]string{"output": out}, func() { fmt.Fprintln(cmd.OutOrStdout(), "Modifications locales annulées.") })
		}}
	discard.Flags().StringVar(&discFile, "file", "", "fichier précis (défaut : tout le dépôt)")
	discard.Flags().BoolVar(&discConfirm, "confirm", false, "confirme l'annulation destructive")
	var logLimit int
	logCmd := &cobra.Command{Use: "log <chemin>", Short: "Historique local récent", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			commits, err := svc.GitLog(args[0], logLimit)
			if err != nil {
				return err
			}
			return emit(cmd, commits, func() {
				for _, c := range commits {
					fmt.Fprintf(cmd.OutOrStdout(), "%s  %s  (%s)\n", c.Short, c.Message, c.Author)
				}
			})
		}}
	logCmd.Flags().IntVar(&logLimit, "limit", 30, "nombre de commits")
	var stashMsg string
	stashSave := &cobra.Command{Use: "stash-save <chemin>", Short: "Met de côté les modifications locales", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := svc.GitStashSave(args[0], stashMsg)
			if err != nil {
				return err
			}
			return emit(cmd, map[string]string{"output": out}, func() { fmt.Fprintln(cmd.OutOrStdout(), "Modifications mises de côté.") })
		}}
	stashSave.Flags().StringVar(&stashMsg, "message", "", "libellé de la remise de côté")
	stashList := &cobra.Command{Use: "stash-list <chemin>", Short: "Liste les remises de côté", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := svc.GitStashList(args[0])
			if err != nil {
				return err
			}
			return emit(cmd, map[string]string{"output": out}, func() { fmt.Fprint(cmd.OutOrStdout(), out) })
		}}
	stashPop := &cobra.Command{Use: "stash-pop <chemin>", Short: "Réapplique la dernière remise de côté", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := svc.GitStashPop(args[0])
			if err != nil {
				return err
			}
			return emit(cmd, map[string]string{"output": out}, func() { fmt.Fprintln(cmd.OutOrStdout(), "Remise de côté réappliquée.") })
		}}

	// --- Git « complet » (v1.6) : gestes de dev avancés. out1/out2 fabriquent une commande
	//     qui relaie à une fonction du cœur et affiche sa sortie git brute. ---
	out1 := func(use, short string, fn func(path string) (string, error)) *cobra.Command {
		return &cobra.Command{Use: use, Short: short, Args: cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				out, err := fn(args[0])
				if err != nil {
					return err
				}
				return emit(cmd, map[string]string{"status": "ok", "output": out}, func() { fmt.Fprint(cmd.OutOrStdout(), out) })
			}}
	}
	out2 := func(use, short string, fn func(path, arg string) (string, error)) *cobra.Command {
		return &cobra.Command{Use: use, Short: short, Args: cobra.ExactArgs(2),
			RunE: func(cmd *cobra.Command, args []string) error {
				out, err := fn(args[0], args[1])
				if err != nil {
					return err
				}
				return emit(cmd, map[string]string{"status": "ok", "output": out}, func() { fmt.Fprint(cmd.OutOrStdout(), out) })
			}}
	}

	merge := out2("merge <chemin> <ref>", "Fusionne une branche/réf dans la branche courante (local)", svc.GitMerge)
	mergeAbort := out1("merge-abort <chemin>", "Annule une fusion en cours", svc.GitMergeAbort)
	rebase := out2("rebase <chemin> <upstream>", "Rejoue la branche courante au sommet d'une autre (local)", svc.GitRebase)
	rebaseCont := out1("rebase-continue <chemin>", "Poursuit un rebase après résolution des conflits", svc.GitRebaseContinue)
	rebaseAbort := out1("rebase-abort <chemin>", "Annule un rebase en cours", svc.GitRebaseAbort)
	cherry := out2("cherry-pick <chemin> <commit>", "Applique un commit précis sur la branche courante", svc.GitCherryPick)
	cherryCont := out1("cherry-pick-continue <chemin>", "Poursuit un cherry-pick après résolution", svc.GitCherryPickContinue)
	cherryAbort := out1("cherry-pick-abort <chemin>", "Annule un cherry-pick en cours", svc.GitCherryPickAbort)
	revert := out2("revert <chemin> <commit>", "Ajoute un commit qui annule un commit précis (non destructif)", svc.GitRevert)

	var resetMode string
	var resetConfirm bool
	reset := &cobra.Command{Use: "reset <chemin> <ref>", Short: "Déplace HEAD vers une réf (--mode soft|mixed|hard)", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if resetMode == "hard" && !resetConfirm {
				return fmt.Errorf("action destructive : « reset --mode hard » perd les modifications non enregistrées — relancez avec --confirm")
			}
			out, err := svc.GitReset(args[0], resetMode, args[1])
			if err != nil {
				return err
			}
			return emit(cmd, map[string]string{"status": "ok", "output": out}, func() { fmt.Fprint(cmd.OutOrStdout(), out) })
		}}
	reset.Flags().StringVar(&resetMode, "mode", "mixed", "soft|mixed|hard|keep|merge")
	reset.Flags().BoolVar(&resetConfirm, "confirm", false, "confirme un reset --mode hard (destructif)")

	var amendMsg string
	amend := &cobra.Command{Use: "amend <chemin>", Short: "Réécrit le dernier commit (intègre l'arbre ; --message pour changer le message)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := svc.GitAmend(args[0], amendMsg)
			if err != nil {
				return err
			}
			return emit(cmd, map[string]string{"status": "ok", "output": out}, func() { fmt.Fprint(cmd.OutOrStdout(), out) })
		}}
	amend.Flags().StringVar(&amendMsg, "message", "", "nouveau message (vide = conserver l'actuel)")

	var cpMsg string
	var cpPaths []string
	commitPaths := &cobra.Command{Use: "commit-paths <chemin>", Short: "Enregistre uniquement les chemins indiqués (staging sélectif)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := svc.GitCommitPaths(args[0], cpMsg, cpPaths)
			if err != nil {
				return err
			}
			return emit(cmd, map[string]string{"status": "ok", "output": out}, func() { fmt.Fprint(cmd.OutOrStdout(), out) })
		}}
	commitPaths.Flags().StringVar(&cpMsg, "message", "", "description de la version")
	commitPaths.Flags().StringSliceVar(&cpPaths, "path", nil, "chemin à enregistrer (répétable)")
	_ = commitPaths.MarkFlagRequired("message")
	_ = commitPaths.MarkFlagRequired("path")

	var tagMsg string
	tagLocal := &cobra.Command{Use: "tag-create <chemin> <nom>", Short: "Crée un tag LOCAL (annoté si --message ; publier avec push-tag)", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := svc.GitTagCreate(args[0], args[1], tagMsg)
			if err != nil {
				return err
			}
			return emit(cmd, map[string]string{"status": "ok", "output": out}, func() { fmt.Fprintf(cmd.OutOrStdout(), "Tag local « %s » créé.\n%s", args[1], out) })
		}}
	tagLocal.Flags().StringVar(&tagMsg, "message", "", "message d'annotation (vide = tag léger)")

	pushForce := &cobra.Command{Use: "push-force <chemin>", Short: "Pousse en force sûr (--force-with-lease) après rebase/amend (PIN demandé)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := svc.GitPushForce(flagInstance, args[0])
			if err != nil {
				return err
			}
			return emit(cmd, map[string]string{"status": "ok", "output": out}, func() { fmt.Fprintf(cmd.OutOrStdout(), "Branche mise à jour en ligne (force sûr).\n%s", out) })
		}}
	pushTag := &cobra.Command{Use: "push-tag <chemin> <tag>", Short: "Publie un tag vers le serveur (PIN demandé)", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := svc.GitPushTag(flagInstance, args[0], args[1])
			if err != nil {
				return err
			}
			return emit(cmd, map[string]string{"status": "ok", "output": out}, func() { fmt.Fprintf(cmd.OutOrStdout(), "Tag « %s » publié.\n%s", args[1], out) })
		}}

	// Suppression de branche locale (v1.9.8) — git refuse une branche non fusionnée sans --force.
	var brDelForce bool
	branchDelete := &cobra.Command{Use: "branch-delete <chemin> <branche>", Short: "Supprime une branche locale (git refuse une branche non fusionnée sans --force)", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := svc.GitDeleteBranch(args[0], args[1], brDelForce)
			if err != nil {
				return err
			}
			return emit(cmd, map[string]string{"status": "ok", "output": out}, func() { fmt.Fprintf(cmd.OutOrStdout(), "Branche « %s » supprimée.\n%s", args[1], out) })
		}}
	branchDelete.Flags().BoolVar(&brDelForce, "force", false, "supprimer même une branche non fusionnée (perte de travail possible)")

	// Worktrees (v1.9.8) — plusieurs arbres de travail depuis un seul dépôt (sessions parallèles).
	wtList := &cobra.Command{Use: "worktree-list <chemin>", Short: "Liste les arbres de travail (worktrees) du dépôt", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			wts, err := svc.GitWorktrees(args[0])
			if err != nil {
				return err
			}
			return emit(cmd, wts, func() {
				for _, w := range wts {
					b := w.Branch
					if b == "" {
						b = "(détaché)"
					}
					main := ""
					if w.Main {
						main = "  [principal]"
					}
					fmt.Fprintf(cmd.OutOrStdout(), "%s  %s%s\n", w.Path, b, main)
				}
			})
		}}
	var wtNew bool
	wtAdd := &cobra.Command{Use: "worktree-add <chemin> <dossier-cible> <branche>", Short: "Ajoute un worktree (--new crée une nouvelle branche ; sinon extrait l'existante)", Args: cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := svc.GitWorktreeAdd(args[0], args[1], args[2], wtNew)
			if err != nil {
				return err
			}
			return emit(cmd, map[string]string{"status": "ok", "output": out}, func() {
				fmt.Fprintf(cmd.OutOrStdout(), "Worktree « %s » ajouté dans %s.\n%s", args[2], args[1], out)
			})
		}}
	wtAdd.Flags().BoolVar(&wtNew, "new", false, "créer une NOUVELLE branche (sinon extraire une branche existante)")
	var wtRmForce bool
	wtRemove := &cobra.Command{Use: "worktree-remove <chemin> <dossier-cible>", Short: "Retire un worktree (son dossier est supprimé ; --force si modifications)", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := svc.GitWorktreeRemove(args[0], args[1], wtRmForce)
			if err != nil {
				return err
			}
			return emit(cmd, map[string]string{"status": "ok", "output": out}, func() { fmt.Fprintf(cmd.OutOrStdout(), "Worktree retiré : %s\n%s", args[1], out) })
		}}
	wtRemove.Flags().BoolVar(&wtRmForce, "force", false, "retirer malgré des modifications non enregistrées")
	wtPrune := out1("worktree-prune <chemin>", "Nettoie les références de worktrees dont le dossier a disparu", svc.GitWorktreePrune)

	c.AddCommand(add, list, status, remove, branch, switchCmd, branchDelete, discard, logCmd, stashSave, stashList, stashPop,
		merge, mergeAbort, rebase, rebaseCont, rebaseAbort, cherry, cherryCont, cherryAbort, revert, reset,
		amend, commitPaths, tagLocal, pushForce, pushTag, wtList, wtAdd, wtRemove, wtPrune)
	return c
}

// valOr renvoie v s'il est non vide, sinon def.
func valOr(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

// cmdProject expose les projets (liens atelier↔production) et leur équilibre de versions. Lecture
// seule en CLI/MCP : la création d'un lien se fait dans la fenêtre (choix humain de classification).
func cmdProject() *cobra.Command {
	c := &cobra.Command{Use: "project", Aliases: []string{"projet"}, Short: "Projets : liens atelier↔production et équilibre des versions"}
	list := &cobra.Command{
		Use: "list", Short: "Liste les projets (atelier + productions)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ps, err := svc.Projects()
			if err != nil {
				return err
			}
			return emit(cmd, ps, func() {
				out := cmd.OutOrStdout()
				if len(ps) == 0 {
					fmt.Fprintln(out, "Aucun projet. Créez un lien atelier↔production dans la fenêtre SituLab.")
					return
				}
				for _, p := range ps {
					fmt.Fprintf(out, "%-20s atelier=%s/%s/%s  productions=%d\n", p.Name, p.Atelier.Instance, p.Atelier.Owner, p.Atelier.Repo, len(p.Productions))
				}
			})
		},
	}
	status := &cobra.Command{
		Use: "status <nom>", Short: "Équilibre d'un projet : version atelier vs productions", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			v, err := svc.ProjectStatus(args[0])
			if err != nil {
				return err
			}
			return emit(cmd, v, func() {
				out := cmd.OutOrStdout()
				av := v.Atelier.Version
				if av == "" {
					av = "(aucune version)"
				}
				fmt.Fprintf(out, "Atelier %s/%s : %s\n", v.Atelier.Owner, v.Atelier.Repo, av)
				for _, p := range v.Prods {
					switch {
					case p.Error != "":
						fmt.Fprintf(out, "  ! %s/%s : %s\n", p.Owner, p.Repo, p.Error)
					case p.Balanced:
						fmt.Fprintf(out, "  ✓ %s/%s : %s — équilibré\n", p.Owner, p.Repo, p.Version)
					default:
						pv := p.Version
						if pv == "" {
							pv = "(aucune version)"
						}
						fmt.Fprintf(out, "  ↓ %s/%s : %s — en retard de %d version(s)\n", p.Owner, p.Repo, pv, p.Behind)
					}
				}
			})
		},
	}
	var promIdx int
	var promSrc, promVer, promDst, promMode, promNotes string
	promote := &cobra.Command{
		Use: "promote <nom>", Short: "Publie une version choisie de l'atelier vers une production (--prod ; --from/--version/--to ; --mode ; PIN sur le poste)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := svc.ProjectPromote(args[0], promIdx, promSrc, promVer, promDst, promMode, promNotes)
			if err != nil {
				return err
			}
			return emit(cmd, res, func() { fmt.Fprintln(cmd.OutOrStdout(), res.Summary) })
		},
	}
	promote.Flags().IntVar(&promIdx, "prod", 0, "index de la production dans le projet (0 = la première)")
	promote.Flags().StringVar(&promSrc, "from", "", "branche de l'atelier à promouvoir (défaut : branche par défaut)")
	promote.Flags().StringVar(&promVer, "version", "", "version/tag exact à publier (défaut : dernier tag de la branche)")
	promote.Flags().StringVar(&promDst, "to", "", "branche de la production cible (défaut : branche par défaut)")
	promote.Flags().StringVar(&promMode, "mode", "tag", "méthode : code (sans tag) | tag (code+tag, déclenche la CI) | release (code+tag+Release)")
	promote.Flags().StringVar(&promNotes, "notes", "", "notes de version (mode release)")
	c.AddCommand(list, status, promote)
	return c
}

// cmdAttest : signature de garantie « SituLab approved ». status (lecture) ; genkey/install-cert (setup).
func cmdAttest() *cobra.Command {
	c := &cobra.Command{Use: "attest", Short: "Signature de garantie « SituLab approved » des commits"}
	var limit int
	status := &cobra.Command{Use: "status <dossier>", Short: "État d'attestation des derniers commits d'un dépôt local", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rows, err := svc.AttestHistory(args[0], limit)
			if err != nil {
				return err
			}
			return emit(cmd, rows, func() {
				if attest.UsingTestRoot() {
					fmt.Fprintln(cmd.OutOrStdout(), "⚠️  racine de signature = racine de TEST (forgeable) — à remplacer par la vraie racine de gouvernance.")
				}
				for _, r := range rows {
					mark := "⬜"
					who := ""
					if r.Attestation.Verified {
						mark, who = "✅", " — par "+r.Attestation.Subject
					}
					fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s%s\n", mark, r.Short, r.Message, who)
				}
			})
		}}
	status.Flags().IntVar(&limit, "limit", 20, "nombre de commits")
	genkey := &cobra.Command{Use: "genkey", Short: "Génère la clé d'appareil et affiche sa clé publique à faire certifier (--instance)", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			pub, err := svc.GenerateSignKey(flagInstance)
			if err != nil {
				return err
			}
			return emit(cmd, map[string]string{"public_key": pub}, func() {
				fmt.Fprintf(cmd.OutOrStdout(), "Clé publique d'appareil (à faire certifier par la gouvernance) :\n%s\n", pub)
			})
		}}
	installCert := &cobra.Command{Use: "install-cert <fichier-cert.json>", Short: "Installe le certificat d'appareil émis par la gouvernance (--instance)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := os.ReadFile(args[0])
			if err != nil {
				return err
			}
			if err := svc.InstallSignCert(flagInstance, string(b)); err != nil {
				return err
			}
			return emit(cmd, map[string]string{"status": "ok"}, func() { fmt.Fprintln(cmd.OutOrStdout(), "Certificat d'appareil installé.") })
		}}
	c.AddCommand(status, genkey, installCert)
	c.AddCommand(cmdAttestEmit()...) // gen-root / gen-ca / issue (gouvernance, hors ligne)
	return c
}

func cmdPIN() *cobra.Command {
	c := &cobra.Command{Use: "pin", Short: "PIN utilisateur (verrouille les push)"}
	c.AddCommand(&cobra.Command{
		Use:   "set",
		Short: "Indique où définir/changer le PIN (jamais par la ligne de commande)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Le PIN ne transite jamais par le socket : sa définition est une action humaine,
			// faite dans la fenêtre SituLab. On n'en demande pas la saisie ici.
			fmt.Fprintln(cmd.OutOrStdout(), "Le PIN se définit dans la fenêtre SituLab : Réglages → Sécurité → « Définir / Changer le PIN ».")
			fmt.Fprintln(cmd.OutOrStdout(), "Par sécurité, il ne peut pas être saisi en ligne de commande.")
			return nil
		},
	})
	return c
}

func cmdUnlock() *cobra.Command {
	return &cobra.Command{
		Use:   "unlock",
		Short: "Déverrouille la session (le PIN est demandé dans une fenêtre sur le poste)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Le PIN ne transite pas par le terminal/socket : le démon ouvre sa fenêtre de
			// saisie sur le poste. La commande rend la main une fois la session ouverte (ou
			// une erreur si l'humain refuse / n'est pas là).
			if err := svc.Unlock(); err != nil {
				return err
			}
			return emit(cmd, map[string]any{"status": "ok", "unlocked": true}, func() {
				fmt.Fprintln(cmd.OutOrStdout(), "Session déverrouillée.")
			})
		},
	}
}

func cmdLock() *cobra.Command {
	return &cobra.Command{
		Use:   "lock",
		Short: "Verrouille la session (oublie la clé en mémoire)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := svc.Lock(); err != nil {
				return err
			}
			return emit(cmd, map[string]any{"status": "ok", "unlocked": false}, func() {
				fmt.Fprintln(cmd.OutOrStdout(), "Session verrouillée.")
			})
		},
	}
}

func cmdStatus() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "État de la session (verrouillée/déverrouillée, PIN, instances)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := svc.Status()
			if err != nil {
				return err
			}
			return emit(cmd, s, func() {
				out := cmd.OutOrStdout()
				fmt.Fprintf(out, "session   : %s\n", map[bool]string{true: "déverrouillée", false: "verrouillée"}[s.Unlocked])
				fmt.Fprintf(out, "PIN       : %s\n", map[bool]string{true: "défini", false: "non défini"}[s.PINSet])
				fmt.Fprintf(out, "instances : %d\n", s.Instances)
			})
		},
	}
}

func cmdAdmin() *cobra.Command {
	c := &cobra.Command{Use: "admin", Short: "Administration (utilisateurs, tokens)"}

	userList := &cobra.Command{
		Use: "users", Short: "Liste les utilisateurs",
		RunE: func(cmd *cobra.Command, _ []string) error {
			us, err := svc.AdminUsers(flagInstance)
			if err != nil {
				return err
			}
			return emit(cmd, us, func() {
				for _, u := range us {
					fmt.Fprintf(cmd.OutOrStdout(), "%-20s %s\n", u.Login, u.Email)
				}
			})
		},
	}

	directory := &cobra.Command{
		Use: "directory", Short: "Annuaire consolidé : les comptes de toutes les instances Forgejo",
		RunE: func(cmd *cobra.Command, _ []string) error {
			d, err := svc.ConsolidatedDirectory()
			if err != nil {
				return err
			}
			return emit(cmd, d, func() {
				for _, in := range d.Instances {
					if !in.OK {
						fmt.Fprintf(cmd.OutOrStdout(), "! %s : %s\n", in.Name, in.Error)
					}
				}
				for _, e := range d.Entries {
					fmt.Fprintf(cmd.OutOrStdout(), "%-20s %-28s %v\n", e.Login, e.Email, e.Presence)
				}
			})
		},
	}

	var email, password string
	var admin bool
	userCreate := &cobra.Command{
		Use: "user-create <identifiant>", Short: "Crée un utilisateur (PIN demandé sur le poste)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := secretOrPrompt(password, "le mot de passe initial (--password)", "Mot de passe initial pour "+args[0]+" : ")
			if err != nil {
				return err
			}
			password = p
			u, err := svc.AdminCreateUser(flagInstance, args[0], email, password, admin)
			if err != nil {
				return err
			}
			return emit(cmd, u, func() { fmt.Fprintf(cmd.OutOrStdout(), "Utilisateur créé : %s\n", u.Login) })
		},
	}
	userCreate.Flags().StringVar(&email, "email", "", "e-mail")
	userCreate.Flags().StringVar(&password, "password", "", "mot de passe initial (sinon demandé, masqué)")
	userCreate.Flags().BoolVar(&admin, "admin", false, "administrateur")

	c.AddCommand(userList, directory, userCreate)
	return c
}

// looksLikeAuthFailure indique si l'erreur ressemble à un refus d'authentification Forgejo
// (identifiant/mot de passe/2FA), auquel cas proposer un code 2FA a du sens. Les erreurs
// locales (doublon, session verrouillée, URL invalide, champ manquant) ne matchent pas.
func looksLikeAuthFailure(err error) bool {
	m := strings.ToLower(err.Error())
	return strings.Contains(m, "mot de passe") || strings.Contains(m, "2fa")
}

func splitRepo(s string) (string, string, error) {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", errors.New("format attendu : owner/repo")
	}
	return parts[0], parts[1], nil
}

// splitRepoNum lit les deux arguments communs « owner/repo numéro » des commandes issue/PR. Factorise
// le bloc splitRepo + Atoi recopié sur chaque sous-commande ExactArgs(2).
func splitRepoNum(args []string) (owner, repo string, num int, err error) {
	owner, repo, err = splitRepo(args[0])
	if err != nil {
		return "", "", 0, err
	}
	num, err = strconv.Atoi(args[1])
	if err != nil {
		return "", "", 0, fmt.Errorf("numéro invalide : %s", args[1])
	}
	return owner, repo, num, nil
}

// warnSecretFlag avertit (stderr) qu'un secret passé en OPTION de ligne de commande reste visible dans
// la liste des processus (ps / Gestionnaire des tâches) et l'historique du shell — à éviter.
func warnSecretFlag(what string) {
	fmt.Fprintf(os.Stderr, "Attention : %s passé en option reste visible dans la liste des processus et l'historique du shell ; laissez l'option vide pour une saisie masquée.\n", what)
}

// secretOrPrompt renvoie le secret fourni par drapeau (en avertissant qu'il est exposé), sinon le
// demande au terminal, masqué. Évite d'imposer un secret sur la ligne de commande.
func secretOrPrompt(flagVal, what, label string) (string, error) {
	if flagVal != "" {
		warnSecretFlag(what)
		return flagVal, nil
	}
	return promptSecret(label)
}

func promptSecret(label string) (string, error) {
	fmt.Fprint(os.Stderr, label)
	if term.IsTerminal(int(os.Stdin.Fd())) {
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(b)), nil
	}
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}
