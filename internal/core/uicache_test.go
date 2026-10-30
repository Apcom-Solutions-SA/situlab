package core

import (
	"strings"
	"testing"
)

// Le cache d'interface : écrire/relire, absence = "", et le garde-fou de souveraineté (CORPUS/cloud
// jamais persistés). On isole le dossier de config via AppData (os.UserConfigDir sous Windows/Linux).
func TestUICacheRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AppData", dir)         // Windows (os.UserConfigDir)
	t.Setenv("XDG_CONFIG_HOME", dir) // Linux (os.UserConfigDir)
	var s Service

	if v, _ := s.UICacheLoad("repo:files:x"); v != "" {
		t.Fatalf("absence attendue, obtenu %q", v)
	}
	if err := s.UICacheSave("repo:files:x", `{"a":1}`); err != nil {
		t.Fatalf("save: %v", err)
	}
	if v, _ := s.UICacheLoad("repo:files:x"); v != `{"a":1}` {
		t.Fatalf("relecture attendue, obtenu %q", v)
	}
}

func TestUICacheForbidsCorpus(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AppData", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	var s Service

	for _, k := range []string{"corpus:apcom:/Business", "CLOUD:x", " Corpus:y"} {
		if err := s.UICacheSave(k, "secret-listing"); err != nil {
			t.Fatalf("save %q: %v", k, err)
		}
		if v, _ := s.UICacheLoad(k); v != "" {
			t.Fatalf("le CORPUS ne doit jamais être persisté (%q → %q)", k, v)
		}
	}
}

func TestUICacheRejectsOversize(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AppData", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	var s Service

	big := strings.Repeat("x", uiCacheMaxValue+1)
	_ = s.UICacheSave("repo:big", big)
	if v, _ := s.UICacheLoad("repo:big"); v != "" {
		t.Fatalf("une valeur > plafond ne doit pas être persistée")
	}
}
