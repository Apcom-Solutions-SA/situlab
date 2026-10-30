// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"errors"
	"os"
	"testing"

	"situlab/internal/config"
	"situlab/internal/gitx"
)

// TestPurgeWipesData : Purge efface le dossier de configuration (config + trousseau), si bien
// qu'après coup plus aucun PIN n'est défini (retour à blanc).
func TestPurgeWipesData(t *testing.T) {
	s := newInTemp(t)
	if err := s.SetPIN("", "123456"); err != nil {
		t.Fatalf("SetPIN : %v", err)
	}
	if !s.PINSet() {
		t.Fatal("un PIN devrait être défini après SetPIN")
	}
	// Une instance en config : exerce l'étape 1 de Purge (boucle de retrait du coffre OS).
	// Le retrait du coffre peut échouer selon l'environnement de test ; Purge l'ignore.
	if cfg, err := config.Load(); err == nil {
		cfg.Instances = append(cfg.Instances, config.Instance{Name: "t", URL: "https://x"})
		_ = cfg.Save()
	}
	if _, err := os.Stat(config.Dir()); err != nil {
		t.Fatalf("le dossier de config devrait exister : %v", err)
	}
	// Garde ANTI-DÉRIVE (1.18.4, retour #12) : on sème ce que les versions récentes ont ajouté et que
	// Purge doit emporter — une entrée du cache d'interface (1.18.3) et la fiche AGENTS.md à la source —
	// puis on vérifie qu'elles ont disparu. Si un de ces états migrait un jour HORS de config.Dir() sans
	// que Purge le suive, ce test le rattraperait (c'est exactement le risque signalé : du dev sur
	// plusieurs versions sans revérifier que la purge reste absolue).
	if err := s.UICacheSave("repos:probe", "data"); err != nil {
		t.Fatalf("UICacheSave : %v", err)
	}
	if v, _ := s.UICacheLoad("repos:probe"); v != "data" {
		t.Fatalf("l'entrée de cache devrait exister avant Purge, obtenu %q", v)
	}
	wroteDoc := os.WriteFile(config.AgentsDocPath(), []byte("# agents"), 0o600) == nil
	if err := s.Purge(); err != nil {
		t.Fatalf("Purge : %v", err)
	}
	if _, err := os.Stat(config.Dir()); !os.IsNotExist(err) {
		t.Errorf("le dossier de config devrait avoir disparu, obtenu %v", err)
	}
	if v, _ := s.UICacheLoad("repos:probe"); v != "" {
		t.Error("le cache d'interface devrait avoir été purgé (aurait-il migré hors de config.Dir() ?)")
	}
	if wroteDoc {
		if _, err := os.Stat(config.AgentsDocPath()); !os.IsNotExist(err) {
			t.Errorf("la fiche AGENTS.md administrée à la source devrait avoir disparu, obtenu %v", err)
		}
	}
	if s.PINSet() {
		t.Error("plus aucun PIN ne devrait être défini après Purge")
	}
}

// TestReadOnlyBlocksMutations : le mode lecture seule (v1.4) refuse toute mutation — y compris
// avec un PIN correct (refus AVANT vérification du PIN via gatePIN) et pour les mutations locales
// sans PIN (assertWritable) — puis cesse de bloquer une fois désactivé.
func TestReadOnlyBlocksMutations(t *testing.T) {
	s := newInTemp(t)
	if err := s.SetPIN("", "123456"); err != nil {
		t.Fatalf("SetPIN : %v", err)
	}
	if err := s.SetReadOnly(true); err != nil {
		t.Fatalf("SetReadOnly(true) : %v", err)
	}
	if !s.ReadOnly() {
		t.Fatal("ReadOnly() devrait être vrai")
	}

	// Écriture distante (passe par gatePIN) : refus AVANT vérification, malgré un PIN correct.
	if _, err := s.CreateRepo("x", "", "r", "", true, "123456"); !errors.Is(err, ErrReadOnly) {
		t.Errorf("CreateRepo en lecture seule : attendu ErrReadOnly, obtenu %v", err)
	}

	// Mutations LOCALES sans PIN (assertWritable).
	if gitx.Available() {
		if _, err := s.Commit(t.TempDir(), "x"); !errors.Is(err, ErrReadOnly) {
			t.Errorf("Commit en lecture seule : attendu ErrReadOnly, obtenu %v", err)
		}
		if _, err := s.GitStashSave(t.TempDir(), "x"); !errors.Is(err, ErrReadOnly) {
			t.Errorf("GitStashSave en lecture seule : attendu ErrReadOnly, obtenu %v", err)
		}
		if _, err := s.GitDiscard(t.TempDir(), ""); !errors.Is(err, ErrReadOnly) {
			t.Errorf("GitDiscard en lecture seule : attendu ErrReadOnly, obtenu %v", err)
		}
	}

	// Projets et dépôts locaux suivis : refus AVANT toute vérification git (ErrReadOnly sort en
	// tête d'assertWritable), donc valable même si git est absent ou le chemin n'est pas un dépôt.
	atelier := config.RepoRef{Instance: "forge", Owner: "acme", Repo: "site"}
	if err := s.SaveProject("", "P", "", atelier, nil); !errors.Is(err, ErrReadOnly) {
		t.Errorf("SaveProject en lecture seule : attendu ErrReadOnly, obtenu %v", err)
	}
	if err := s.RemoveProject("x"); !errors.Is(err, ErrReadOnly) {
		t.Errorf("RemoveProject en lecture seule : attendu ErrReadOnly, obtenu %v", err)
	}
	dir := t.TempDir()
	if err := s.AddLocalRepo(dir); !errors.Is(err, ErrReadOnly) {
		t.Errorf("AddLocalRepo en lecture seule : attendu ErrReadOnly, obtenu %v", err)
	}
	if err := s.RemoveLocalRepo(dir); !errors.Is(err, ErrReadOnly) {
		t.Errorf("RemoveLocalRepo en lecture seule : attendu ErrReadOnly, obtenu %v", err)
	}

	// Désactivation : la lecture seule ne bloque plus (l'erreur n'est plus ErrReadOnly).
	if err := s.SetReadOnly(false); err != nil {
		t.Fatalf("SetReadOnly(false) : %v", err)
	}
	if s.ReadOnly() {
		t.Fatal("ReadOnly() devrait être faux après désactivation")
	}
	if _, err := s.CreateRepo("x", "", "r", "", true, "123456"); errors.Is(err, ErrReadOnly) {
		t.Error("après désactivation, CreateRepo ne devrait plus renvoyer ErrReadOnly")
	}
}

// TestPruneMissingLocalRepos : un clone dont le dossier a disparu est retiré du suivi ; un dossier
// existant est conservé (correctif « à vérifier » fantômes après suppression des clones).
func TestPruneMissingLocalRepos(t *testing.T) {
	s := newInTemp(t)
	alive := t.TempDir() // existe
	dead := alive + "-supprime"
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.LocalRepos = []string{alive, dead}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	s.retrustPolicy() // policy posée hors setters (test) : ré-ancrer la signature
	s.pruneMissingLocalRepos()
	cfg2, _ := config.Load()
	if len(cfg2.LocalRepos) != 1 || config.CleanPath(cfg2.LocalRepos[0]) != config.CleanPath(alive) {
		t.Fatalf("après nettoyage, seul le dossier existant doit rester : %v", cfg2.LocalRepos)
	}
}
