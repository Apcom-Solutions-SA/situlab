// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

// Frontal MCP (Model Context Protocol) — 3ᵉ accès de SituLab, à parité GUI/CLI.
//
// `situlab mcp` démarre un serveur MCP local sur stdio (JSON-RPC 2.0, messages délimités
// par des retours-ligne). C'est un ADAPTATEUR MINCE : aucune logique métier ici — chaque
// outil relaie au démon résident via le même client que la CLI (`remote`). Donc le modèle
// de sécurité est identique : lectures libres (session déverrouillée), mutations distantes
// sous PIN confirmé dans la fenêtre du bureau (l'agent prépare, l'humain débloque). Le PIN
// ne transite jamais par le socket ni par MCP.
//
// Implémentation volontairement sans dépendance externe (le protocole MCP tient en quelques
// méthodes : initialize, tools/list, tools/call, ping).

const mcpProtocolVersion = "2024-11-05"

// mcpTool décrit un outil exposé : nom, description, schéma d'entrée JSON, et exécuteur.
type mcpTool struct {
	Name    string
	Desc    string
	Schema  map[string]any
	Handler func(args map[string]any) (any, error)
}

// --- Cadre JSON-RPC 2.0 ---

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func cmdMCP() *cobra.Command {
	c := &cobra.Command{
		Use:   "mcp",
		Short: "Démarre le serveur MCP (stdio) pour les agents IA — lectures libres, mutations sous PIN",
		Long: "Serveur MCP local sur stdio. À déclarer dans un client MCP (Cursor, Claude Desktop, …) :\n" +
			"  commande : situlab   argument : mcp\n" +
			"Les mutations distantes ouvrent la fenêtre PIN sur le poste (consentement humain).\n" +
			"Toute mutation accepte un argument facultatif « intent » : une phrase expliquant POURQUOI, affichée dans la fenêtre PIN à côté des actions que SituLab constate.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runMCP(os.Stdin, os.Stdout)
		},
	}
	return c
}

// runMCP lit les requêtes JSON-RPC ligne par ligne et écrit les réponses sur out. Seules
// des trames JSON-RPC sont émises sur stdout ; toute trace éventuelle irait sur stderr.
func runMCP(in io.Reader, out io.Writer) error {
	tools := mcpTools()
	byName := map[string]mcpTool{}
	for _, t := range tools {
		byName[t.Name] = t
	}

	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024) // lignes jusqu'à 8 Mo
	enc := json.NewEncoder(out)

	send := func(resp rpcResponse) {
		resp.JSONRPC = "2.0"
		_ = enc.Encode(resp) // Encoder ajoute le retour-ligne (délimiteur stdio)
	}

	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var req rpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			send(rpcResponse{Error: &rpcError{Code: -32700, Message: "JSON invalide"}})
			continue
		}
		// Notification (sans id) : on ne répond pas (ex. notifications/initialized).
		notification := len(req.ID) == 0

		switch req.Method {
		case "initialize":
			send(rpcResponse{ID: req.ID, Result: map[string]any{
				"protocolVersion": mcpProtocolVersion,
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "situlab", "version": version},
			}})
		case "ping":
			send(rpcResponse{ID: req.ID, Result: map[string]any{}})
		case "notifications/initialized", "notifications/cancelled":
			// rien
		case "tools/list":
			list := make([]map[string]any, 0, len(tools))
			for _, t := range tools {
				list = append(list, map[string]any{
					"name": t.Name, "description": t.Desc, "inputSchema": t.Schema,
				})
			}
			send(rpcResponse{ID: req.ID, Result: map[string]any{"tools": list}})
		case "tools/call":
			if notification {
				continue
			}
			var p struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			}
			_ = json.Unmarshal(req.Params, &p)
			t, ok := byName[p.Name]
			if !ok {
				send(rpcResponse{ID: req.ID, Result: toolError("outil inconnu : " + p.Name)})
				continue
			}
			if p.Arguments == nil {
				p.Arguments = map[string]any{}
			}
			// Point 4 : TOUTE mutation accepte un « intent » (le pourquoi), affiché dans la fenêtre PIN à
			// côté des actions que SituLab constate. On le pose pour CET appel (MCP est séquentiel) via le
			// même canal que la CLI ; vide si absent → aucune fuite vers l'appel suivant. Pur affichage.
			flagIntent = argStr(p.Arguments, "intent")
			res, err := t.Handler(p.Arguments)
			if err != nil {
				send(rpcResponse{ID: req.ID, Result: toolError(err.Error())})
				continue
			}
			send(rpcResponse{ID: req.ID, Result: toolText(res)})
		default:
			if notification {
				continue
			}
			send(rpcResponse{ID: req.ID, Error: &rpcError{Code: -32601, Message: "méthode inconnue : " + req.Method}})
		}
	}
	return sc.Err()
}

// toolText emballe un résultat d'outil en contenu texte MCP (JSON lisible).
func toolText(v any) map[string]any {
	var text string
	switch s := v.(type) {
	case string:
		text = s
	default:
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			text = fmt.Sprintf("%v", v)
		} else {
			text = string(b)
		}
	}
	return map[string]any{"content": []map[string]any{{"type": "text", "text": text}}}
}

