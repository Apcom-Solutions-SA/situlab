// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package main

import (
	"path/filepath"
	"situlab/internal/config"
	"situlab/internal/core"
	"situlab/internal/webdav"
	"strings"

	wailsrt "github.com/wailsapp/wails/v2/pkg/runtime"
)

// CloudEffectivePolicy renvoie les niveaux de droits résolus (write/delete/rights) d'un chemin cloud,
// pour l'affichage GUI par ligne (lecture).
func (a *App) CloudEffectivePolicy(name, path string) map[string]string {
	if err := a.ready(); err != nil {
		return nil
	}
	w, d, r := a.svc.CloudEffectivePolicy(name, path)
	return map[string]string{"write": string(w), "delete": string(d), "rights": string(r)}
}

// --- Export / import de la configuration SituLab (P18, v1.18.2) — FENÊTRE uniquement ---------
// Le PIN de sauvegarde transite en argument in-process, jamais par le socket. Rien en clair.

func (a *App) FileClouds() ([]config.FileCloud, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.FileClouds(), nil
}

// CloudResolveURL valide/normalise une URL de cloud sans connexion (tunnel d'ajout, étape URL).
func (a *App) CloudResolveURL(kind, url, user string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.svc.CloudResolveURL(kind, url, user)
}

func (a *App) AddFileCloud(name, kind, url, user, password string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.AddFileCloud(name, kind, url, user, password)
}

// EditFileCloud : modifier une connexion cloud WebDAV existante (URL + identifiant + mot de passe
// optionnel). Nom stable. Les clouds REST (kDrive par jeton) se retirent/reconnectent.
func (a *App) EditFileCloud(name, url, user, password string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.EditFileCloud(name, url, user, password)
}

// AddKDriveREST connecte un kDrive via l'API REST (jeton personnel scope drive). v1.18.2.
func (a *App) AddKDriveREST(name, driveID, token, email string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.AddKDriveREST(name, driveID, token, email)
}

// CloudListShared : fichiers « partagés avec moi » (kDrive REST ; vide en WebDAV). v1.18.2.
func (a *App) CloudListShared(cloud string) ([]webdav.Entry, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.CloudListShared(cloud)
}

func (a *App) RemoveFileCloud(name string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.RemoveFileCloud(name)
}

func (a *App) SetDefaultCloud(name string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.SetDefaultCloud(name)
}

// SetCloudAllowedDirs — FENÊTRE UNIQUEMENT (non exposé socket/CLI/MCP) : l'humain choisit les
// dossiers visibles par l'IA.
func (a *App) SetCloudAllowedDirs(name string, dirs []string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.SetCloudAllowedDirs(name, dirs)
}

// SetCloudAccess — FENÊTRE UNIQUEMENT : l'humain règle l'accès de l'IA par ARBRE (autorisations +
// exclusions fines) en un seul geste.
func (a *App) SetCloudAccess(name string, allowed, denied []string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.SetCloudAccess(name, allowed, denied)
}

// SetCloudPINPolicy — FENÊTRE UNIQUEMENT : l'humain règle l'exigence de PIN par action cloud.
func (a *App) SetCloudPINPolicy(name string, pol *config.CloudPINPolicy) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.svc.SetCloudPINPolicy(name, pol)
}

func (a *App) CloudList(cloud, dir string) ([]webdav.Entry, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.CloudList(cloud, dir)
}

func (a *App) CloudRead(cloud, path string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.svc.CloudRead(cloud, path)
}

// CloudSaveAs ouvre le sélecteur « enregistrer sous » et télécharge le fichier cloud. Renvoie le
// chemin local écrit (vide si l'utilisateur annule).
func (a *App) CloudSaveAs(cloud, remotePath string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	if a.ctx == nil {
		return "", nil
	}
	def := remotePath
	if i := strings.LastIndex(remotePath, "/"); i >= 0 {
		def = remotePath[i+1:]
	}
	dest, err := wailsrt.SaveFileDialog(a.ctx, wailsrt.SaveDialogOptions{Title: "Enregistrer sous", DefaultFilename: def})
	if err != nil || dest == "" {
		return "", err
	}
	if err := a.svc.CloudDownload(cloud, remotePath, dest); err != nil {
		return "", err
	}
	return dest, nil
}

func (a *App) CloudMkdir(cloud, path, pin string) error {
	if err := a.ready(); err != nil {
		return err
	}
	err := a.svc.CloudMkdir(cloud, path, pin)
	if err == nil {
		a.afterWrite()
	}
	return err
}

func (a *App) CloudWrite(cloud, remotePath, content, pin string) error {
	if err := a.ready(); err != nil {
		return err
	}
	err := a.svc.CloudWrite(cloud, remotePath, content, pin)
	if err == nil {
		a.afterWrite()
	}
	return err
}

func (a *App) CloudDelete(cloud, path, pin string) error {
	if err := a.ready(); err != nil {
		return err
	}
	err := a.svc.CloudDelete(cloud, path, pin)
	if err == nil {
		a.afterWrite()
	}
	return err
}

// CloudPickAndUpload ouvre le sélecteur multi-fichiers natif et envoie chaque fichier choisi vers
// le dossier cloud. Renvoie la liste des noms envoyés. Un seul PIN couvre le lot (session ouverte).
func (a *App) CloudPickAndUpload(cloud, remoteDir, pin string) ([]string, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	if a.ctx == nil {
		return nil, nil
	}
	files, err := wailsrt.OpenMultipleFilesDialog(a.ctx, wailsrt.OpenDialogOptions{Title: "Importer des fichiers vers le cloud"})
	if err != nil {
		return nil, err
	}
	var sent []string
	for _, f := range files {
		if err := a.svc.CloudUpload(cloud, f, remoteDir, pin); err != nil {
			return sent, err
		}
		sent = append(sent, filepath.Base(f))
	}
	if len(sent) > 0 {
		a.afterWrite()
	}
	return sent, nil
}

// CloudUpload envoie un fichier local précis (ex. depuis un glisser-déposer qui a fourni le chemin).
func (a *App) CloudUpload(cloud, localPath, remoteDir, pin string) error {
	if err := a.ready(); err != nil {
		return err
	}
	err := a.svc.CloudUpload(cloud, localPath, remoteDir, pin)
	if err == nil {
		a.afterWrite()
	}
	return err
}

// --- Sauvegardes git → cloud (v1.10) ---

// CloudHealth teste la joignabilité de chaque connexion cloud (pour la pastille de santé).
func (a *App) CloudHealth() ([]core.CloudHealthItem, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.CloudHealth()
}
