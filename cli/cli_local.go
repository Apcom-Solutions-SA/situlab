// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

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
				return fmt.Errorf("action destructive : « reset --mode hard » perd les modifications non enregistrées, relancez avec --confirm")
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
						fmt.Fprintf(out, "  ✓ %s/%s : %s, équilibré\n", p.Owner, p.Repo, p.Version)
					default:
						pv := p.Version
						if pv == "" {
							pv = "(aucune version)"
						}
						fmt.Fprintf(out, "  ↓ %s/%s : %s, en retard de %d version(s)\n", p.Owner, p.Repo, pv, p.Behind)
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
