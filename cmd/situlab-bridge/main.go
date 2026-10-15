// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

// situlab-bridge : le pont CÔTE BOX (v1.15.1). Binaire MINUSCULE et SANS ÉTAT, installé dans la box
// (dans le PATH : /usr/local/bin/situlab-bridge). Il ne contient AUCUNE clé et n'est PAS le démon :
// il transmet l'intention au démon HÔTE (SituLab, Windows) via un canal TCP gardé par un jeton
// éphémère. Les clés git/cloud + la validation PIN restent sur l'hôte.
//
// L'agent (claude) l'utilise ainsi, DANS la box — 100 % du CLI SituLab, en flux temps réel :
//
//	situlab-bridge connect            → vérifie la liaison avec SituLab (hôte Windows)
//	situlab-bridge repo list          → relaie une commande situlab à l'hôte (flux live ; PIN sur l'hôte)
//	situlab-bridge issue new …        → n'importe quelle commande situlab, stdin/stdout/stderr branchés
//
// Configuration, injectée dans la box à l'installation depuis SituLab :
//
//	/etc/situlab/bridge.json  = { "host": "<ip hôte vue depuis la box>", "port": <port>, "token": "<jeton>" }
//
// (surchargeables par SITULAB_HOST / SITULAB_PORT / SITULAB_TOKEN).
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"situlab/internal/bridgewire"
)

type bridgeCfg struct {
	Host  string `json:"host"`
	Port  int    `json:"port"`
	Token string `json:"token"`
}

func loadCfg() bridgeCfg {
	var c bridgeCfg
	if b, err := os.ReadFile("/etc/situlab/bridge.json"); err == nil {
		_ = json.Unmarshal(b, &c)
	}
	if v := os.Getenv("SITULAB_HOST"); v != "" {
		c.Host = v
	}
	if v := os.Getenv("SITULAB_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			c.Port = p
		}
	}
	if v := os.Getenv("SITULAB_TOKEN"); v != "" {
		c.Token = v
	}
	return c
}

func (c bridgeCfg) addr() string { return net.JoinHostPort(c.Host, strconv.Itoa(c.Port)) }

// dial ouvre la connexion et envoie le préambule (jeton + requête JSON).
func dial(c bridgeCfg, args []string, mode string) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", c.addr(), 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("hôte SituLab injoignable (%s) : %w", c.addr(), err)
	}
	fmt.Fprintln(conn, c.Token)
	req, _ := json.Marshal(map[string]any{"args": args, "intent": "cli", "mode": mode})
	fmt.Fprintln(conn, string(req))
	return conn, nil
}

// ping : un aller-retour (test de liaison).
func ping(c bridgeCfg) (string, error) {
	conn, err := dial(c, nil, "")
	if err != nil {
		return "", err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
	line, _ := bufio.NewReader(conn).ReadString('\n')
	var resp struct {
		Out   string `json:"out"`
		Error string `json:"error"`
	}
	_ = json.Unmarshal([]byte(line), &resp)
	if resp.Error != "" {
		return resp.Out, fmt.Errorf("%s", resp.Error)
	}
	return resp.Out, nil
}

// stream : SESSION CLI COMPLÈTE. Pompe stdin → hôte, et stdout/stderr ← hôte, jusqu'au code de sortie.
func stream(c bridgeCfg, args []string) int {
	conn, err := dial(c, args, "stream")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer conn.Close()

	// stdin de la box → trames (en arrière-plan ; s'arrête quand la session se termine).
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := os.Stdin.Read(buf)
			if n > 0 {
				if werr := bridgewire.WriteFrame(conn, bridgewire.FrStdin, buf[:n]); werr != nil {
					return
				}
			}
			if err != nil {
				_ = bridgewire.WriteFrame(conn, bridgewire.FrStdinEOF, nil)
				return
			}
		}
	}()

	// trames hôte → stdout/stderr de la box, jusqu'à FrExit.
	br := bufio.NewReader(conn)
	for {
		t, p, err := bridgewire.ReadFrame(br)
		if err != nil {
			fmt.Fprintln(os.Stderr, "situlab-bridge : liaison interrompue :", err)
			return 1
		}
		switch t {
		case bridgewire.FrStdout:
			_, _ = os.Stdout.Write(p)
		case bridgewire.FrStderr:
			_, _ = os.Stderr.Write(p)
		case bridgewire.FrExit:
			code, _ := strconv.Atoi(strings.TrimSpace(string(p)))
			return code
		}
	}
}

func main() {
	args := os.Args[1:]
	c := loadCfg()
	if c.Host == "" || c.Port == 0 || c.Token == "" {
		fmt.Fprintln(os.Stderr, "situlab-bridge : non configuré. La box n'est pas (encore) reliée à SituLab.")
		fmt.Fprintln(os.Stderr, "Active-le dans SituLab ▸ Réglages ▸ OS de travail ▸ Installer « SituLab Bridge ».")
		os.Exit(2)
	}

	// Sans argument, « connect » ou « ping » : simple vérification de liaison.
	if len(args) == 0 || args[0] == "connect" || args[0] == "ping" {
		out, err := ping(c)
		if err != nil {
			fmt.Fprintln(os.Stderr, "✗ Pas de liaison avec SituLab :", err)
			os.Exit(1)
		}
		fmt.Println("✓ Connecté à SituLab (hôte Windows).")
		if strings.TrimSpace(out) != "" {
			fmt.Println(strings.TrimSpace(out))
		}
		return
	}

	// Sinon : session CLI complète en flux (le PIN, si nécessaire, surgit sur l'écran de l'hôte).
	os.Exit(stream(c, args))
}
