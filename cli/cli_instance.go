// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

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
				fmt.Fprintf(cmd.OutOrStdout(), "Connexion OK : %s (%s)\n", u.Login, u.FullName)
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
