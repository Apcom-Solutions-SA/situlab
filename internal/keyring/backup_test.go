// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package keyring

import (
	"bytes"
	"testing"
)

// Le .slbak ne doit JAMAIS contenir de secret en clair (jeton, mot de passe) ni le PIN.
func TestExportBundleNoPlaintextSecret(t *testing.T) {
	isolate(t)
	const secretToken = "glpat-SUPERSECRET-TOKEN-xyz"
	const secretPwd = "MotDePasseAppli-123!"
	f, dek, err := Init("1234")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := f.SetToken(dek, "forge", "https://git.example", []string{"repo"}, secretToken); err != nil {
		t.Fatalf("SetToken: %v", err)
	}
	if err := f.SetPassword(dek, "forge", secretPwd); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	kr, err := BuildExport(dek, "9999", nil)
	if err != nil {
		t.Fatalf("BuildExport: %v", err)
	}
	data, err := MakeBundle([]byte(`{"read_only":false}`), []string{"git"}, kr)
	if err != nil {
		t.Fatalf("MakeBundle: %v", err)
	}
	for _, needle := range []string{secretToken, secretPwd, "1234", "9999"} {
		if bytes.Contains(data, []byte(needle)) {
			t.Fatalf("FUITE : le .slbak contient %q en clair", needle)
		}
	}
}

// Un .slbak doit s'ouvrir avec le PIN de sauvegarde et redonner le jeton d'origine.
func TestExportImportRoundtrip(t *testing.T) {
	isolate(t)
	const tok = "token-abc-123"
	f, dek, err := Init("1111")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := f.SetToken(dek, "forge", "https://u", nil, tok); err != nil {
		t.Fatalf("SetToken: %v", err)
	}
	kr, err := BuildExport(dek, "2222", nil)
	if err != nil {
		t.Fatalf("BuildExport: %v", err)
	}
	data, err := MakeBundle([]byte(`{}`), nil, kr)
	if err != nil {
		t.Fatalf("MakeBundle: %v", err)
	}
	b, err := ParseBundle(data)
	if err != nil {
		t.Fatalf("ParseBundle: %v", err)
	}
	// Le bon PIN de sauvegarde ouvre ; un mauvais échoue.
	if _, err := b.Keyring.Unlock("0000"); err == nil {
		t.Fatal("un mauvais PIN de sauvegarde ne doit pas ouvrir le trousseau")
	}
	edek, err := b.Keyring.Unlock("2222")
	if err != nil {
		t.Fatalf("Unlock avec le PIN de sauvegarde: %v", err)
	}
	got, err := b.Keyring.GetToken(edek, "forge")
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}
	if got != tok {
		t.Fatalf("jeton restauré = %q, voulu %q", got, tok)
	}
}
