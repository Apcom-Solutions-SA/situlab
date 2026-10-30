// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package webdav

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeDAV : serveur WebDAV minimal pour exercer le client (PROPFIND/GET/PUT/MKCOL/DELETE).
func fakeDAV(t *testing.T, basePath string) *httptest.Server {
	t.Helper()
	ms := `<?xml version="1.0"?>
<d:multistatus xmlns:d="DAV:">
  <d:response><d:href>` + basePath + `/</d:href>
    <d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>
  <d:response><d:href>` + basePath + `/Docs/</d:href>
    <d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>
  <d:response><d:href>` + basePath + `/note.md</d:href>
    <d:propstat><d:prop><d:resourcetype/><d:getcontentlength>12</d:getcontentlength></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>
</d:multistatus>`
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); !ok || u != "alice" || p != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.Method {
		case "PROPFIND":
			w.WriteHeader(http.StatusMultiStatus)
			w.Write([]byte(ms))
		case http.MethodGet:
			w.Write([]byte("hello world!"))
		case http.MethodPut, "MKCOL":
			w.WriteHeader(http.StatusCreated)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
}

func TestWebDAVClient(t *testing.T) {
	const bp = "/remote.php/dav/files/alice"
	srv := fakeDAV(t, bp)
	defer srv.Close()
	c := New(srv.URL+bp, "alice", "secret", "test")
	ctx := context.Background()

	if err := c.TestConnection(ctx); err != nil {
		t.Fatalf("TestConnection : %v", err)
	}

	entries, err := c.List(ctx, "/")
	if err != nil {
		t.Fatalf("List : %v", err)
	}
	// Le dossier lui-même est exclu : on attend Docs (dossier) + note.md (fichier).
	if len(entries) != 2 {
		t.Fatalf("attendu 2 entrées, obtenu %d : %+v", len(entries), entries)
	}
	var sawDir, sawFile bool
	for _, e := range entries {
		switch e.Path {
		case "/Docs":
			sawDir = e.IsDir
		case "/note.md":
			sawFile = !e.IsDir && e.Size == 12
		}
	}
	if !sawDir || !sawFile {
		t.Errorf("entrées mal relativisées/typées : %+v", entries)
	}

	b, err := c.Get(ctx, "/note.md")
	if err != nil || string(b) != "hello world!" {
		t.Fatalf("Get : %q err=%v", b, err)
	}
	if err := c.Put(ctx, "/Docs/x.md", []byte("x")); err != nil {
		t.Errorf("Put : %v", err)
	}
	if err := c.Mkcol(ctx, "/Docs/sub"); err != nil {
		t.Errorf("Mkcol : %v", err)
	}
	if err := c.Delete(ctx, "/note.md"); err != nil {
		t.Errorf("Delete : %v", err)
	}
}

func TestWebDAVBadAuth(t *testing.T) {
	const bp = "/dav"
	srv := fakeDAV(t, bp)
	defer srv.Close()
	c := New(srv.URL+bp, "alice", "WRONG", "test")
	if err := c.TestConnection(context.Background()); err == nil || !strings.Contains(err.Error(), "refusé") {
		t.Errorf("mauvais mot de passe devrait être refusé, got %v", err)
	}
}
