// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"situlab/internal/core"

	"github.com/spf13/cobra"
)

// runOpenInfo : réponse de « kernel/spoke open » — le fichier d'ouverture pour l'IA locale, en chemin
// relatif ET absolu (prêt à être lu). Kernel non vide = parent (pour un spoke).
type runOpenInfo struct {
	Name      string `json:"name"`
	LocalPath string `json:"local_path"`
	RunFile   string `json:"run_file"`
	RunAbs    string `json:"run_abs,omitempty"`
	Kernel    string `json:"kernel,omitempty"`
}

func runAbs(localPath, runFile string) string {
	if runFile == "" {
		return ""
	}
	return filepath.Join(localPath, filepath.FromSlash(runFile))
}

// cmdKernel : la mémoire, lisible (KERNEL). Lecture (liste, état, carte du contenu) et gestes mémoire
// (récupérer, enregistrer) sur toutes les surfaces. La DÉSIGNATION (ajout/retrait/actif) reste
// réservée à la FENÊTRE — volontairement absente de la CLI : un agent ne choisit pas sa mémoire.
func cmdKernel() *cobra.Command {
	c := &cobra.Command{Use: "kernel", Short: "La mémoire (KERNEL) : état, contenu, récupérer/enregistrer vos notes"}

	list := &cobra.Command{
		Use: "list", Short: "Liste les KERNEL désignés (l'actif est marqué)", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ks, err := svc.Kernels()
			if err != nil {
				return err
			}
			return emit(cmd, ks, func() {
				if len(ks) == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "Aucun KERNEL désigné (l'humain le désigne dans la fenêtre).")
					return
				}
				for _, k := range ks {
					star := " "
					if k.Active {
						star = "*"
					}
					run := ""
					if k.RunFile != "" {
						run = "  [ouverture: " + k.RunFile + "]"
					}
					fmt.Fprintf(cmd.OutOrStdout(), "%s %s — %s%s\n", star, k.Name, k.LocalPath, run)
				}
			})
		},
	}

	open := &cobra.Command{
		Use: "open [nom]", Short: "Fichier d'OUVERTURE du KERNEL pour l'IA locale (chemin relatif + absolu)", Args: cobra.MaximumNArgs(1),
		Long: "Donne le fichier que l'IA doit lire pour « ouvrir » un KERNEL. En texte, affiche le chemin ABSOLU\n" +
			"(prêt à être lu) ; en --output json, renvoie name/local_path/run_file/run_abs. L'humain définit ce\n" +
			"fichier dans la fenêtre (KERNEL ▸ Fichier d'ouverture).",
		RunE: func(cmd *cobra.Command, args []string) error {
			k, err := resolveKernelArg(args)
			if err != nil {
				return err
			}
			info := runOpenInfo{Name: k.Name, LocalPath: k.LocalPath, RunFile: k.RunFile, RunAbs: runAbs(k.LocalPath, k.RunFile)}
			return emit(cmd, info, func() {
				if info.RunFile == "" {
					fmt.Fprintf(cmd.OutOrStdout(), "%s : aucun fichier d'ouverture défini (l'humain le définit dans la fenêtre : KERNEL ▸ Fichier d'ouverture).\n", info.Name)
					return
				}
				fmt.Fprintln(cmd.OutOrStdout(), info.RunAbs)
			})
		},
	}

	status := &cobra.Command{
		Use: "status [nom]", Short: "État lisible du KERNEL (actif par défaut)", Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := svc.KernelStatus(arg0(args))
			if err != nil {
				return err
			}
			return emit(cmd, st, func() {
				fmt.Fprintf(cmd.OutOrStdout(), "%s (%s)\n%s\n", st.Name, st.Branch, st.Summary)
			})
		},
	}

	contents := &cobra.Command{
		Use: "contents [nom]", Short: "Carte du contenu du KERNEL (dossiers/dépôts qu'il englobe)", Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			es, err := svc.KernelContents(arg0(args))
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

	cat := &cobra.Command{
		Use: "cat [nom]", Short: "Contenu du fichier d'OUVERTURE (lu sur le poste — utile via le pont SafeRoom)", Args: cobra.MaximumNArgs(1),
		Long: "Imprime le CONTENU du fichier d'ouverture (celui que « kernel open » désigne). Pensé pour un\n" +
			"agent qui ne partage pas le disque du poste (SafeRoom) : il reçoit le contenu, pas un chemin\n" +
			"illisible chez lui. Lecture seule, sans PIN.",
		RunE: func(cmd *cobra.Command, args []string) error {
			k, err := resolveKernelArg(args)
			if err != nil {
				return err
			}
			if k.RunFile == "" {
				return fmt.Errorf("%s : aucun fichier d'ouverture défini (l'humain le définit dans la fenêtre : KERNEL ▸ Fichier d'ouverture)", k.Name)
			}
			abs := runAbs(k.LocalPath, k.RunFile)
			data, err := os.ReadFile(abs)
			if err != nil {
				return fmt.Errorf("lecture du fichier d'ouverture (%s) : %w", abs, err)
			}
			info := struct {
				Name    string `json:"name"`
				RunFile string `json:"run_file"`
				RunAbs  string `json:"run_abs"`
				Content string `json:"content"`
			}{k.Name, k.RunFile, abs, string(data)}
			return emit(cmd, info, func() { fmt.Fprint(cmd.OutOrStdout(), string(data)) })
		},
	}

	pull := &cobra.Command{
		Use: "pull [nom]", Short: "Récupère les dernières notes (fast-forward)", Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := svc.KernelPull(arg0(args))
			if err != nil {
				return err
			}
			return emit(cmd, map[string]string{"output": out}, func() { fmt.Fprintln(cmd.OutOrStdout(), "Notes récupérées.") })
		},
	}

	var saveMsg string
	save := &cobra.Command{
		Use: "save [nom]", Short: "Enregistre vos notes (récupérer → enregistrer → envoyer ; PIN sur le poste)", Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := svc.KernelSave(arg0(args), saveMsg)
			if err != nil {
				return err
			}
			return emit(cmd, res, func() { fmt.Fprintln(cmd.OutOrStdout(), res.Summary) })
		},
	}
	save.Flags().StringVar(&saveMsg, "message", "", "libellé de l'enregistrement (sinon daté automatiquement)")

	c.AddCommand(list, open, status, contents, cat, pull, save)
	return c
}

// resolveKernelArg résout le KERNEL visé par l'argument optionnel : nom donné (insensible à la casse),
// sinon l'actif, sinon le premier désigné. Erreur explicite si aucun ne correspond.
func resolveKernelArg(args []string) (*core.KernelInfo, error) {
	ks, err := svc.Kernels()
	if err != nil {
		return nil, err
	}
	name := arg0(args)
	for i := range ks {
		if (name == "" && ks[i].Active) || (name != "" && strings.EqualFold(ks[i].Name, name)) {
			return &ks[i], nil
		}
	}
	if name == "" && len(ks) > 0 {
		return &ks[0], nil
	}
	return nil, fmt.Errorf("aucun KERNEL%s", map[bool]string{true: " « " + name + " »", false: " désigné"}[name != ""])
}

// arg0 renvoie le premier argument optionnel (ou "" => KERNEL actif).
func arg0(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	return ""
}
