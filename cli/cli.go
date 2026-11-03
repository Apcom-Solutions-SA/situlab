// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

// Package cli est le frontal ligne de commande de SituLab (binaire `situlab`).
// Il traduit des commandes en appels au noyau et présente le retour en texte ou JSON.
// Conçu pour être piloté par un agent IA : voir docs/AGENTS.md.
//
// Contrat agent : en --output json, les SUCCÈS comme les ERREURS sont du JSON, et les
// codes de sortie sont stables (0 ok, 1 générique, 2 instance, 3 PIN, 4 git absent).
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	flagOutput   string
	flagInstance string
	flagIntent   string // Point 4 : phrase « pourquoi » de l'agent, affichée dans la fenêtre PIN
	version      = "dev"
	svc          *remote // client du démon résident (connexion paresseuse)
)

// Execute lance la CLI et renvoie un code de sortie. La CLI ne détient pas de session :
// elle relaie tout au démon (qui porte la session et affiche la modale PIN sur le poste).
func Execute(ver string) int {
	version = ver
	svc = newRemote()
	if err := root().Execute(); err != nil {
		renderError(os.Stderr, err)
		return codeFor(err)
	}
	return 0
}

func root() *cobra.Command {
	root := &cobra.Command{
		Use:           "situlab",
		Short:         "SituLab : client Forgejo de bureau, l'utilitaire git pour tous",
		Long:          "SituLab connecte une ou plusieurs instances Forgejo via URL + token,\nexpose dépôts/profil/tokens selon vos droits et pilote git en local.\nSans argument : ouvre la fenêtre. Avec une commande : mode agent (voir docs/AGENTS.md).",
		SilenceUsage:  true,
		SilenceErrors: true, // le rendu d'erreur (texte/JSON) est géré par Execute
		Version:       version,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if flagOutput != "text" && flagOutput != "json" {
				return fmt.Errorf("format de sortie inconnu : %q (text|json)", flagOutput)
			}
			return nil
		},
	}
	root.PersistentFlags().StringVar(&flagOutput, "output", "text", "format de sortie : text|json")
	root.PersistentFlags().StringVar(&flagInstance, "instance", "", "instance ciblée (défaut : instance par défaut)")
	root.PersistentFlags().StringVar(&flagIntent, "intent", "", "phrase expliquant POURQUOI (affichée dans la fenêtre PIN sur le poste, à côté des actions constatées par SituLab)")

	root.AddCommand(cmdVersion(), cmdDoctor(), cmdInstance(), cmdWhoami(), cmdDashboard(),
		cmdRepo(), cmdIssue(), cmdPull(), cmdRelease(), cmdOrg(), cmdToken(), cmdLocal(), cmdProject(), cmdAttest(), cmdAdmin(), cmdPIN(), cmdMCP(),
		cmdCloud(), cmdBackup(), cmdKernel(), cmdSpoke(), cmdMap(),
		cmdUnlock(), cmdLock(), cmdStatus(), cmdJournal(), cmdPurge())
	return root
}
