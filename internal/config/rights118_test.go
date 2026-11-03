// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package config

import "testing"

func lvl(fc FileCloud, p string, a CloudAction) CloudOpLevel { return fc.EffectiveCloudLevel(p, a) }

func TestEffectiveCloudLevelDefaults(t *testing.T) {
	fc := FileCloud{Name: "c"}
	if got := lvl(fc, "/x", CloudWrite); got != CloudLevelNone {
		t.Errorf("write défaut = %q, voulu none", got)
	}
	if got := lvl(fc, "/x", CloudDelete); got != CloudLevelPIN {
		t.Errorf("delete défaut = %q, voulu pin", got)
	}
	if got := lvl(fc, "/x", CloudRights); got != CloudLevelForbid {
		t.Errorf("rights défaut = %q, voulu forbid", got)
	}
}

func TestEffectiveCloudLevelMigrationRepli(t *testing.T) {
	yes, no := true, false
	fc := FileCloud{PINPolicy: &CloudPINPolicy{Write: &yes, Delete: &no}}
	if got := lvl(fc, "/x", CloudWrite); got != CloudLevelPIN {
		t.Errorf("legacy write=true → %q, voulu pin", got)
	}
	if got := lvl(fc, "/x", CloudDelete); got != CloudLevelNone {
		t.Errorf("legacy delete=false → %q, voulu none", got)
	}
}

func TestEffectiveCloudLevelPathSpecificity(t *testing.T) {
	fc := FileCloud{
		PINPolicy: &CloudPINPolicy{WriteLevel: CloudLevelNone},
		PathPolicies: []CloudPathRule{
			{Path: "/A", Write: CloudLevelPIN},
			{Path: "/A/secret", Write: CloudLevelForbid},
		},
	}
	if got := lvl(fc, "/B", CloudWrite); got != CloudLevelNone {
		t.Errorf("/B hérite du cloud → %q, voulu none", got)
	}
	if got := lvl(fc, "/A/doc", CloudWrite); got != CloudLevelPIN {
		t.Errorf("/A/doc → %q, voulu pin (règle /A)", got)
	}
	if got := lvl(fc, "/A/secret/k", CloudWrite); got != CloudLevelForbid {
		t.Errorf("/A/secret/k → %q, voulu forbid (règle la plus spécifique)", got)
	}
}

func TestEffectiveCloudLevelTieMostRestrictive(t *testing.T) {
	fc := FileCloud{PathPolicies: []CloudPathRule{
		{Path: "/A", Write: CloudLevelNone},
		{Path: "/A/", Write: CloudLevelForbid}, // même chemin normalisé → le plus restrictif gagne
	}}
	if got := lvl(fc, "/A/x", CloudWrite); got != CloudLevelForbid {
		t.Errorf("égalité → %q, voulu forbid (le plus restrictif gagne)", got)
	}
}

func TestSecurityDigestCoversPathPolicies(t *testing.T) {
	base := &Config{FileClouds: []FileCloud{{Name: "c", URL: "https://h"}}}
	d0 := base.SecurityDigest()
	withRule := &Config{FileClouds: []FileCloud{{Name: "c", URL: "https://h", PathPolicies: []CloudPathRule{{Path: "/A", Write: CloudLevelNone}}}}}
	d1 := withRule.SecurityDigest()
	if string(d0) == string(d1) {
		t.Fatal("ajouter une PathPolicy ne change PAS l'empreinte : élévation IA silencieuse possible")
	}
	// Un niveau de la politique de connexion doit aussi changer l'empreinte.
	withLevel := &Config{FileClouds: []FileCloud{{Name: "c", URL: "https://h", PINPolicy: &CloudPINPolicy{WriteLevel: CloudLevelForbid}}}}
	if string(withLevel.SecurityDigest()) == string(d0) {
		t.Fatal("changer WriteLevel ne change PAS l'empreinte")
	}
	// La méthode d'accès (webdav/rest) fait partie de l'identité signée.
	rest := &Config{FileClouds: []FileCloud{{Name: "c", URL: "https://h", Method: "rest"}}}
	if string(rest.SecurityDigest()) == string(d0) {
		t.Fatal("la méthode d'accès devrait faire partie du digest")
	}
}

func TestSecurityDigestStableOrder(t *testing.T) {
	a := &Config{FileClouds: []FileCloud{{Name: "a", URL: "u"}, {Name: "b", URL: "u"}}}
	b := &Config{FileClouds: []FileCloud{{Name: "b", URL: "u"}, {Name: "a", URL: "u"}}}
	if string(a.SecurityDigest()) != string(b.SecurityDigest()) {
		t.Fatal("l'empreinte doit être indépendante de l'ordre des clouds")
	}
}

func TestTagRegistry(t *testing.T) {
	c := &Config{Projects: []Project{{Name: "p", Tags: []string{"Alpha", "Beta"}}}}
	c.UpsertTagDef("Alpha", "blue")
	if c.TagColorOf("alpha") != "blue" {
		t.Error("couleur de tag non enregistrée (insensible à la casse)")
	}
	c.RenameTag("Alpha", "Gamma")
	if c.TagColorOf("Gamma") != "blue" {
		t.Error("la couleur doit suivre le renommage")
	}
	if got := c.Projects[0].TagList(); len(got) != 2 || got[0] != "Gamma" {
		t.Errorf("renommage non répercuté sur le projet : %v", got)
	}
	c.DeleteTag("Beta")
	if got := c.Projects[0].TagList(); len(got) != 1 || got[0] != "Gamma" {
		t.Errorf("suppression non répercutée : %v", got)
	}
}
