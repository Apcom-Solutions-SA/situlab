// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

// SituLab — client Forgejo de bureau, l'utilitaire git pour tous.
//
// Un seul binaire, deux visages :
//   - sans argument  -> ouvre la fenêtre graphique (humain)
//   - avec commande  -> mode CLI (agent) ; voir docs/AGENTS.md
package main

import (
	"embed"
	"os"

	"situlab/cli"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

// version est injectée au build (-ldflags "-X main.version=0.1").
var version = "1.9.9"

//go:embed all:frontend/dist
var assets embed.FS

// agentsDoc : la documentation (agents IA + humains) embarquée, consultable dans l'app.
//
//go:embed docs/AGENTS.md
var agentsDoc string

// licenseText et noticeText : la licence Apache 2.0 et l'avis de droits, embarqués et
// consultables depuis l'app (Aide → À propos). Conserver ces fichiers à la racine.
//
//go:embed LICENSE
var licenseText string

//go:embed NOTICE
var noticeText string

func main() {
	// Mode agent : toute invocation avec arguments passe par la CLI.
	if len(os.Args) > 1 {
		os.Exit(cli.Execute(version))
	}

	// Mode humain : fenêtre graphique native. On libère la console éventuellement
	// allouée par un double-clic pour ne pas laisser de fenêtre noire.
	releaseOwnConsole()
	app := NewApp()
	err := wails.Run(&options.App{
		Title:     "SituLab",
		Width:     1280,
		Height:    860,
		MinWidth:  1024,
		MinHeight: 640,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 248, G: 249, B: 251, A: 1},
		OnStartup:        app.startup,
		OnDomReady:       app.domReady,
		Bind:             []interface{}{app},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
		},
	})
	if err != nil {
		os.Exit(1)
	}
}
