// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package main

import (
	"os"
	"situlab/internal/core"

	wailsrt "github.com/wailsapp/wails/v2/pkg/runtime"
)

func (a *App) ExportConfig(exportPIN string, categories []string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.svc.ExportConfig(exportPIN, categories)
}

func (a *App) ListConfigBackups() []core.ConfigBackupInfo {
	if err := a.ready(); err != nil {
		return nil
	}
	return a.svc.ListConfigBackups()
}

func (a *App) DeleteConfigBackup(name string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.DeleteConfigBackup(name)
}

func (a *App) ImportConfigFile(importPIN, name string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.ImportConfigFile(importPIN, name)
}

func (a *App) InspectConfigBackup(name string) (map[string]any, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.InspectConfigBackup(name)
}

// SaveConfigBackupAs copie une sauvegarde vers un emplacement choisi (pour l'emporter sur une autre machine).
func (a *App) SaveConfigBackupAs(name string) error {
	if err := a.ready(); err != nil {
		return err
	}
	data, err := a.svc.ReadConfigBackup(name)
	if err != nil {
		return err
	}
	dest, err := wailsrt.SaveFileDialog(a.ctx, wailsrt.SaveDialogOptions{DefaultFilename: name, Title: "Enregistrer la sauvegarde SituLab"})
	if err != nil {
		return err
	}
	if dest == "" {
		return nil
	}
	return os.WriteFile(dest, data, 0o600)
}

// ImportConfigFromFile ouvre un sélecteur de fichier .slbak et l'importe sous importPIN.
func (a *App) ImportConfigFromFile(importPIN string) error {
	if err := a.ready(); err != nil {
		return err
	}
	path, err := wailsrt.OpenFileDialog(a.ctx, wailsrt.OpenDialogOptions{Title: "Choisir une sauvegarde SituLab (.slbak)", Filters: []wailsrt.FileFilter{{DisplayName: "Sauvegarde SituLab (*.slbak)", Pattern: "*.slbak"}}})
	if err != nil {
		return err
	}
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return a.svc.ImportConfig(importPIN, data)
}
