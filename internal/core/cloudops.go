// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"errors"
	"os"
	"path"
	"situlab/internal/config"
	"situlab/internal/kdrive"
	"situlab/internal/webdav"
	"strings"
	"sync"
)

// CloudListShared renvoie les fichiers « partagés avec moi » d'une connexion. kDrive REST seulement ;
// WebDAV (Nextcloud, kDrive WebDAV) n'expose pas les partages reçus → renvoie une liste vide.
func (s *Service) CloudListShared(name string) ([]webdav.Entry, error) {
	cl, _, err := s.fileCloudClientFor(name)
	if err != nil {
		return nil, err
	}
	if kc, ok := cl.(*kdrive.Client); ok {
		ctx, cancel := opCtx()
		defer cancel()
		list, err := kc.ListShared(ctx)
		if err != nil {
			return nil, err
		}
		for i := range list { // ces entrées sont, par définition, partagées AVEC MOI (#24)
			list[i].Shared = true
		}
		return list, nil
	}
	return []webdav.Entry{}, nil
}

// CloudHealthItem : santé d'une connexion cloud (pour la pastille de santé en fenêtre, v1.11).
type CloudHealthItem struct {
	Name string `json:"name"`
	OK   bool   `json:"ok"`
	Err  string `json:"err,omitempty"`
}

// CloudHealth teste chaque connexion cloud (PROPFIND racine, best-effort). Renvoie ErrLocked si la
// session est verrouillée (le secret est alors indisponible) ; sinon chaque item porte son état.
func (s *Service) CloudHealth() ([]CloudHealthItem, error) {
	clouds := s.FileClouds()
	// Perf (1.17, Point 2) : chaque test est un aller-retour réseau indépendant, avec un délai de garde
	// par cloud (opCtx). En file, un seul cloud lent (jusqu'à l'expiration) retardait TOUS les suivants —
	// les pastilles de santé mettaient alors des dizaines de secondes à s'afficher. On teste en parallèle ;
	// chaque goroutine écrit SA case (pas de course), même modèle que boardFor.
	out := make([]CloudHealthItem, len(clouds))
	var mu sync.Mutex
	locked := false
	var wg sync.WaitGroup
	for i, fc := range clouds {
		wg.Add(1)
		go func(i int, fc config.FileCloud) {
			defer wg.Done()
			cl, _, err := s.fileCloudClientFor(fc.Name)
			if err != nil {
				if errors.Is(err, ErrLocked) {
					mu.Lock()
					locked = true
					mu.Unlock()
				}
				out[i] = CloudHealthItem{Name: fc.Name, OK: false, Err: err.Error()}
				return
			}
			ctx, cancel := opCtx()
			terr := cl.TestConnection(ctx)
			cancel()
			it := CloudHealthItem{Name: fc.Name, OK: terr == nil}
			if terr != nil {
				it.Err = terr.Error()
			}
			out[i] = it
		}(i, fc)
	}
	wg.Wait()
	if locked {
		// Session verrouillée : condition globale (tous les clouds) — on remonte ErrLocked comme avant.
		return nil, ErrLocked
	}
	return out, nil
}

// --- Lectures (session déverrouillée, pas de PIN) -------------------------------------------

// CloudList énumère le contenu d'un dossier cloud.
func (s *Service) CloudList(name, dir string) ([]webdav.Entry, error) {
	cl, _, err := s.fileCloudClientFor(name)
	if err != nil {
		return nil, err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.List(ctx, dir)
}

// CloudGet télécharge le contenu d'un fichier (octets).
func (s *Service) CloudGet(name, p string) ([]byte, error) {
	cl, _, err := s.fileCloudClientFor(name)
	if err != nil {
		return nil, err
	}
	ctx, cancel := opCtx()
	defer cancel()
	return cl.Get(ctx, p)
}

// CloudRead renvoie le contenu TEXTE d'un fichier (pour l'IA qui consulte un document).
func (s *Service) CloudRead(name, p string) (string, error) {
	b, err := s.CloudGet(name, p)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// CloudDownload télécharge un fichier cloud vers un chemin local du poste.
func (s *Service) CloudDownload(name, remotePath, localDest string) error {
	b, err := s.CloudGet(name, remotePath)
	if err != nil {
		return err
	}
	return os.WriteFile(localDest, b, 0o600)
}

// --- Mutations (politique PIN de la connexion) ----------------------------------------------

// CloudMkdir crée un dossier sur le cloud.
func (s *Service) CloudMkdir(name, p, pin string) error {
	if err := s.authorizeCloud(name, p, config.CloudWrite, pin); err != nil {
		return err
	}
	cl, _, err := s.fileCloudClientFor(name)
	if err != nil {
		return err
	}
	ctx, cancel := opCtx()
	defer cancel()
	if err := cl.Mkcol(ctx, p); err != nil {
		return err
	}
	return nil
}

// CloudUpload envoie un fichier LOCAL du poste vers un dossier cloud (nom de base conservé).
func (s *Service) CloudUpload(name, localPath, remoteDir, pin string) error {
	// Autoriser sur le chemin FINAL (dossier + nom de base) : une règle fine ne doit pas être
	// contournable par le dossier parent (durcissement path-aware, v1.18.2).
	remote := path.Join("/"+strings.Trim(config.CloudCleanPath(remoteDir), "/"), filepathBase(localPath))
	if err := s.authorizeCloud(name, remote, config.CloudWrite, pin); err != nil {
		return err
	}
	fi, err := os.Stat(localPath)
	if err != nil {
		return err
	}
	if fi.IsDir() {
		return errors.New("l'import d'un dossier n'est pas pris en charge (sélectionnez des fichiers)")
	}
	if fi.Size() > maxUploadBytes {
		return webdav.ErrTooLarge
	}
	content, err := os.ReadFile(localPath)
	if err != nil {
		return err
	}
	cl, _, err := s.fileCloudClientFor(name)
	if err != nil {
		return err
	}
	ctx, cancel := opCtx()
	defer cancel()
	if err := cl.Put(ctx, remote, content); err != nil {
		return err
	}
	return nil
}

// CloudWrite écrit (crée ou remplace) un fichier cloud à partir d'un contenu fourni — notamment
// l'IA qui dépose un résumé .md dans le CORPUS.
func (s *Service) CloudWrite(name, remotePath, content, pin string) error {
	if err := s.authorizeCloud(name, remotePath, config.CloudWrite, pin); err != nil {
		return err
	}
	cl, _, err := s.fileCloudClientFor(name)
	if err != nil {
		return err
	}
	ctx, cancel := opCtx()
	defer cancel()
	if err := cl.Put(ctx, remotePath, []byte(content)); err != nil {
		return err
	}
	return nil
}

// CloudDelete supprime un fichier ou un dossier cloud (geste destructif → PIN par défaut).
func (s *Service) CloudDelete(name, p, pin string) error {
	if err := s.authorizeCloud(name, p, config.CloudDelete, pin); err != nil {
		return err
	}
	cl, _, err := s.fileCloudClientFor(name)
	if err != nil {
		return err
	}
	ctx, cancel := opCtx()
	defer cancel()
	if err := cl.Delete(ctx, p); err != nil {
		return err
	}
	return nil
}

// filepathBase renvoie le nom de base d'un chemin local (séparateurs OS ou POSIX).
func filepathBase(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	return path.Base(p)
}
