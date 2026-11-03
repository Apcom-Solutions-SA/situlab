// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package cli

import (
	"fmt"

	"situlab/internal/daemon"
	"situlab/internal/journal"

	"github.com/spf13/cobra"
)

// Méthodes du client démon pour le journal local (relais vers le démon résident, comme le reste).
func (r *remote) JournalRead(limit int, onlyErr bool) ([]journal.Entry, error) {
	a := daemon.Args{Limit: limit}
	if onlyErr {
		a.State = "errors"
	}
	var out []journal.Entry
	err := r.call(daemon.OpJournalRead, a, &out)
	return out, err
}

func (r *remote) JournalLevel(set string) (string, error) {
	return r.callStr(daemon.OpJournalLevel, "level", daemon.Args{Name: set})
}

func (r *remote) JournalClear() error {
	return r.call(daemon.OpJournalClear, daemon.Args{}, nil)
}

// cmdJournal : journal local des opérations (lecture, niveau, purge).
func cmdJournal() *cobra.Command {
	c := &cobra.Command{
		Use:   "journal",
		Short: "Journal local des opérations (lecture, niveau, purge)",
		Long:  "Trace locale des opérations SituLab (surface, succès/échec, durée). Reste sur le poste, jamais envoyée. Documentation : docs/JOURNAL.md.",
	}

	var limit int
	var onlyErr bool
	tail := &cobra.Command{
		Use:   "tail",
		Short: "Affiche les dernières entrées (récentes d'abord)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ents, err := svc.JournalRead(limit, onlyErr)
			if err != nil {
				return err
			}
			return emit(cmd, ents, func() {
				out := cmd.OutOrStdout()
				if len(ents) == 0 {
					fmt.Fprintln(out, "(journal vide)")
					return
				}
				for _, e := range ents {
					st := "ok"
					if !e.OK {
						st = "ÉCHEC"
					}
					fmt.Fprintf(out, "%s  %-7s %-20s %-5s %s", e.Time.Format("2006-01-02 15:04:05"), e.Actor, e.Op, st, e.Target)
					if e.Err != "" {
						fmt.Fprintf(out, "  — %s", e.Err)
					}
					fmt.Fprintln(out)
				}
			})
		},
	}
	tail.Flags().IntVar(&limit, "limit", 50, "nombre d'entrées à afficher")
	tail.Flags().BoolVar(&onlyErr, "errors", false, "n'afficher que les échecs")

	level := &cobra.Command{
		Use:   "level [off|errors|all]",
		Short: "Affiche (sans argument) ou change le niveau du journal",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			set := ""
			if len(args) == 1 {
				set = args[0]
			}
			lvl, err := svc.JournalLevel(set)
			if err != nil {
				return err
			}
			return emit(cmd, map[string]string{"level": lvl}, func() {
				fmt.Fprintf(cmd.OutOrStdout(), "niveau : %s\n", lvl)
			})
		},
	}

	clear := &cobra.Command{
		Use:   "clear",
		Short: "Efface le journal local",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := svc.JournalClear(); err != nil {
				return err
			}
			return emit(cmd, ok(), func() { fmt.Fprintln(cmd.OutOrStdout(), "journal effacé") })
		},
	}

	c.AddCommand(tail, level, clear)
	return c
}
