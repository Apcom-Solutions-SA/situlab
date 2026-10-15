// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package osbox

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"situlab/internal/bridgewire"
)

// Pont HÔTE ⇄ BOX (§5 de la note d'architecture). La box ne détient AUCUN secret : elle ne fait que
// *demander* des actions au démon hôte, via ce pont. ⚠️ Fermeture de P2 (confused-deputy) : toute
// requête doit présenter un JETON ÉPHÉMÈRE par box. Sans jeton valide → refus. La clé git/cloud reste
// sur l'hôte ; une action sensible déclenche le PIN sur l'écran de l'hôte (comportement existant réutilisé).
//
// Deux modes :
//   - ping (legacy, un aller-retour JSON) : simple test de liaison.
//   - stream (v1.15.1) : SESSION CLI COMPLÈTE. L'intégralité du CLI `situlab` est relayée, avec flux
//     temps réel stdin ⇄ stdout/stderr et code de sortie propagé. La gouvernance n'est PAS ici : c'est
//     le démon hôte qui, pour une action sensible, fait surgir le PIN sur l'écran de l'utilisateur.
//
// Transport : TCP sur l'interface atteignable depuis WSL2 (la box joint l'hôte par son IP de passerelle).
// vsock (sans réseau) = durcissement v1.16+.

// NewToken génère un jeton éphémère (par box).
func NewToken() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Dispatch exécute une requête PING relayée et renvoie un court message d'état (une seule réponse).
type Dispatch func(args []string, intent string) (string, error)

// StreamDispatch exécute une SESSION CLI complète côté hôte : lit stdin, écrit stdout/stderr au fil de
// l'eau, et renvoie le code de sortie du process. ctx est annulé si la box se déconnecte.
type StreamDispatch func(ctx context.Context, args []string, intent string, stdin io.Reader, stdout, stderr io.Writer) int

// Bridge : pont actif (listener TCP token-gated).
type Bridge struct {
	ln       net.Listener
	token    string
	dispatch Dispatch
	stream   StreamDispatch
}

// StartBridge ouvre le pont sur addr (ex. "0.0.0.0:51774"), gardé par token, et sert en arrière-plan.
// dispatch traite le ping ; stream traite une session CLI complète (peut être nil pour désactiver).
func StartBridge(addr, token string, dispatch Dispatch, stream StreamDispatch) (*Bridge, error) {
	if token == "" || dispatch == nil {
		return nil, errors.New("pont : jeton et dispatch requis")
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	b := &Bridge{ln: ln, token: token, dispatch: dispatch, stream: stream}
	go b.serve()
	return b, nil
}

func (b *Bridge) Addr() string {
	if b == nil || b.ln == nil {
		return ""
	}
	return b.ln.Addr().String()
}
func (b *Bridge) Close() {
	if b != nil && b.ln != nil {
		_ = b.ln.Close()
	}
}

func (b *Bridge) serve() {
	for {
		c, err := b.ln.Accept()
		if err != nil {
			return
		}
		go b.handle(c)
	}
}

func (b *Bridge) handle(c net.Conn) {
	defer c.Close()
	r := bufio.NewReader(c)
	// Préambule gardé par un court délai (le flux, lui, n'a pas de deadline).
	_ = c.SetReadDeadline(time.Now().Add(30 * time.Second))

	// Ligne 1 : jeton (fermeture P2 — comparaison à temps constant).
	tok, _ := r.ReadString('\n')
	if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(tok)), []byte(b.token)) != 1 {
		_ = writeJSON(c, map[string]any{"error": "jeton invalide"})
		return
	}
	// Ligne 2 : JSON { args:[...], intent:"...", mode:"stream"|"" }.
	line, _ := r.ReadString('\n')
	var req struct {
		Args   []string `json:"args"`
		Intent string   `json:"intent"`
		Mode   string   `json:"mode"`
	}
	if json.Unmarshal([]byte(line), &req) != nil {
		_ = writeJSON(c, map[string]any{"error": "requête illisible"})
		return
	}

	if req.Mode == "stream" {
		_ = c.SetDeadline(time.Time{}) // session longue : pas de deadline
		b.serveStream(c, r, req.Args, req.Intent)
		return
	}

	// Legacy : ping / un aller-retour.
	_ = c.SetDeadline(time.Now().Add(3 * time.Minute))
	out, err := b.dispatch(req.Args, req.Intent)
	resp := map[string]any{"out": out}
	if err != nil {
		resp["error"] = err.Error()
	}
	_ = writeJSON(c, resp)
}

// serveStream pilote une session CLI complète : pompe les trames stdin de la box vers un pipe, exécute
// le dispatch streaming (qui écrit stdout/stderr en trames), puis émet le code de sortie.
func (b *Bridge) serveStream(c net.Conn, r *bufio.Reader, args []string, intent string) {
	if b.stream == nil {
		var mu sync.Mutex
		writeFrameLocked(c, &mu, bridgewire.FrStderr, []byte("pont : mode streaming indisponible\n"))
		writeFrameLocked(c, &mu, bridgewire.FrExit, []byte("127"))
		return
	}

	pr, pw := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// box → hôte : trames stdin / fin d'entrée. Une erreur de lecture = box déconnectée → on annule.
	go func() {
		for {
			t, p, err := bridgewire.ReadFrame(r)
			if err != nil {
				_ = pw.CloseWithError(err)
				cancel()
				return
			}
			switch t {
			case bridgewire.FrStdin:
				if _, err := pw.Write(p); err != nil {
					return
				}
			case bridgewire.FrStdinEOF:
				_ = pw.Close()
				return
			}
		}
	}()

	// hôte → box : stdout/stderr sérialisés (même connexion → mutex partagé).
	var mu sync.Mutex
	sout := &frameWriter{c: c, t: bridgewire.FrStdout, mu: &mu}
	serr := &frameWriter{c: c, t: bridgewire.FrStderr, mu: &mu}

	code := b.stream(ctx, args, intent, pr, sout, serr)
	_ = pr.Close()

	mu.Lock()
	_ = bridgewire.WriteFrame(c, bridgewire.FrExit, []byte(strconv.Itoa(code)))
	mu.Unlock()
}

// frameWriter : io.Writer qui émet chaque écriture comme une trame typée, sous verrou (flux concurrents).
type frameWriter struct {
	c  net.Conn
	t  byte
	mu *sync.Mutex
}

func (w *frameWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := bridgewire.WriteFrame(w.c, w.t, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

func writeFrameLocked(c net.Conn, mu *sync.Mutex, t byte, payload []byte) {
	mu.Lock()
	defer mu.Unlock()
	_ = bridgewire.WriteFrame(c, t, payload)
}

func writeJSON(c net.Conn, v any) error {
	enc, _ := json.Marshal(v)
	_, err := c.Write(append(enc, '\n'))
	return err
}
