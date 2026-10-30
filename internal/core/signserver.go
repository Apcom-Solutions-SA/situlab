// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client du SERVICE DE SIGNATURE SERVEUR (v1.9.6, « agent-proof »). Le démon demande l'empreinte d'un
// commit au service ; la clé privée du service ne quitte JAMAIS le serveur → un agent sur le poste ne
// peut rien forger hors ligne. L'autorisation est le jeton Forgejo de l'utilisateur (accès au dépôt visé),
// que le démon détient déjà — il n'est jamais remis au client/agent. Contrat :
// STACK_TECHNIQUE/SITULAB/signserver/SIGNATURE-SERVEUR.md.

const defaultSignServerURL = "https://situememoring.com"

func signNonce() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// requestServerPreimageAttestation demande au service la signature d'un PRÉ-IMAGE de commit (attestation
// v2 → portée par un trailer immuable, visible en ligne). Le serveur signe EXACTEMENT les octets du
// pré-image fournis ; la clé privée reste sur le serveur. Timeout court : la signature est best-effort et
// ne doit jamais faire traîner un commit si le service est lent/injoignable.
func requestServerPreimageAttestation(baseURL, token, preimageB64, repoURL string) ([]byte, error) {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultSignServerURL
	}
	payload, _ := json.Marshal(map[string]string{"preimage_b64": preimageB64, "repo_url": repoURL, "nonce": signNonce()})
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(baseURL, "/")+"/api/sign-preimage", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "token "+token)
	}
	resp, err := (&http.Client{Timeout: 8 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("service de signature : HTTP %d", resp.StatusCode)
	}
	var out struct {
		Attestation json.RawMessage `json:"attestation"`
	}
	if err := json.Unmarshal(body, &out); err != nil || len(out.Attestation) == 0 {
		return nil, fmt.Errorf("réponse du service de signature illisible")
	}
	return out.Attestation, nil
}

// fetchRevokedSerials récupère la liste de révocation du service (best-effort). Accepte un tableau JSON
// de serials, ou un objet { "revoked": [...] }.
func fetchRevokedSerials(baseURL string) ([]string, error) {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultSignServerURL
	}
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Get(strings.TrimRight(baseURL, "/") + "/.well-known/situlab-revoked.json")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("révocation : HTTP %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var arr []string
	if json.Unmarshal(body, &arr) == nil && arr != nil {
		return arr, nil
	}
	var obj struct {
		Revoked []string `json:"revoked"`
	}
	if json.Unmarshal(body, &obj) == nil {
		return obj.Revoked, nil
	}
	return nil, fmt.Errorf("liste de révocation illisible")
}