// toolError emballe une erreur d'outil (isError=true) — le PIN refusé/requis arrive ici.
func toolError(msg string) map[string]any {
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": "Erreur : " + msg}},
		"isError": true,
	}
}

// --- Aides d'extraction d'arguments ---

func argStr(a map[string]any, k string) string {
	if v, ok := a[k].(string); ok {
		return v
	}
	return ""
}

func argInt(a map[string]any, k string) int {
	switch v := a[k].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return 0
}

func argBool(a map[string]any, k string, def bool) bool {
	if v, ok := a[k].(bool); ok {
		return v
	}
	return def
}

// schéma JSON d'entrée (helper).
func obj(props map[string]any, required ...string) map[string]any {
	if props == nil {
		props = map[string]any{}
	}
	m := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		m["required"] = required
	}
	return m
}
func strProp(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}
func intProp(desc string) map[string]any {
	return map[string]any{"type": "integer", "description": desc}
}
func boolProp(desc string) map[string]any {
	return map[string]any{"type": "boolean", "description": desc}
}

var instProp = strProp("Instance ciblée (vide = instance par défaut).")

// ok renvoie un petit objet de statut pour les mutations sans valeur de retour.
func ok() map[string]any { return map[string]any{"status": "ok"} }

// mcpTools déclare les outils exposés (mêmes capacités noyau que la CLI). Lectures sans PIN ;
// mutations distantes sous PIN (la fenêtre du bureau s'ouvre, l'humain confirme).
func mcpTools() []mcpTool {
	return []mcpTool{
		// --- Lectures ---
		{"whoami", "Qui suis-je sur l'instance (compte, droits).", obj(map[string]any{"instance": instProp}),
			func(a map[string]any) (any, error) { return svc.Whoami(argStr(a, "instance")) }},
		{"instance_list", "Liste les instances git configurées.", obj(nil),
			func(a map[string]any) (any, error) { return svc.Instances() }},
		{"dashboard", "Vue unifiée : dépôts distants + clones locaux et leur état.", obj(map[string]any{"instance": instProp}),
			func(a map[string]any) (any, error) { return svc.Dashboard(argStr(a, "instance")) }},
		{"repo_list", "Liste les dépôts accessibles (y compris via organisations).", obj(map[string]any{"instance": instProp}),
			func(a map[string]any) (any, error) { return svc.Repos(argStr(a, "instance")) }},
		{"repo_view", "Détail d'un dépôt.", obj(map[string]any{"instance": instProp, "owner": strProp("propriétaire"), "repo": strProp("nom du dépôt")}, "owner", "repo"),
			func(a map[string]any) (any, error) {
				return svc.ViewRepo(argStr(a, "instance"), argStr(a, "owner"), argStr(a, "repo"))
			}},
		{"repo_branches", "Branches distantes d'un dépôt.", obj(map[string]any{"instance": instProp, "owner": strProp(""), "repo": strProp("")}, "owner", "repo"),
			func(a map[string]any) (any, error) {
				return svc.RepoBranches(argStr(a, "instance"), argStr(a, "owner"), argStr(a, "repo"))
			}},
		{"repo_tags", "Tags/versions d'un dépôt.", obj(map[string]any{"instance": instProp, "owner": strProp(""), "repo": strProp("")}, "owner", "repo"),
			func(a map[string]any) (any, error) {
				return svc.RepoTags(argStr(a, "instance"), argStr(a, "owner"), argStr(a, "repo"))
			}},
		{"repo_commits", "Commits d'une référence.", obj(map[string]any{"instance": instProp, "owner": strProp(""), "repo": strProp(""), "ref": strProp("branche/tag/sha"), "limit": intProp("nombre max")}, "owner", "repo"),
			func(a map[string]any) (any, error) {
				return svc.RepoCommits(argStr(a, "instance"), argStr(a, "owner"), argStr(a, "repo"), argStr(a, "ref"), argInt(a, "limit"))
			}},
		{"repo_tree", "Arborescence d'un dossier (path vide = racine).", obj(map[string]any{"instance": instProp, "owner": strProp(""), "repo": strProp(""), "path": strProp("chemin"), "ref": strProp("référence")}, "owner", "repo"),
			func(a map[string]any) (any, error) {
				return svc.RepoContents(argStr(a, "instance"), argStr(a, "owner"), argStr(a, "repo"), argStr(a, "path"), argStr(a, "ref"))
			}},
		{"repo_file", "Contenu texte d'un fichier.", obj(map[string]any{"instance": instProp, "owner": strProp(""), "repo": strProp(""), "path": strProp("chemin"), "ref": strProp("référence")}, "owner", "repo", "path"),
			func(a map[string]any) (any, error) {
				return svc.RepoFile(argStr(a, "instance"), argStr(a, "owner"), argStr(a, "repo"), argStr(a, "path"), argStr(a, "ref"))
			}},
		{"org_list", "Liste les organisations.", obj(map[string]any{"instance": instProp}),
			func(a map[string]any) (any, error) { return svc.Orgs(argStr(a, "instance")) }},
		{"issue_list", "Liste les issues (state: open|closed|all).", obj(map[string]any{"instance": instProp, "owner": strProp(""), "repo": strProp(""), "state": strProp("open|closed|all")}, "owner", "repo"),
			func(a map[string]any) (any, error) {
				return svc.RepoIssues(argStr(a, "instance"), argStr(a, "owner"), argStr(a, "repo"), argStr(a, "state"))
			}},
		{"pull_list", "Liste les demandes de fusion (state: open|closed|all).", obj(map[string]any{"instance": instProp, "owner": strProp(""), "repo": strProp(""), "state": strProp("open|closed|all")}, "owner", "repo"),
			func(a map[string]any) (any, error) {
				return svc.RepoPulls(argStr(a, "instance"), argStr(a, "owner"), argStr(a, "repo"), argStr(a, "state"))
			}},
		{"local_status", "État git d'un dépôt local (chemin).", obj(map[string]any{"path": strProp("chemin du dossier local")}, "path"),
			func(a map[string]any) (any, error) { return svc.LocalStatus(argStr(a, "path")) }},
		{"attest_status", "Signature de garantie « SituLab approved » : état d'attestation des derniers commits d'un dépôt local (par commit : vérifié ou non, et par qui). Lecture.", obj(map[string]any{"path": strProp("chemin du dossier local"), "limit": intProp("nombre de commits (défaut 20)")}, "path"),
			func(a map[string]any) (any, error) { return svc.AttestHistory(argStr(a, "path"), argInt(a, "limit")) }},
		{"project_list", "Liste les projets : liens atelier↔production (v1.7). Un projet relie un dépôt atelier (dev) à des dépôts production (vitrines, jamais poussées depuis le local).", obj(nil),
			func(a map[string]any) (any, error) { return svc.Projects() }},
		{"project_status", "Équilibre d'un projet : version de l'atelier vs chaque production (équilibré / en retard de N versions). Utile pour savoir ce qui est en prod.", obj(map[string]any{"name": strProp("nom du projet")}, "name"),
			func(a map[string]any) (any, error) { return svc.ProjectStatus(argStr(a, "name")) }},
		{"project_promote", "Publie une version CHOISIE de l'atelier vers une production du projet (un commit propre, pas les détails). Vers la PRODUCTION → PIN confirmé dans la fenêtre : tu prépares, l'humain valide. prod = index de la production (0 = la première) ; from = branche atelier ; version = tag exact ; to = branche prod cible ; mode = code|tag|release (défaut tag ; attention : pousser un tag ne crée PAS seul une Release GitHub — la CI la produit).", obj(map[string]any{"name": strProp("nom du projet"), "prod": intProp("index de la production (0 = première)"), "from": strProp("branche de l'atelier (défaut : par défaut)"), "version": strProp("version/tag exact (défaut : dernier tag)"), "to": strProp("branche de la production cible (défaut : par défaut)"), "mode": strProp("méthode : code | tag (défaut) | release"), "notes": strProp("notes de version (mode release)")}, "name"),
			func(a map[string]any) (any, error) {
				return svc.ProjectPromote(argStr(a, "name"), argInt(a, "prod"), argStr(a, "from"), argStr(a, "version"), argStr(a, "to"), argStr(a, "mode"), argStr(a, "notes"))
			}},

		// --- Mutations locales (sans PIN) ---
		{"repo_clone", "Clone un dépôt distant dans un dossier local.", obj(map[string]any{"instance": instProp, "owner": strProp(""), "repo": strProp(""), "dir": strProp("dossier cible")}, "owner", "repo", "dir"),
			func(a map[string]any) (any, error) {
				return svc.Clone(argStr(a, "instance"), argStr(a, "owner"), argStr(a, "repo"), argStr(a, "dir"))
			}},
		{"repo_fetch", "Récupère les nouveautés distantes (sans fusion).", obj(map[string]any{"instance": instProp, "path": strProp("dossier local")}, "path"),
			func(a map[string]any) (any, error) { return svc.Fetch(argStr(a, "instance"), argStr(a, "path")) }},
		{"repo_pull", "Met à jour la branche locale en fast-forward.", obj(map[string]any{"instance": instProp, "path": strProp("dossier local")}, "path"),
			func(a map[string]any) (any, error) { return svc.Pull(argStr(a, "instance"), argStr(a, "path")) }},
		{"repo_commit", "Enregistre une version locale.", obj(map[string]any{"path": strProp("dossier local"), "message": strProp("message")}, "path", "message"),
			func(a map[string]any) (any, error) { return svc.Commit(argStr(a, "path"), argStr(a, "message")) }},
		{"git_branch_create", "Crée une branche locale et bascule dessus.", obj(map[string]any{"path": strProp("dossier local"), "name": strProp("nom de branche")}, "path", "name"),
			func(a map[string]any) (any, error) { return svc.GitCreateBranch(argStr(a, "path"), argStr(a, "name")) }},
		{"git_switch", "Bascule sur une branche locale existante.", obj(map[string]any{"path": strProp("dossier local"), "name": strProp("branche")}, "path", "name"),
			func(a map[string]any) (any, error) { return svc.GitSwitchBranch(argStr(a, "path"), argStr(a, "name")) }},
		{"git_branch_delete", "Supprime une branche LOCALE. Sans force, git REFUSE une branche non fusionnée (anti-perte de travail) ; force=true force la suppression. Local, pas de PIN.", obj(map[string]any{"path": strProp("dossier local"), "name": strProp("branche à supprimer"), "force": boolProp("forcer même si non fusionnée")}, "path", "name"),
			func(a map[string]any) (any, error) {
				return svc.GitDeleteBranch(argStr(a, "path"), argStr(a, "name"), argBool(a, "force", false))
			}},
		{"git_worktree_list", "Liste les arbres de travail (worktrees) d'un dépôt : travailler sur plusieurs branches en parallèle depuis un seul dépôt (dossiers distincts, même historique).", obj(map[string]any{"path": strProp("dossier local")}, "path"),
			func(a map[string]any) (any, error) { return svc.GitWorktrees(argStr(a, "path")) }},
		{"git_worktree_add", "Ajoute un worktree dans un dossier. create=true crée une NOUVELLE branche ; sinon extrait la branche/réf existante (git refuse une branche déjà ouverte ailleurs). Idéal pour une session parallèle. Local.", obj(map[string]any{"path": strProp("dossier du dépôt"), "dir": strProp("dossier cible du worktree (créé)"), "name": strProp("branche"), "create": boolProp("créer une nouvelle branche")}, "path", "dir", "name"),
			func(a map[string]any) (any, error) {
				return svc.GitWorktreeAdd(argStr(a, "path"), argStr(a, "dir"), argStr(a, "name"), argBool(a, "create", false))
			}},
		{"git_worktree_remove", "Retire un worktree (supprime son dossier). force=true si modifications non enregistrées. Local.", obj(map[string]any{"path": strProp("dossier du dépôt"), "dir": strProp("dossier du worktree à retirer"), "force": boolProp("forcer malgré des modifications")}, "path", "dir"),
			func(a map[string]any) (any, error) {
				return svc.GitWorktreeRemove(argStr(a, "path"), argStr(a, "dir"), argBool(a, "force", false))
			}},
		{"git_worktree_prune", "Nettoie les références de worktrees dont le dossier a disparu.", obj(map[string]any{"path": strProp("dossier du dépôt")}, "path"),
			func(a map[string]any) (any, error) { return svc.GitWorktreePrune(argStr(a, "path")) }},
		{"git_log", "Historique local récent (structuré).", obj(map[string]any{"path": strProp("dossier local"), "limit": intProp("nombre de commits")}, "path"),
			func(a map[string]any) (any, error) { return svc.GitLog(argStr(a, "path"), argInt(a, "limit")) }},
		{"git_discard", "Annule les modifications locales non enregistrées (DESTRUCTIF). file vide = tout.", obj(map[string]any{"path": strProp("dossier local"), "file": strProp("fichier précis (vide = tout)")}, "path"),
			func(a map[string]any) (any, error) { return svc.GitDiscard(argStr(a, "path"), argStr(a, "file")) }},
		{"git_stash_save", "Met de côté les modifications locales.", obj(map[string]any{"path": strProp("dossier local"), "message": strProp("libellé")}, "path"),
			func(a map[string]any) (any, error) { return svc.GitStashSave(argStr(a, "path"), argStr(a, "message")) }},
		{"git_stash_list", "Liste les remises de côté.", obj(map[string]any{"path": strProp("dossier local")}, "path"),
			func(a map[string]any) (any, error) { return svc.GitStashList(argStr(a, "path")) }},
		{"git_stash_pop", "Réapplique la dernière remise de côté.", obj(map[string]any{"path": strProp("dossier local")}, "path"),
			func(a map[string]any) (any, error) { return svc.GitStashPop(argStr(a, "path")) }},

		// --- Git « complet » (v1.6) : gestes de dev avancés LOCAUX (sans PIN). En cas de
		//     conflit, l'outil renvoie une erreur guidée ; résous puis ...-continue (ou ...-abort). ---
		{"git_merge", "Fusionne une branche/réf dans la branche courante (local). Conflit → résoudre puis git_commit, ou git_merge_abort.", obj(map[string]any{"path": strProp("dossier local"), "ref": strProp("branche/réf à fusionner")}, "path", "ref"),
			func(a map[string]any) (any, error) { return svc.GitMerge(argStr(a, "path"), argStr(a, "ref")) }},
		{"git_merge_abort", "Annule une fusion en cours.", obj(map[string]any{"path": strProp("dossier local")}, "path"),
			func(a map[string]any) (any, error) { return svc.GitMergeAbort(argStr(a, "path")) }},
		{"git_rebase", "Rejoue la branche courante au sommet d'une autre (local). Conflit → résoudre puis git_rebase_continue, ou git_rebase_abort.", obj(map[string]any{"path": strProp("dossier local"), "upstream": strProp("branche/réf de base")}, "path", "upstream"),
			func(a map[string]any) (any, error) { return svc.GitRebase(argStr(a, "path"), argStr(a, "upstream")) }},
		{"git_rebase_continue", "Poursuit un rebase après résolution des conflits.", obj(map[string]any{"path": strProp("dossier local")}, "path"),
			func(a map[string]any) (any, error) { return svc.GitRebaseContinue(argStr(a, "path")) }},
		{"git_rebase_abort", "Annule un rebase en cours.", obj(map[string]any{"path": strProp("dossier local")}, "path"),
			func(a map[string]any) (any, error) { return svc.GitRebaseAbort(argStr(a, "path")) }},
		{"git_cherry_pick", "Applique un commit précis sur la branche courante.", obj(map[string]any{"path": strProp("dossier local"), "commit": strProp("sha/réf du commit")}, "path", "commit"),
			func(a map[string]any) (any, error) { return svc.GitCherryPick(argStr(a, "path"), argStr(a, "commit")) }},
		{"git_cherry_pick_continue", "Poursuit un cherry-pick après résolution des conflits.", obj(map[string]any{"path": strProp("dossier local")}, "path"),
			func(a map[string]any) (any, error) { return svc.GitCherryPickContinue(argStr(a, "path")) }},
		{"git_cherry_pick_abort", "Annule un cherry-pick en cours.", obj(map[string]any{"path": strProp("dossier local")}, "path"),
			func(a map[string]any) (any, error) { return svc.GitCherryPickAbort(argStr(a, "path")) }},
		{"git_revert", "Ajoute un commit qui annule les effets d'un commit précis (non destructif).", obj(map[string]any{"path": strProp("dossier local"), "commit": strProp("sha/réf du commit")}, "path", "commit"),
			func(a map[string]any) (any, error) { return svc.GitRevert(argStr(a, "path"), argStr(a, "commit")) }},
		{"git_reset", "Déplace HEAD vers une réf (mode: soft|mixed|hard|keep|merge). hard est DESTRUCTIF (perte des modifications non enregistrées).", obj(map[string]any{"path": strProp("dossier local"), "ref": strProp("commit/réf cible"), "mode": strProp("soft|mixed|hard|keep|merge (défaut mixed)")}, "path", "ref"),
			func(a map[string]any) (any, error) {
				mode := argStr(a, "mode")
				if mode == "" {
					mode = "mixed"
				}
				return svc.GitReset(argStr(a, "path"), mode, argStr(a, "ref"))
			}},
		{"git_amend", "Réécrit le dernier commit (intègre l'arbre courant ; message vide = conserver). À n'utiliser que sur un commit non publié.", obj(map[string]any{"path": strProp("dossier local"), "message": strProp("nouveau message (vide = conserver)")}, "path"),
			func(a map[string]any) (any, error) { return svc.GitAmend(argStr(a, "path"), argStr(a, "message")) }},
		{"git_commit_paths", "Enregistre UNIQUEMENT les chemins indiqués (staging sélectif, évite les fichiers parasites).", obj(map[string]any{"path": strProp("dossier local"), "message": strProp("message"), "paths": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "chemins à enregistrer"}}, "path", "message", "paths"),
			func(a map[string]any) (any, error) {
				return svc.GitCommitPaths(argStr(a, "path"), argStr(a, "message"), argStrSlice(a, "paths"))
			}},
		{"git_tag_create", "Crée un tag LOCAL (annoté si message). Publier ensuite avec git_push_tag.", obj(map[string]any{"path": strProp("dossier local"), "name": strProp("nom du tag"), "message": strProp("annotation (vide = tag léger)")}, "path", "name"),
			func(a map[string]any) (any, error) {
				return svc.GitTagCreate(argStr(a, "path"), argStr(a, "name"), argStr(a, "message"))
			}},

		// --- Mutations distantes (PIN requis : fenêtre sur le poste) ---
		{"repo_push", "Pousse la branche locale vers le distant. PIN requis.", obj(map[string]any{"instance": instProp, "path": strProp("dossier local")}, "path"),
			func(a map[string]any) (any, error) { return svc.Push(argStr(a, "instance"), argStr(a, "path")) }},
		{"repo_sync", "Synchronise un dépôt local : enregistre les modifications, récupère les nouveautés, puis envoie en ligne (geste unique). PIN requis.", obj(map[string]any{"instance": instProp, "path": strProp("dossier local"), "message": strProp("description de la version (sinon libellé daté)")}, "path"),
			func(a map[string]any) (any, error) {
				return svc.Synchronize(argStr(a, "instance"), argStr(a, "path"), argStr(a, "message"))
			}},
		{"git_push_force", "Pousse la branche courante en force sûr (--force-with-lease) après un rebase/amend : refuse d'écraser un travail distant apparu entre-temps. PIN requis.", obj(map[string]any{"instance": instProp, "path": strProp("dossier local")}, "path"),
			func(a map[string]any) (any, error) { return svc.GitPushForce(argStr(a, "instance"), argStr(a, "path")) }},
		{"git_push_tag", "Publie un tag vers le serveur. PIN requis.", obj(map[string]any{"instance": instProp, "path": strProp("dossier local"), "tag": strProp("nom du tag")}, "path", "tag"),
			func(a map[string]any) (any, error) {
				return svc.GitPushTag(argStr(a, "instance"), argStr(a, "path"), argStr(a, "tag"))
			}},
		{"repo_create", "Crée un dépôt distant (org recommandé). PIN requis.", obj(map[string]any{"instance": instProp, "org": strProp("organisation (vide = perso)"), "name": strProp("nom"), "description": strProp("description"), "private": boolProp("privé (défaut true)")}, "name"),
			func(a map[string]any) (any, error) {
				return svc.CreateRepo(argStr(a, "instance"), argStr(a, "org"), argStr(a, "name"), argStr(a, "description"), argBool(a, "private", true))
			}},
		{"repo_delete", "Supprime définitivement un dépôt distant. PIN requis.", obj(map[string]any{"instance": instProp, "owner": strProp(""), "repo": strProp("")}, "owner", "repo"),
			func(a map[string]any) (any, error) {
				return ok(), svc.DeleteRepo(argStr(a, "instance"), argStr(a, "owner"), argStr(a, "repo"))
			}},
		{"issue_create", "Crée une issue. PIN requis.", obj(map[string]any{"instance": instProp, "owner": strProp(""), "repo": strProp(""), "title": strProp("titre"), "body": strProp("description")}, "owner", "repo", "title"),
			func(a map[string]any) (any, error) {
				return svc.CreateIssue(argStr(a, "instance"), argStr(a, "owner"), argStr(a, "repo"), argStr(a, "title"), argStr(a, "body"))
			}},
		{"issue_comment", "Commente une issue/PR. PIN requis.", obj(map[string]any{"instance": instProp, "owner": strProp(""), "repo": strProp(""), "index": intProp("numéro"), "body": strProp("contenu")}, "owner", "repo", "index", "body"),
			func(a map[string]any) (any, error) {
				return ok(), svc.CommentIssue(argStr(a, "instance"), argStr(a, "owner"), argStr(a, "repo"), argInt(a, "index"), argStr(a, "body"))
			}},
		{"issue_close", "Ferme une issue. PIN requis.", obj(map[string]any{"instance": instProp, "owner": strProp(""), "repo": strProp(""), "index": intProp("numéro")}, "owner", "repo", "index"),
			func(a map[string]any) (any, error) {
				return ok(), svc.SetIssueState(argStr(a, "instance"), argStr(a, "owner"), argStr(a, "repo"), argInt(a, "index"), "closed")
			}},
		{"issue_reopen", "Rouvre une issue. PIN requis.", obj(map[string]any{"instance": instProp, "owner": strProp(""), "repo": strProp(""), "index": intProp("numéro")}, "owner", "repo", "index"),
			func(a map[string]any) (any, error) {
				return ok(), svc.SetIssueState(argStr(a, "instance"), argStr(a, "owner"), argStr(a, "repo"), argInt(a, "index"), "open")
			}},
		{"issue_edit", "Modifie titre/corps d'une issue. PIN requis.", obj(map[string]any{"instance": instProp, "owner": strProp(""), "repo": strProp(""), "index": intProp("numéro"), "title": strProp("titre"), "body": strProp("corps")}, "owner", "repo", "index"),
			func(a map[string]any) (any, error) {
				return ok(), svc.EditIssue(argStr(a, "instance"), argStr(a, "owner"), argStr(a, "repo"), argInt(a, "index"), argStr(a, "title"), argStr(a, "body"))
			}},
		{"pull_create", "Ouvre une demande de fusion. PIN requis.", obj(map[string]any{"instance": instProp, "owner": strProp(""), "repo": strProp(""), "title": strProp("titre"), "head": strProp("branche source"), "base": strProp("branche cible"), "body": strProp("description")}, "owner", "repo", "title", "head", "base"),
			func(a map[string]any) (any, error) {
				return svc.CreatePull(argStr(a, "instance"), argStr(a, "owner"), argStr(a, "repo"), argStr(a, "title"), argStr(a, "head"), argStr(a, "base"), argStr(a, "body"))
			}},
		{"pull_merge", "Fusionne une demande de fusion (method: merge|rebase|squash). PIN requis.", obj(map[string]any{"instance": instProp, "owner": strProp(""), "repo": strProp(""), "index": intProp("numéro"), "method": strProp("merge|rebase|squash")}, "owner", "repo", "index"),
			func(a map[string]any) (any, error) {
				return ok(), svc.MergePull(argStr(a, "instance"), argStr(a, "owner"), argStr(a, "repo"), argInt(a, "index"), argStr(a, "method"))
			}},

		// --- PR : détail / diff / fichiers (lecture) ; revue / état / édition (PIN) ---
		{"pull_detail", "Détail d'une demande de fusion (état, mergeable, branches).", obj(map[string]any{"instance": instProp, "owner": strProp(""), "repo": strProp(""), "index": intProp("numéro")}, "owner", "repo", "index"),
			func(a map[string]any) (any, error) {
				return svc.PullDetail(argStr(a, "instance"), argStr(a, "owner"), argStr(a, "repo"), argInt(a, "index"))
			}},
		{"pull_diff", "Diff unifié d'une demande de fusion (texte).", obj(map[string]any{"instance": instProp, "owner": strProp(""), "repo": strProp(""), "index": intProp("numéro")}, "owner", "repo", "index"),
			func(a map[string]any) (any, error) {
				return svc.PullDiff(argStr(a, "instance"), argStr(a, "owner"), argStr(a, "repo"), argInt(a, "index"))
			}},
		{"pull_files", "Fichiers modifiés par une demande de fusion.", obj(map[string]any{"instance": instProp, "owner": strProp(""), "repo": strProp(""), "index": intProp("numéro")}, "owner", "repo", "index"),
			func(a map[string]any) (any, error) {
				return svc.PullFiles(argStr(a, "instance"), argStr(a, "owner"), argStr(a, "repo"), argInt(a, "index"))
			}},
		{"pull_review", "Dépose une revue (event: APPROVED|REQUEST_CHANGES|COMMENT). PIN requis.", obj(map[string]any{"instance": instProp, "owner": strProp(""), "repo": strProp(""), "index": intProp("numéro"), "event": strProp("APPROVED|REQUEST_CHANGES|COMMENT"), "body": strProp("commentaire")}, "owner", "repo", "index", "event"),
			func(a map[string]any) (any, error) {
				return ok(), svc.ReviewPull(argStr(a, "instance"), argStr(a, "owner"), argStr(a, "repo"), argInt(a, "index"), argStr(a, "event"), argStr(a, "body"))
			}},
		{"pull_close", "Ferme une demande de fusion sans la fusionner. PIN requis.", obj(map[string]any{"instance": instProp, "owner": strProp(""), "repo": strProp(""), "index": intProp("numéro")}, "owner", "repo", "index"),
			func(a map[string]any) (any, error) {
				return ok(), svc.SetPullState(argStr(a, "instance"), argStr(a, "owner"), argStr(a, "repo"), argInt(a, "index"), "closed")
			}},
		{"pull_edit", "Modifie titre/corps d'une demande de fusion. PIN requis.", obj(map[string]any{"instance": instProp, "owner": strProp(""), "repo": strProp(""), "index": intProp("numéro"), "title": strProp("titre"), "body": strProp("corps")}, "owner", "repo", "index"),
			func(a map[string]any) (any, error) {
				return ok(), svc.EditPull(argStr(a, "instance"), argStr(a, "owner"), argStr(a, "repo"), argInt(a, "index"), argStr(a, "title"), argStr(a, "body"))
			}},

		// --- Releases & tags ---
		{"release_list", "Liste les versions (id, tag, titre) d'un dépôt. L'id sert à release_delete.", obj(map[string]any{"instance": instProp, "owner": strProp(""), "repo": strProp("")}, "owner", "repo"),
			func(a map[string]any) (any, error) {
				return svc.RepoReleases(argStr(a, "instance"), argStr(a, "owner"), argStr(a, "repo"))
			}},
		{"release_create", "Publie une version (tag). PIN requis.", obj(map[string]any{"instance": instProp, "owner": strProp(""), "repo": strProp(""), "tag": strProp("nom du tag"), "name": strProp("titre"), "body": strProp("notes"), "target": strProp("branche/commit cible"), "draft": boolProp("brouillon"), "prerelease": boolProp("pré-version")}, "owner", "repo", "tag"),
			func(a map[string]any) (any, error) {
				return svc.CreateRelease(argStr(a, "instance"), argStr(a, "owner"), argStr(a, "repo"), argStr(a, "tag"), argStr(a, "name"), argStr(a, "body"), argStr(a, "target"), argBool(a, "draft", false), argBool(a, "prerelease", false))
			}},
		{"release_delete", "Supprime une version par son id. PIN requis.", obj(map[string]any{"instance": instProp, "owner": strProp(""), "repo": strProp(""), "id": intProp("identifiant de la release")}, "owner", "repo", "id"),
			func(a map[string]any) (any, error) {
				return ok(), svc.DeleteRelease(argStr(a, "instance"), argStr(a, "owner"), argStr(a, "repo"), int64(argInt(a, "id")))
			}},
		{"tag_create", "Crée un tag. PIN requis.", obj(map[string]any{"instance": instProp, "owner": strProp(""), "repo": strProp(""), "tag": strProp("nom du tag"), "target": strProp("branche/commit cible")}, "owner", "repo", "tag"),
			func(a map[string]any) (any, error) {
				return ok(), svc.CreateTag(argStr(a, "instance"), argStr(a, "owner"), argStr(a, "repo"), argStr(a, "tag"), argStr(a, "target"))
			}},
		{"tag_delete", "Supprime un tag. PIN requis.", obj(map[string]any{"instance": instProp, "owner": strProp(""), "repo": strProp(""), "tag": strProp("nom du tag")}, "owner", "repo", "tag"),
			func(a map[string]any) (any, error) {
				return ok(), svc.DeleteTag(argStr(a, "instance"), argStr(a, "owner"), argStr(a, "repo"), argStr(a, "tag"))
			}},

		// --- Réglages de dépôt (PIN) ---
		{"repo_rename", "Renomme un dépôt. PIN requis.", obj(map[string]any{"instance": instProp, "owner": strProp(""), "repo": strProp(""), "new_name": strProp("nouveau nom")}, "owner", "repo", "new_name"),
			func(a map[string]any) (any, error) {
				return svc.RenameRepo(argStr(a, "instance"), argStr(a, "owner"), argStr(a, "repo"), argStr(a, "new_name"))
			}},
		{"repo_visibility", "Change la visibilité (private: true/false). PIN requis.", obj(map[string]any{"instance": instProp, "owner": strProp(""), "repo": strProp(""), "private": boolProp("privé")}, "owner", "repo", "private"),
			func(a map[string]any) (any, error) {
				return svc.SetRepoVisibility(argStr(a, "instance"), argStr(a, "owner"), argStr(a, "repo"), argBool(a, "private", true))
			}},
		{"repo_archive", "Archive/désarchive un dépôt. PIN requis.", obj(map[string]any{"instance": instProp, "owner": strProp(""), "repo": strProp(""), "archived": boolProp("archivé")}, "owner", "repo", "archived"),
			func(a map[string]any) (any, error) {
				return svc.ArchiveRepo(argStr(a, "instance"), argStr(a, "owner"), argStr(a, "repo"), argBool(a, "archived", true))
			}},
		{"repo_default_branch", "Change la branche par défaut. PIN requis.", obj(map[string]any{"instance": instProp, "owner": strProp(""), "repo": strProp(""), "branch": strProp("branche")}, "owner", "repo", "branch"),
			func(a map[string]any) (any, error) {
				return svc.SetDefaultBranch(argStr(a, "instance"), argStr(a, "owner"), argStr(a, "repo"), argStr(a, "branch"))
			}},
		{"repo_delete_branch", "Supprime une branche distante (refuse la branche par défaut). PIN requis.", obj(map[string]any{"instance": instProp, "owner": strProp(""), "repo": strProp(""), "branch": strProp("branche à supprimer")}, "owner", "repo", "branch"),
			func(a map[string]any) (any, error) {
				return ok(), svc.DeleteRemoteBranch(argStr(a, "instance"), argStr(a, "owner"), argStr(a, "repo"), argStr(a, "branch"))
			}},

		// --- Labels / assignés / milestones ---
		{"label_list", "Liste les étiquettes d'un dépôt.", obj(map[string]any{"instance": instProp, "owner": strProp(""), "repo": strProp("")}, "owner", "repo"),
			func(a map[string]any) (any, error) {
				return svc.RepoLabels(argStr(a, "instance"), argStr(a, "owner"), argStr(a, "repo"))
			}},
		{"milestone_list", "Liste les jalons d'un dépôt.", obj(map[string]any{"instance": instProp, "owner": strProp(""), "repo": strProp("")}, "owner", "repo"),
			func(a map[string]any) (any, error) {
				return svc.RepoMilestones(argStr(a, "instance"), argStr(a, "owner"), argStr(a, "repo"))
			}},
		{"issue_set_labels", "Remplace les étiquettes d'une issue (par noms). PIN requis.", obj(map[string]any{"instance": instProp, "owner": strProp(""), "repo": strProp(""), "index": intProp("numéro"), "labels": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "noms d'étiquettes"}}, "owner", "repo", "index", "labels"),
			func(a map[string]any) (any, error) {
				return ok(), svc.SetIssueLabels(argStr(a, "instance"), argStr(a, "owner"), argStr(a, "repo"), argInt(a, "index"), argStrSlice(a, "labels"))
			}},
		{"issue_set_assignees", "Remplace les assignés d'une issue (par logins). PIN requis.", obj(map[string]any{"instance": instProp, "owner": strProp(""), "repo": strProp(""), "index": intProp("numéro"), "assignees": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "identifiants"}}, "owner", "repo", "index", "assignees"),
			func(a map[string]any) (any, error) {
				return ok(), svc.SetIssueAssignees(argStr(a, "instance"), argStr(a, "owner"), argStr(a, "repo"), argInt(a, "index"), argStrSlice(a, "assignees"))
			}},
		{"issue_set_milestone", "Fixe (ou retire avec 0) le jalon d'une issue. PIN requis.", obj(map[string]any{"instance": instProp, "owner": strProp(""), "repo": strProp(""), "index": intProp("numéro"), "milestone_id": intProp("id du jalon (0 = retirer)")}, "owner", "repo", "index", "milestone_id"),
			func(a map[string]any) (any, error) {
				return ok(), svc.SetIssueMilestone(argStr(a, "instance"), argStr(a, "owner"), argStr(a, "repo"), argInt(a, "index"), int64(argInt(a, "milestone_id")))
			}},
	}
}

// argStrSlice extrait un tableau de chaînes d'un argument JSON.
func argStrSlice(a map[string]any, k string) []string {
	raw, ok := a[k].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
