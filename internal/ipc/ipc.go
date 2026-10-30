// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

// Package ipc est le transport local entre les clients SituLab (commandes CLI one-shot)
// et le DÉMON résident qui détient la session déverrouillée (la DEK du trousseau).
//
// Pourquoi : chaque invocation CLI est un process neuf, sans mémoire ; sans démon, chaque
// commande repartirait verrouillée. Le démon porte la session ; les commandes lui parlent.
// Le PIN, lui, ne transite JAMAIS par ce canal : quand une opération l'exige, c'est le
// démon qui ouvre une fenêtre de saisie sur le bureau (modèle « courtier à consentement »).
//
// Canal : socket de domaine Unix (AF_UNIX, supporté par Go sur Windows 10+/11), rangé dans
// le dossier de config de l'utilisateur — donc restreint à sa session par les ACL du
// système de fichiers. Cadrage cohérent avec le modèle de menace déjà assumé.
//
// Cadre : messages préfixés d'une longueur (4 octets big-endian) puis une charge JSON.
package ipc

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	"situlab/internal/config"
)

const (
	// maxFrame borne la taille d'un message (anti-saturation mémoire).
	maxFrame = 8 << 20
	// maxConns borne le nombre de connexions simultanées servies (anti-saturation locale).
	maxConns = 64
	// idleTimeout borne l'inactivité ENTRE deux requêtes d'une même connexion (libère une
	// connexion abandonnée). Choisi > pinPromptTTL (2 min) : le temps d'une modale PIN se passe
	// dans le handler, pas en lecture de frame, donc il n'entre pas dans ce délai.
	idleTimeout = 5 * time.Minute
	// writeTimeout borne l'envoi d'une réponse (un client qui ne lit plus ne bloque pas la goroutine).
	writeTimeout = 60 * time.Second
	// dialTimeout borne la connexion au démon (socket local : immédiat en pratique).
	dialTimeout = 5 * time.Second
)

// ErrFrameTooLarge : message au-delà de la borne autorisée.
var ErrFrameTooLarge = errors.New("message IPC trop volumineux")

// Request est une demande d'opération au démon. Args porte les paramètres typés en JSON.
type Request struct {
	Op   string          `json:"op"`
	Args json.RawMessage `json:"args,omitempty"`
}

// Response est la réponse du démon. Kind classe l'erreur pour que la CLI retrouve son code
// de sortie stable (voir le mapping dans le frontal CLI).
type Response struct {
	OK   bool            `json:"ok"`
	Data json.RawMessage `json:"data,omitempty"`
	Err  string          `json:"err,omitempty"`
	Kind string          `json:"kind,omitempty"`
}

// SocketPath renvoie le chemin du socket du démon (dossier de config de l'utilisateur).
func SocketPath() string { return filepath.Join(config.Dir(), "daemon.sock") }

// handshakeTimeout borne l'attente du jeton d'autorisation en tête de connexion.
const handshakeTimeout = 10 * time.Second

// tokenPath : fichier du jeton d'autorisation du démon, dans le même dossier que le socket
// (restreint à l'utilisateur, 0600). Durcissement 1.16 (fermeture P2).
func tokenPath(sockPath string) string {
	return filepath.Join(filepath.Dir(sockPath), "daemon.token")
}

// newToken génère un jeton d'autorisation éphémère (régénéré à chaque session de démon).
func newToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// writeTokenFile écrit le jeton de façon atomique (0600).
func writeTokenFile(path, tok string) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(tok), 0o600); err != nil {
		return err
	}
	_ = os.Chmod(tmp, 0o600)
	return os.Rename(tmp, path)
}

func writeFrame(w io.Writer, b []byte) error {
	if len(b) > maxFrame {
		return ErrFrameTooLarge
	}
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(b)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err := w.Write(b)
	return err
}

