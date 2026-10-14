// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// cmdMap : la CARTE des dépôts pour l'IA — la couche de classement propre à SituLab. En un appel,
// l'IA voit chaque dépôt appairé avec : son rôle (KERNEL / spoke / normal), son tag projet, le lien
// local↔atelier↔vitrine, et le fichier d'ouverture (run) du KERNEL/spoke. C'est l'entrée pour qu'un
// agent comprenne l'organisation d'un poste sans deviner. Lecture seule, hors ligne (config + git local).
func cmdMap() *cobra.Command {
	return &cobra.Command{
		Use:   "map",
		Short: "Carte des dépôts pour l'IA (KERNEL/spoke/tag + liens local↔atelier↔vitrine)",
		Long: "Donne, pour chaque dépôt appairé : son rôle (KERNEL/spoke/normal), son tag de classement,\n" +
			"le clone LOCAL sur ce poste, le dépôt ATELIER (source de vérité), le nombre de VITRINES\n" +
			"(production), et le fichier d'ouverture (run) s'il est défini.\n\n" +
			"Rappel du modèle SituLab : l'`origin` du clone local est l'ATELIER (lien exact). Une VITRINE est\n" +
			"un dépôt de PRODUCTION séparé, relié par PROMOTION (version/tag), pas par un remote git.\n" +
			"Pour « ouvrir » un KERNEL, lire le fichier donné par `situlab kernel open`.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			rows, err := svc.ProjectsLight()
			if err != nil {
				return err
			}
			// Tri : KERNEL d'abord, puis spokes, puis par tag, puis par nom.
			sort.SliceStable(rows, func(i, j int) bool {
				ri := roleRank(rows[i].Kernel, rows[i].Spoke)
				rj := roleRank(rows[j].Kernel, rows[j].Spoke)
				if ri != rj {
					return ri < rj
				}
				if rows[i].Tag != rows[j].Tag {
					return strings.ToLower(rows[i].Tag) < strings.ToLower(rows[j].Tag)
				}
				return strings.ToLower(rows[i].Name) < strings.ToLower(rows[j].Name)
			})
			return emit(cmd, rows, func() {
				if len(rows) == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "Aucun dépôt appairé. L'humain appaire ses dépôts dans la fenêtre (Dépôts git).")
					return
				}
				for _, p := range rows {
					role := "dépôt"
					if p.Kernel {
						role = "KERNEL"
					} else if p.Spoke {
						role = "spoke"
					}
					tag := p.Tag
					if tag == "" {
						tag = "—"
					}
					local := "non cloné ici"
					if p.Local != nil && p.Local.Path != "" {
						local = p.Local.Path
					}
					run := ""
					if p.KernelRun != "" {
						run = "  ouverture=" + p.KernelRun
					} else if p.SpokeRun != "" {
						run = "  run=" + p.SpokeRun
					}
					fmt.Fprintf(cmd.OutOrStdout(), "[%s] %s  (tag: %s)%s\n", role, p.Name, tag, run)
					fmt.Fprintf(cmd.OutOrStdout(), "    atelier : %s/%s @ %s\n", p.Atelier.Owner, p.Atelier.Repo, p.Atelier.Instance)
					fmt.Fprintf(cmd.OutOrStdout(), "    local   : %s\n", local)
					fmt.Fprintf(cmd.OutOrStdout(), "    vitrines: %d (production ; reliées par promotion)\n", p.Vitrines)
				}
			})
		},
	}
}

// roleRank ordonne les rôles : KERNEL (0), spoke (1), dépôt normal (2).
func roleRank(kernel, spoke bool) int {
	switch {
	case kernel:
		return 0
	case spoke:
		return 1
	default:
		return 2
	}
}
