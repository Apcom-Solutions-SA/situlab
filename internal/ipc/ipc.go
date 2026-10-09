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
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"

	"situlab/internal/config"
)

// maxFrame borne la taille d'un message (anti-saturation mémoire).
const maxFrame = 8 << 20

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
	ln net.Listener
	h  Handler
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
	if c, derr := net.Dial("unix", path); derr == nil {
		c.Close()
		return nil, ErrAlreadyRunning // un démon vivant écoute : ne pas écraser
	}
	_ = os.Remove(path) // socket mort/résiduel éventuel
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	_ = os.Chmod(path, 0o600)
	s := &Server{ln: ln, h: h}
	go s.loop()
	return s, nil
}

func (s *Server) loop() {
	for {
		c, err := s.ln.Accept()
		if err != nil {
			return // listener fermé
		}
		go s.serve(c)
	}
}

func (s *Server) serve(c net.Conn) {
	defer c.Close()
	for {
		b, err := readFrame(c)
		if err != nil {
			return // connexion close ou erreur
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
		if writeFrame(c, out) != nil {
			return
		}
	}
}

// Close arrête l'écoute (les connexions en cours se terminent d'elles-mêmes).
func (s *Server) Close() error { return s.ln.Close() }

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
	c, err := net.Dial("unix", path)
	if err != nil {
		return nil, err
	}
	return &Client{c: c}, nil
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
