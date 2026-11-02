// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package cli

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

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
