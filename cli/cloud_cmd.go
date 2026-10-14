// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// cmdCloud : connexions aux clouds de fichiers souverains (CORPUS) + gestes de fichiers. L'IA ne
// voit que les dossiers autorisés par l'humain en fenêtre (l'allowlist est imposée par le démon).
func cmdCloud() *cobra.Command {
	c := &cobra.Command{Use: "cloud", Short: "Clouds de fichiers souverains (CORPUS : Nextcloud, kDrive)"}

	var clKind, clURL, clUser, clPass string
	add := &cobra.Command{
		Use:   "add <nom>",
		Short: "Connecte un cloud souverain (PIN/session requis). Types : nextcloud | kdrive.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clPass == "" {
				p, err := promptSecret("Mot de passe (app-password) pour " + clUser + " : ")
				if err != nil {
					return err
				}
				clPass = p
			}
			if err := svc.AddFileCloud(args[0], clKind, clURL, clUser, clPass); err != nil {
				return err
			}
			return emit(cmd, map[string]string{"status": "ok", "cloud": args[0]}, func() {
				fmt.Fprintf(cmd.OutOrStdout(), "Cloud « %s » connecté.\n", args[0])
			})
		},
	}
	add.Flags().StringVar(&clKind, "kind", "nextcloud", "type de cloud : nextcloud | kdrive")
	add.Flags().StringVar(&clURL, "url", "", "URL WebDAV de base")
	add.Flags().StringVar(&clUser, "user", "", "identifiant")
	add.Flags().StringVar(&clPass, "password", "", "mot de passe / app-password (sinon demandé)")

	list := &cobra.Command{
		Use: "list", Short: "Liste les connexions cloud", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cs, err := svc.FileClouds()
			if err != nil {
				return err
			}
			return emit(cmd, cs, func() {
				if len(cs) == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "Aucune connexion cloud.")
					return
				}
				for _, fc := range cs {
					def := ""
					if fc.Default {
						def = " (défaut)"
					}
					fmt.Fprintf(cmd.OutOrStdout(), "%s [%s] %s — %d dossier(s) autorisé(s) à l'IA%s\n", fc.Name, fc.Kind, fc.URL, len(fc.AllowedDirs), def)
				}
			})
		},
	}

	remove := &cobra.Command{
		Use: "remove <nom>", Short: "Supprime une connexion cloud", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := svc.RemoveFileCloud(args[0]); err != nil {
				return err
			}
			return emit(cmd, map[string]string{"status": "ok"}, func() { fmt.Fprintf(cmd.OutOrStdout(), "Cloud « %s » supprimé.\n", args[0]) })
		},
	}

	def := &cobra.Command{
		Use: "default <nom>", Short: "Définit la connexion cloud par défaut", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := svc.SetDefaultCloud(args[0]); err != nil {
				return err
			}
			return emit(cmd, map[string]string{"status": "ok"}, func() { fmt.Fprintf(cmd.OutOrStdout(), "Défaut : %s\n", args[0]) })
		},
	}

	allowed := &cobra.Command{
		Use: "allowed <cloud>", Short: "Dossiers autorisés à l'IA pour cette connexion", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dirs, err := svc.CloudAllowedDirs(args[0])
			if err != nil {
				return err
			}
			return emit(cmd, map[string]any{"dirs": dirs}, func() {
				if len(dirs) == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "Aucun dossier autorisé (l'humain les choisit dans la fenêtre).")
					return
				}
				for _, d := range dirs {
					fmt.Fprintln(cmd.OutOrStdout(), d)
				}
			})
		},
	}

	var lsCloud string
	ls := &cobra.Command{
		Use: "ls <dossier>", Short: "Liste le contenu d'un dossier cloud (dans le périmètre autorisé)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			es, err := svc.CloudList(lsCloud, args[0])
			if err != nil {
				return err
			}
			return emit(cmd, es, func() {
				for _, e := range es {
					kind := "f"
					if e.IsDir {
						kind = "d"
					}
					fmt.Fprintf(cmd.OutOrStdout(), "%s  %s\n", kind, e.Path)
				}
			})
		},
	}
	ls.Flags().StringVar(&lsCloud, "cloud", "", "connexion cloud (défaut : la connexion par défaut)")

	var opCloud string
	read := &cobra.Command{
		Use: "read <fichier>", Short: "Affiche le contenu texte d'un fichier cloud", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			txt, err := svc.CloudRead(opCloud, args[0])
			if err != nil {
				return err
			}
			return emit(cmd, map[string]string{"content": txt}, func() { fmt.Fprint(cmd.OutOrStdout(), txt) })
		},
	}
	read.Flags().StringVar(&opCloud, "cloud", "", "connexion cloud")

	var dlCloud string
	download := &cobra.Command{
		Use: "download <fichier> <destination-locale>", Short: "Télécharge un fichier cloud sur le poste", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := svc.CloudDownload(dlCloud, args[0], args[1]); err != nil {
				return err
			}
			return emit(cmd, map[string]string{"status": "ok"}, func() { fmt.Fprintf(cmd.OutOrStdout(), "Téléchargé : %s\n", args[1]) })
		},
	}
	download.Flags().StringVar(&dlCloud, "cloud", "", "connexion cloud")

	var mkCloud string
	mkdir := &cobra.Command{
		Use: "mkdir <dossier>", Short: "Crée un dossier (PIN selon la politique de la connexion)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := svc.CloudMkdir(mkCloud, args[0]); err != nil {
				return err
			}
			return emit(cmd, map[string]string{"status": "ok"}, func() { fmt.Fprintf(cmd.OutOrStdout(), "Dossier créé : %s\n", args[0]) })
		},
	}
	mkdir.Flags().StringVar(&mkCloud, "cloud", "", "connexion cloud")

	var upCloud string
	upload := &cobra.Command{
		Use: "upload <fichier-local> <dossier-cloud>", Short: "Envoie un fichier local vers un dossier cloud", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := svc.CloudUpload(upCloud, args[0], args[1]); err != nil {
				return err
			}
			return emit(cmd, map[string]string{"status": "ok"}, func() { fmt.Fprintln(cmd.OutOrStdout(), "Envoyé.") })
		},
	}
	upload.Flags().StringVar(&upCloud, "cloud", "", "connexion cloud")

	var wrCloud, wrContent string
	write := &cobra.Command{
		Use: "write <fichier-cloud>", Short: "Écrit/remplace un fichier cloud (contenu via --content)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := svc.CloudWrite(wrCloud, args[0], wrContent); err != nil {
				return err
			}
			return emit(cmd, map[string]string{"status": "ok"}, func() { fmt.Fprintln(cmd.OutOrStdout(), "Écrit.") })
		},
	}
	write.Flags().StringVar(&wrCloud, "cloud", "", "connexion cloud")
	write.Flags().StringVar(&wrContent, "content", "", "contenu texte à écrire")

	var delCloud string
	del := &cobra.Command{
		Use: "delete <chemin-cloud>", Short: "Supprime un fichier/dossier cloud (PIN par défaut)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := svc.CloudDelete(delCloud, args[0]); err != nil {
				return err
			}
			return emit(cmd, map[string]string{"status": "ok"}, func() { fmt.Fprintln(cmd.OutOrStdout(), "Supprimé.") })
		},
	}
	del.Flags().StringVar(&delCloud, "cloud", "", "connexion cloud")

	c.AddCommand(add, list, remove, def, allowed, ls, read, download, mkdir, upload, write, del)
	return c
}

