// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// cmdSpoke : les projections de mission (SPOKES), dérivées d'un KERNEL. Lecture + gestes mémoire.
// La DÉSIGNATION (ajout/retrait) reste réservée à la FENÊTRE — absente de la CLI.
func cmdSpoke() *cobra.Command {
	c := &cobra.Command{Use: "spoke", Short: "Projections de mission (SPOKES) : état, contenu, récupérer/enregistrer"}

	list := &cobra.Command{
		Use: "list", Short: "Liste les spokes désignés (avec leur KERNEL parent)", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			sp, err := svc.Spokes()
			if err != nil {
				return err
			}
			return emit(cmd, sp, func() {
				if len(sp) == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "Aucun spoke désigné (l'humain les désigne dans la fenêtre).")
					return
				}
				for _, s := range sp {
					k := s.Kernel
					if k == "" {
						k = "—"
					}
					run := ""
					if s.RunFile != "" {
						run = "  [run: " + s.RunFile + "]"
					}
					fmt.Fprintf(cmd.OutOrStdout(), "%s [kernel: %s] — %s%s\n", s.Name, k, s.LocalPath, run)
					if s.Mission != "" {
						fmt.Fprintf(cmd.OutOrStdout(), "    mission : %s\n", s.Mission)
					}
				}
			})
		},
	}

	open := &cobra.Command{
		Use: "open <nom>", Short: "Fichier RUN du spoke pour l'IA locale (chemin relatif + absolu)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sp, err := svc.Spokes()
			if err != nil {
				return err
			}
			for _, s := range sp {
				if strings.EqualFold(s.Name, args[0]) {
					info := runOpenInfo{Name: s.Name, LocalPath: s.LocalPath, RunFile: s.RunFile, RunAbs: runAbs(s.LocalPath, s.RunFile), Kernel: s.Kernel}
					return emit(cmd, info, func() {
						if info.RunFile == "" {
							fmt.Fprintf(cmd.OutOrStdout(), "%s : aucun fichier run défini (l'humain le définit dans la fenêtre : SPOKES ▸ Fichier run).\n", info.Name)
							return
						}
						fmt.Fprintln(cmd.OutOrStdout(), info.RunAbs)
					})
				}
			}
			return fmt.Errorf("spoke « %s » introuvable", args[0])
		},
	}

	status := &cobra.Command{
		Use: "status <nom>", Short: "État lisible d'un spoke", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := svc.SpokeStatus(args[0])
			if err != nil {
				return err
			}
			return emit(cmd, st, func() { fmt.Fprintf(cmd.OutOrStdout(), "%s (%s)\n%s\n", st.Name, st.Branch, st.Summary) })
		},
	}

	contents := &cobra.Command{
		Use: "contents <nom>", Short: "Carte du contenu d'un spoke", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			es, err := svc.SpokeContents(args[0])
			if err != nil {
				return err
			}
			return emit(cmd, es, func() {
				for _, e := range es {
					fmt.Fprintf(cmd.OutOrStdout(), "%-8s %s\n", e.Kind, e.Name)
				}
			})
		},
	}

	pull := &cobra.Command{
		Use: "pull <nom>", Short: "Récupère les dernières nouveautés (fast-forward)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := svc.SpokePull(args[0])
			if err != nil {
				return err
			}
			return emit(cmd, map[string]string{"output": out}, func() { fmt.Fprintln(cmd.OutOrStdout(), "Récupéré.") })
		},
	}

	var saveMsg string
	save := &cobra.Command{
		Use: "save <nom>", Short: "Enregistre et envoie le travail du spoke (PIN sur le poste)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := svc.SpokeSave(args[0], saveMsg)
			if err != nil {
				return err
			}
			return emit(cmd, res, func() { fmt.Fprintln(cmd.OutOrStdout(), res.Summary) })
		},
	}
	save.Flags().StringVar(&saveMsg, "message", "", "libellé de l'enregistrement (sinon daté automatiquement)")

	c.AddCommand(list, open, status, contents, pull, save)
	return c
}
