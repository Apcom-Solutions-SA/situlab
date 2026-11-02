// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package cli

import (
	"errors"
	"fmt"
	"situlab/internal/core"
	"situlab/internal/ipc"

	"github.com/spf13/cobra"
)

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
