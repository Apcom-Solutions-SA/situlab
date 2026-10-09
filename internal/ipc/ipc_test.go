// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package ipc

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// sockPath donne un chemin de socket court dans un dossier temporaire (la limite AF_UNIX
// est d'environ 108 octets, y compris sous Windows).
func sockPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "s.sock")
}

func TestRoundtrip(t *testing.T) {
	path := sockPath(t)
	srv, err := Listen(path, func(r Request) Response {
		data, _ := json.Marshal(map[string]string{"echo": r.Op})
		return Response{OK: true, Data: data}
	})
	if err != nil {
		t.Fatalf("Listen (AF_UNIX indisponible ?) : %v", err)
	}
	defer srv.Close()

	cl, err := Dial(path)
	if err != nil {
		t.Fatalf("Dial : %v", err)
	}
	defer cl.Close()

	resp, err := cl.Call(Request{Op: "ping"})
	if err != nil {
		t.Fatalf("Call : %v", err)
	}
	if !resp.OK {
		t.Fatalf("OK = false, resp = %+v", resp)
	}
	var out map[string]string
	if err := json.Unmarshal(resp.Data, &out); err != nil {
		t.Fatalf("Data illisible : %v", err)
	}
	if out["echo"] != "ping" {
		t.Fatalf("echo = %q, want ping", out["echo"])
	}
}

func TestMultipleCallsSameConn(t *testing.T) {
	path := sockPath(t)
	srv, _ := Listen(path, func(r Request) Response {
		return Response{OK: true, Data: json.RawMessage(`"` + r.Op + `"`)}
	})
	defer srv.Close()
	cl, err := Dial(path)
	if err != nil {
		t.Fatalf("Dial : %v", err)
	}
	defer cl.Close()
	for _, op := range []string{"a", "b", "c"} {
		resp, err := cl.Call(Request{Op: op})
		if err != nil {
			t.Fatalf("Call %q : %v", op, err)
		}
		if string(resp.Data) != `"`+op+`"` {
			t.Fatalf("Data = %s, want %q", resp.Data, op)
		}
	}
}

func TestErrorResponse(t *testing.T) {
	path := sockPath(t)
	srv, _ := Listen(path, func(r Request) Response {
		return Response{OK: false, Err: "boum", Kind: "pinlocked"}
	})
	defer srv.Close()
	cl, _ := Dial(path)
	defer cl.Close()
	resp, err := cl.Call(Request{Op: "x"})
	if err != nil {
		t.Fatalf("Call : %v", err)
	}
	if resp.OK || resp.Err != "boum" || resp.Kind != "pinlocked" {
		t.Fatalf("réponse d'erreur inattendue : %+v", resp)
	}
}

func TestDialNoServer(t *testing.T) {
	if _, err := Dial(sockPath(t)); err == nil {
		t.Fatal("Dial sur socket absent aurait dû échouer (démon absent)")
	}
}

func TestLargeArgs(t *testing.T) {
	path := sockPath(t)
	srv, _ := Listen(path, func(r Request) Response {
		return Response{OK: true, Data: r.Args}
	})
	defer srv.Close()
	cl, _ := Dial(path)
	defer cl.Close()
	big := strings.Repeat("x", 200000)
	args, _ := json.Marshal(map[string]string{"blob": big})
	resp, err := cl.Call(Request{Op: "echo", Args: args})
	if err != nil {
		t.Fatalf("Call : %v", err)
	}
	var out map[string]string
	_ = json.Unmarshal(resp.Data, &out)
	if out["blob"] != big {
		t.Fatalf("charge volumineuse corrompue (len=%d)", len(out["blob"]))
	}
}
