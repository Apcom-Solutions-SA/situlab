// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

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
