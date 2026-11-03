// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package cli

import (
	"fmt"
	"os"
	"situlab/internal/attest"

	"github.com/spf13/cobra"
)

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
					fmt.Fprintln(cmd.OutOrStdout(), "⚠️  racine de signature = racine de TEST (forgeable), à remplacer par la vraie racine de gouvernance.")
				}
				for _, r := range rows {
					mark := "⬜"
					who := ""
					if r.Attestation.Verified {
						mark, who = "✅", " · par "+r.Attestation.Subject
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
