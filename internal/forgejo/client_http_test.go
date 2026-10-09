// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package forgejo

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeForgejo monte un faux serveur Forgejo (httptest) couvrant les routes utilisées par les
// tests. Permet de tester le client, le parsing et les écritures SANS vrai compte ni réseau.
func fakeForgejo(t *testing.T) (*Client, *recorder) {
	t.Helper()
	rec := &recorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.lastPath = r.URL.Path
		rec.lastQuery = r.URL.RawQuery
		rec.lastMethod = r.Method
		rec.lastAuth = r.Header.Get("Authorization")
		switch {
		case r.URL.Path == "/api/v1/user":
			io.WriteString(w, `{"id":7,"login":"greg","full_name":"Greg","is_admin":true}`)
		case r.URL.Path == "/api/v1/repos/apcom/glim":
			io.WriteString(w, `{"full_name":"apcom/glim","name":"glim","private":true,"default_branch":"main","clone_url":"x"}`)
		case r.URL.Path == "/api/v1/repos/apcom/glim/issues" && r.Method == http.MethodGet:
			// Un vrai ticket + une PR (que ListIssues doit écarter).
			io.WriteString(w, `[{"number":1,"title":"bug","state":"open"},{"number":2,"title":"pr","pull_request":{}}]`)
		case r.URL.Path == "/api/v1/repos/apcom/glim/issues" && r.Method == http.MethodPost:
			b, _ := io.ReadAll(r.Body)
			rec.lastBody = string(b)
			io.WriteString(w, `{"number":3,"title":"créée"}`)
		case r.URL.Path == "/api/v1/repos/apcom/glim/branches":
			io.WriteString(w, `[{"name":"main","commit":{"id":"abc","timestamp":"2026-01-01T00:00:00Z"}}]`)
		default:
			w.WriteHeader(404)
			io.WriteString(w, `{"message":"not found"}`)
		}
	}))
	t.Cleanup(srv.Close)
	return NewKind(srv.URL, "tok", "test", KindForgejo), rec
}

type recorder struct{ lastPath, lastQuery, lastMethod, lastAuth, lastBody string }

func TestCurrentUserParsing(t *testing.T) {
	c, rec := fakeForgejo(t)
	u, err := c.CurrentUser(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if u.Login != "greg" || u.ID != 7 || !u.IsAdmin {
		t.Fatalf("user mal parsé : %+v", u)
	}
	if rec.lastAuth != "token tok" {
		t.Errorf("Authorization = %q, attendu 'token tok'", rec.lastAuth)
	}
}

func TestViewRepoParsing(t *testing.T) {
	c, _ := fakeForgejo(t)
	r, err := c.ViewRepo(context.Background(), "apcom", "glim")
	if err != nil {
		t.Fatal(err)
	}
	if r.FullName != "apcom/glim" || !r.Private || r.DefaultBranch != "main" {
		t.Fatalf("repo mal parsé : %+v", r)
	}
}

func TestListIssuesExcludesPullsAndWhitelistsState(t *testing.T) {
	c, rec := fakeForgejo(t)
	issues, err := c.ListIssues(context.Background(), "apcom", "glim", "open")
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 1 || issues[0].Number != 1 {
		t.Fatalf("les PR doivent être écartées ; obtenu %+v", issues)
	}
	// État non autorisé → ramené à « open » (pas d'injection de query).
	if _, err := c.ListIssues(context.Background(), "apcom", "glim", "open&injection=1"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rec.lastQuery, "state=open&") || strings.Contains(rec.lastQuery, "injection") {
		t.Fatalf("state non assaini dans la query : %q", rec.lastQuery)
	}
}

func TestCreateIssueSendsBody(t *testing.T) {
	c, rec := fakeForgejo(t)
	iss, err := c.CreateIssue(context.Background(), "apcom", "glim", "titre", "corps")
	if err != nil {
		t.Fatal(err)
	}
	if iss.Number != 3 {
		t.Fatalf("issue créée mal parsée : %+v", iss)
	}
	var sent map[string]any
	_ = json.Unmarshal([]byte(rec.lastBody), &sent)
	if sent["title"] != "titre" || sent["body"] != "corps" {
		t.Fatalf("corps POST incorrect : %s", rec.lastBody)
	}
}

func TestListBranchesParsing(t *testing.T) {
	c, _ := fakeForgejo(t)
	bs, err := c.ListBranches(context.Background(), "apcom", "glim")
	if err != nil {
		t.Fatal(err)
	}
	if len(bs) != 1 || bs[0].Name != "main" || bs[0].CommitSHA != "abc" {
		t.Fatalf("branches mal parsées : %+v", bs)
	}
}

func TestSafePathRejectsTraversal(t *testing.T) {
	for _, bad := range []string{"../etc", "a/../b", "a/./b", "a//b", "/.."} {
		if err := safePath(bad); err == nil {
			t.Errorf("safePath(%q) devrait être refusé", bad)
		}
	}
	for _, ok := range []string{"", "README.md", "src/app/main.go"} {
		if err := safePath(ok); err != nil {
			t.Errorf("safePath(%q) devrait passer : %v", ok, err)
		}
	}
}

func TestNormState(t *testing.T) {
	cases := map[string]string{"": "open", "open": "open", "closed": "closed", "all": "all", "bidon": "open", "open&x=1": "open"}
	for in, want := range cases {
		if got := normState(in); got != want {
			t.Errorf("normState(%q) = %q, attendu %q", in, got, want)
		}
	}
}