// cmdBackup : plans de sauvegarde git → dossier cloud (bundle de l'historique complet).
func cmdBackup() *cobra.Command {
	c := &cobra.Command{Use: "backup", Short: "Sauvegardes d'un dépôt git vers un dossier cloud"}

	var bkRepo, bkCloud, bkDest string
	add := &cobra.Command{
		Use: "add <nom>", Short: "Enregistre un plan « dépôt git → cloud/dossier »", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := svc.AddBackupPlan(args[0], bkRepo, bkCloud, bkDest); err != nil {
				return err
			}
			return emit(cmd, map[string]string{"status": "ok"}, func() { fmt.Fprintf(cmd.OutOrStdout(), "Plan « %s » enregistré.\n", args[0]) })
		},
	}
	add.Flags().StringVar(&bkRepo, "repo", "", "chemin du dépôt git local à sauvegarder")
	add.Flags().StringVar(&bkCloud, "cloud", "", "connexion cloud de destination")
	add.Flags().StringVar(&bkDest, "dest", "/", "dossier de destination dans le cloud")

	list := &cobra.Command{
		Use: "list", Short: "Liste les plans de sauvegarde", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			bs, err := svc.Backups()
			if err != nil {
				return err
			}
			return emit(cmd, bs, func() {
				if len(bs) == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "Aucun plan de sauvegarde.")
					return
				}
				for _, b := range bs {
					last := b.LastRun
					if last == "" {
						last = "jamais"
					}
					fmt.Fprintf(cmd.OutOrStdout(), "%s : %s → %s:%s (dernier : %s)\n", b.Name, b.RepoPath, b.Cloud, b.Dest, last)
				}
			})
		},
	}

	remove := &cobra.Command{
		Use: "remove <nom>", Short: "Supprime un plan de sauvegarde", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := svc.RemoveBackupPlan(args[0]); err != nil {
				return err
			}
			return emit(cmd, map[string]string{"status": "ok"}, func() { fmt.Fprintf(cmd.OutOrStdout(), "Plan « %s » supprimé.\n", args[0]) })
		},
	}

	run := &cobra.Command{
		Use: "run <nom>", Short: "Exécute un plan de sauvegarde (bundle → cloud, PIN selon la connexion)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := svc.RunBackup(args[0])
			if err != nil {
				return err
			}
			return emit(cmd, map[string]string{"status": "ok", "path": out}, func() { fmt.Fprintf(cmd.OutOrStdout(), "Sauvegardé : %s\n", out) })
		},
	}

	c.AddCommand(add, list, remove, run)
	return c
}
