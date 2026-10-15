// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

// Package bridgewire définit le PROTOCOLE DE TRAMES du canal streaming box ⇄ hôte (SafeRoom ⇄ SituLab).
// Volontairement minuscule et SANS dépendance (importable à la fois par le démon hôte — paquet osbox —
// et par le binaire minuscule côté box — cmd/situlab-bridge), et indépendant de l'OS.
//
// Après le préambule (ligne jeton + ligne JSON {args,intent,mode:"stream"}), la connexion passe en mode
// trames bidirectionnel. Chaque trame = 1 octet de type + 4 octets de longueur (big-endian) + charge.
//
//	box → hôte : FrStdin (données d'entrée), FrStdinEOF (fin d'entrée)
//	hôte → box : FrStdout, FrStderr (flux temps réel), FrExit (code de sortie du process, fin de session)
package bridgewire

import (
	"encoding/binary"
	"errors"
	"io"
)

// Types de trames.
const (
	FrStdin    byte = 0 // box → hôte : octets d'entrée standard
	FrStdout   byte = 1 // hôte → box : octets de sortie standard
	FrStderr   byte = 2 // hôte → box : octets d'erreur standard
	FrExit     byte = 3 // hôte → box : code de sortie (ASCII), termine la session
	FrStdinEOF byte = 4 // box → hôte : plus d'entrée (ferme stdin du process hôte)
)

// maxFrame borne la taille d'une charge (anti-abus mémoire).
const maxFrame = 16 << 20 // 16 Mio

// WriteFrame écrit une trame complète (en-tête + charge) sur w.
func WriteFrame(w io.Writer, t byte, payload []byte) error {
	var h [5]byte
	h[0] = t
	binary.BigEndian.PutUint32(h[1:], uint32(len(payload)))
	if _, err := w.Write(h[:]); err != nil {
		return err
	}
	if len(payload) > 0 {
		_, err := w.Write(payload)
		return err
	}
	return nil
}

// ReadFrame lit une trame complète depuis r.
func ReadFrame(r io.Reader) (byte, []byte, error) {
	var h [5]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		return 0, nil, err
	}
	n := binary.BigEndian.Uint32(h[1:])
	if n > maxFrame {
		return 0, nil, errors.New("trame trop grande")
	}
	var p []byte
	if n > 0 {
		p = make([]byte, n)
		if _, err := io.ReadFull(r, p); err != nil {
			return 0, nil, err
		}
	}
	return h[0], p, nil
}
