// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// TestMCPInitializeAndList vérifie le cadre JSON-RPC : initialize répond, tools/list liste
// les outils, et une méthode inconnue renvoie une erreur -32601. Aucun démon requis.
func TestMCPInitializeAndList(t *testing.T) {
	in := strings.NewReader(strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize"}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"inconnue"}`,
	}, "\n") + "\n")
	var out bytes.Buffer
	if err := runMCP(in, &out); err != nil {
		t.Fatalf("runMCP: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	// 3 réponses attendues (la notification n'en produit pas).
	if len(lines) != 3 {
		t.Fatalf("attendu 3 réponses, obtenu %d : %q", len(lines), out.String())
	}
	// Ligne 2 : tools/list doit contenir au moins repo_create et whoami.
	if !strings.Contains(lines[1], `"whoami"`) || !strings.Contains(lines[1], `"repo_create"`) {
		t.Errorf("tools/list incomplet : %s", lines[1])
	}
	// Ligne 3 : méthode inconnue → erreur -32601.
	var r rpcResponse
	if err := json.Unmarshal([]byte(lines[2]), &r); err != nil {
		t.Fatalf("réponse 3 illisible: %v", err)
	}
	if r.Error == nil || r.Error.Code != -32601 {
		t.Errorf("méthode inconnue devrait donner -32601, obtenu %+v", r.Error)
	}
}

// TestMCPToolsUnique garantit l'absence de doublons de noms d'outils.
func TestMCPToolsUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, tl := range mcpTools() {
		if seen[tl.Name] {
			t.Errorf("outil dupliqué : %s", tl.Name)
		}
		seen[tl.Name] = true
		if tl.Handler == nil {
			t.Errorf("outil sans handler : %s", tl.Name)
		}
	}
}