func readFrame(r io.Reader) ([]byte, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n > maxFrame {
		return nil, ErrFrameTooLarge
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

// Handler traite une requête et renvoie une réponse. Fourni par le démon.
type Handler func(Request) Response

// Server écoute les clients locaux sur le socket.
type Server struct {
	ln    net.Listener
	h     Handler
	token string // jeton d'autorisation présenté par chaque client (fermeture P2, 1.16)
	tpath string // fichier du jeton (supprimé à la fermeture)
}

// ErrAlreadyRunning : un démon répond déjà sur ce socket (ne pas le voler).
var ErrAlreadyRunning = errors.New("un démon SituLab écoute déjà")

// Listen crée (ou recrée) le socket et sert les connexions en arrière-plan. Avant de
// supprimer un socket résiduel, on SONDE : si un démon vivant répond déjà, on refuse (sans
// voler son socket) ; on ne retire que les sockets morts (démon précédent mal arrêté).
func Listen(path string, h Handler) (*Server, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if c, derr := net.DialTimeout("unix", path, dialTimeout); derr == nil {
		c.Close()
		return nil, ErrAlreadyRunning // un démon vivant écoute : ne pas écraser
	}
	_ = os.Remove(path) // socket mort/résiduel éventuel
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	_ = os.Chmod(path, 0o600)
	// Jeton d'autorisation (fermeture P2, 1.16) : généré par session de démon et écrit AVANT d'accepter.
	// Tout client doit le présenter en tête de connexion. Le PIN ne transite toujours pas ici. Défense
	// en profondeur : le fichier reste lisible par le même compte OS (modèle de menace déjà assumé),
	// mais plus aucune connexion anonyme n'est servie (ex. process sans accès au dossier de config).
	tok := newToken()
	tp := tokenPath(path)
	if werr := writeTokenFile(tp, tok); werr != nil {
		_ = ln.Close()
		return nil, werr
	}
	s := &Server{ln: ln, h: h, token: tok, tpath: tp}
	go s.loop()
	return s, nil
}

func (s *Server) loop() {
	sem := make(chan struct{}, maxConns) // plafond de connexions simultanées
	for {
		c, err := s.ln.Accept()
		if err != nil {
			return // listener fermé
		}
		select {
		case sem <- struct{}{}:
			go func() {
				defer func() { <-sem }()
				s.serve(c)
			}()
		default:
			c.Close() // trop de connexions simultanées : refus propre (anti-saturation locale)
		}
	}
}

func (s *Server) serve(c net.Conn) {
	defer c.Close()
	// Handshake : la PREMIÈRE frame doit porter le jeton d'autorisation (comparaison à temps
	// constant). Sans jeton valide, on ne sert rien (fermeture P2, 1.16).
	_ = c.SetReadDeadline(time.Now().Add(handshakeTimeout))
	tb, herr := readFrame(c)
	if herr != nil {
		return
	}
	if subtle.ConstantTimeCompare(tb, []byte(s.token)) != 1 {
		return
	}
	for {
		// Échéance d'inactivité entre requêtes : une connexion abandonnée (client qui n'envoie
		// plus rien) est libérée au lieu de retenir une goroutine indéfiniment.
		_ = c.SetReadDeadline(time.Now().Add(idleTimeout))
		b, err := readFrame(c)
		if err != nil {
			return // connexion close, erreur, ou inactivité trop longue
		}
		var req Request
		var resp Response
		if jerr := json.Unmarshal(b, &req); jerr != nil {
			resp = Response{OK: false, Err: "requête IPC illisible", Kind: "generic"}
		} else {
			resp = s.h(req)
		}
		out, merr := json.Marshal(resp)
		if merr != nil {
			out, _ = json.Marshal(Response{OK: false, Err: "réponse IPC non sérialisable", Kind: "generic"})
		}
		_ = c.SetWriteDeadline(time.Now().Add(writeTimeout)) // un client qui ne lit plus ne bloque pas la goroutine
		if writeFrame(c, out) != nil {
			return
		}
	}
}

// Close arrête l'écoute (les connexions en cours se terminent d'elles-mêmes) et retire le jeton.
func (s *Server) Close() error {
	if s.tpath != "" {
		_ = os.Remove(s.tpath)
	}
	return s.ln.Close()
}

// Addr renvoie l'adresse d'écoute (utile aux tests).
func (s *Server) Addr() string {
	if s.ln == nil {
		return ""
	}
	return s.ln.Addr().String()
}

// Client est une connexion cliente vers le démon.
type Client struct{ c net.Conn }

// Dial se connecte au démon. Une erreur signifie généralement « démon absent » (l'appelant
// peut alors le démarrer puis réessayer).
func Dial(path string) (*Client, error) {
	c, err := net.DialTimeout("unix", path, dialTimeout)
	if err != nil {
		return nil, err
	}
	// Présente le jeton d'autorisation en tête de connexion (handshake, fermeture P2, 1.16).
	tok, rerr := os.ReadFile(tokenPath(path))
	if rerr != nil {
		_ = c.Close()
		return nil, fmt.Errorf("jeton du démon introuvable : %w", rerr)
	}
	_ = c.SetWriteDeadline(time.Now().Add(writeTimeout))
	if werr := writeFrame(c, tok); werr != nil {
		_ = c.Close()
		return nil, werr
	}
	_ = c.SetWriteDeadline(time.Time{})
	return &Client{c: c}, nil
}

// Alive sonde la PRÉSENCE d'un démon sur le socket par une connexion BRUTE, sans handshake. À utiliser
// pour la détection de vivacité (ex. refuser « purge » tant qu'une fenêtre tourne) : ce test ne doit
// PAS dépendre du jeton d'autorisation (un démon d'une version < 1.16 n'a pas écrit daemon.token, et
// Dial échouerait alors que le démon écoute bel et bien — fail-open du garde). Fermeture P2 oblige à
// séparer « est-ce qu'un démon écoute ? » de « suis-je autorisé à lui parler ? ».
func Alive(path string) bool {
	c, err := net.DialTimeout("unix", path, dialTimeout)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

// Call envoie une requête et attend la réponse (synchrone, une à la fois par Client).
func (cl *Client) Call(req Request) (Response, error) {
	b, err := json.Marshal(req)
	if err != nil {
		return Response{}, err
	}
	if err := writeFrame(cl.c, b); err != nil {
		return Response{}, err
	}
	rb, err := readFrame(cl.c)
	if err != nil {
		return Response{}, err
	}
	var resp Response
	if err := json.Unmarshal(rb, &resp); err != nil {
		return Response{}, err
	}
	return resp, nil
}

// Close ferme la connexion cliente.
func (cl *Client) Close() error { return cl.c.Close() }
