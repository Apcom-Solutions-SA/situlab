// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package forgejo

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// fakeCommit décrit un commit du faux serveur /commits (date d'auteur RFC3339 + volumes).
type fakeCommit struct {
	sha        string
	authorName string
	login      string
	date       string
	adds, dels int
}

// pageRec mémorise les pages réellement demandées. Le serveur httptest sert chaque requête
// dans sa propre goroutine, d'où le verrou.
type pageRec struct {
	mu  sync.Mutex
	max int
}

func (p *pageRec) observe(page int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if page > p.max {
		p.max = page
	}
}

func (p *pageRec) maxPage() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.max
}

// commitsServer monte un faux Forgejo dont /commits renvoie, pour la page demandée, les commits
// produits par pageFn. Forgejo ignore since/until côté serveur : pageFn ne les lit donc pas, c'est
// le filtrage client de ListCommitsRange qui est éprouvé ici.
func commitsServer(t *testing.T, pageFn func(page int) []fakeCommit) (*Client, *pageRec) {
	t.Helper()
	rec := &pageRec{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repos/apcom/glim/commits" {
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"message":"not found"}`)
			return
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		rec.observe(page)
		writeCommits(w, pageFn(page))
	}))
	t.Cleanup(srv.Close)
	return NewKind(srv.URL, "tok", "test", KindForgejo), rec
}

// writeCommits sérialise une page au format attendu par apiCommitStat (stat=true).
func writeCommits(w http.ResponseWriter, cs []fakeCommit) {
	var b strings.Builder
	b.WriteByte('[')
	for i, c := range cs {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b,
			`{"sha":%q,"commit":{"author":{"name":%q,"date":%q}},"author":{"login":%q},"stats":{"additions":%d,"deletions":%d}}`,
			c.sha, c.authorName, c.date, c.login, c.adds, c.dels)
	}
	b.WriteByte(']')
	io.WriteString(w, b.String())
}

// find localise un commit du résultat par son SHA.
func find(out []CommitStat, sha string) (CommitStat, bool) {
	for _, cs := range out {
		if cs.SHA == sha {
			return cs, true
		}
	}
	return CommitStat{}, false
}

// Fenêtre [since, until] : un commit trop récent est écarté sans stopper la pagination, les
// commits dans la fenêtre ressortent avec leurs volumes, et le premier commit antérieur à since
// arrête la pagination (reachedOlder). La page 1 est PLEINE (perPage) : seul reachedOlder peut
// donc interrompre le parcours, et la page 2 — servie mais défendue d'être lue — ne doit jamais
// être demandée.
func TestListCommitsRangeWindow(t *testing.T) {
	page1 := []fakeCommit{
		{sha: "recent", authorName: "Zoe", login: "zoe", date: "2026-02-01T08:00:00Z", adds: 100, dels: 100}, // après until
		{sha: "winA", authorName: "Ana", login: "ana", date: "2026-01-18T09:00:00Z", adds: 10, dels: 2},
		{sha: "winB", authorName: "Bo", login: "bo", date: "2026-01-12T09:00:00Z", adds: 4, dels: 1},
		{sha: "old", authorName: "Ed", login: "ed", date: "2026-01-01T09:00:00Z", adds: 7, dels: 7}, // avant since
	}
	// Bourrage jusqu'à perPage, tous antérieurs à since : la page reste pleine sans fausser la fenêtre.
	for i := len(page1); i < perPage; i++ {
		page1 = append(page1, fakeCommit{sha: fmt.Sprintf("pad-%d", i), authorName: "x", date: "2025-12-15T00:00:00Z", adds: 1, dels: 1})
	}
	c, rec := commitsServer(t, func(page int) []fakeCommit {
		if page == 1 {
			return page1
		}
		// Page 2 : un commit dans la fenêtre qui surgirait dans le résultat si la pagination
		// ne s'était pas arrêtée sur reachedOlder.
		return []fakeCommit{{sha: "winC", authorName: "Cy", login: "cy", date: "2026-01-15T09:00:00Z", adds: 99, dels: 99}}
	})

	out, truncated, err := c.ListCommitsRange(context.Background(), "apcom", "glim", "main",
		"2026-01-10T00:00:00Z", "2026-01-20T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if truncated {
		t.Error("tronqué attendu à false (arrêt propre sur reachedOlder)")
	}
	if rec.maxPage() != 1 {
		t.Fatalf("la page %d a été demandée : reachedOlder n'a pas arrêté la pagination", rec.maxPage())
	}
	if len(out) != 2 {
		t.Fatalf("2 commits attendus dans la fenêtre, obtenu %d : %+v", len(out), out)
	}
	if _, ok := find(out, "recent"); ok {
		t.Error("le commit postérieur à until doit être exclu")
	}
	if _, ok := find(out, "winC"); ok {
		t.Error("winC (page 2) ne doit pas être lu après reachedOlder")
	}
	a, _ := find(out, "winA")
	if a.Additions != 10 || a.Deletions != 2 || a.AuthorName != "Ana" || a.AuthorUser != "ana" {
		t.Errorf("winA mal rapporté : %+v", a)
	}
	b, _ := find(out, "winB")
	if b.Additions != 4 || b.Deletions != 1 {
		t.Errorf("winB : volumes attendus 4/1, obtenu %d/%d", b.Additions, b.Deletions)
	}
	// Ordre préservé (du plus récent au plus ancien).
	if out[0].SHA != "winA" || out[1].SHA != "winB" {
		t.Errorf("ordre inattendu : %s puis %s", out[0].SHA, out[1].SHA)
	}
}

// Fin de données « propre » : une dernière page partielle (moins de perPage) clôt le parcours
// sans toucher maxPages, donc tronqué=false. On vérifie aussi l'accumulation sur plusieurs pages.
func TestListCommitsRangePartialPageNotTruncated(t *testing.T) {
	inWindow := func(n int, prefix string) []fakeCommit {
		out := make([]fakeCommit, n)
		for i := range out {
			out[i] = fakeCommit{sha: fmt.Sprintf("%s-%d", prefix, i), authorName: "Ana", login: "ana", date: "2026-06-15T12:00:00Z", adds: 2, dels: 1}
		}
		return out
	}
	c, rec := commitsServer(t, func(page int) []fakeCommit {
		switch page {
		case 1:
			return inWindow(perPage, "p1") // page pleine
		case 2:
			return inWindow(7, "p2") // page partielle → fin propre
		default:
			return nil
		}
	})

	out, truncated, err := c.ListCommitsRange(context.Background(), "apcom", "glim", "",
		"2026-01-01T00:00:00Z", "2026-12-31T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if truncated {
		t.Error("tronqué attendu à false sur une dernière page partielle")
	}
	if want := perPage + 7; len(out) != want {
		t.Fatalf("%d commits attendus, obtenu %d", want, len(out))
	}
	if rec.maxPage() != 2 {
		t.Errorf("pagination jusqu'à la page %d, attendu 2", rec.maxPage())
	}
	if out[0].Additions != 2 || out[0].Deletions != 1 {
		t.Errorf("volumes perdus : %+v", out[0])
	}
}

// Résultat partiel : si chaque page reste pleine jusqu'à la borne maxPages, le parcours s'arrête
// sur le garde-fou et tronqué=true. La fenêtre couvre large pour n'écarter aucun commit.
func TestListCommitsRangeTruncatedAtMaxPages(t *testing.T) {
	c, rec := commitsServer(t, func(page int) []fakeCommit {
		out := make([]fakeCommit, perPage) // toujours une page pleine
		for i := range out {
			out[i] = fakeCommit{sha: fmt.Sprintf("c-%d-%d", page, i), authorName: "Ana", date: "2026-06-15T12:00:00Z", adds: 1, dels: 0}
		}
		return out
	})

	out, truncated, err := c.ListCommitsRange(context.Background(), "apcom", "glim", "",
		"2000-01-01T00:00:00Z", "2100-01-01T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if !truncated {
		t.Error("tronqué attendu à true quand la borne maxPages est atteinte")
	}
	if want := perPage * maxPages; len(out) != want {
		t.Fatalf("%d commits attendus (perPage*maxPages), obtenu %d", want, len(out))
	}
	if rec.maxPage() != maxPages {
		t.Errorf("dernière page demandée %d, attendu %d (pas de lecture au-delà de la borne)", rec.maxPage(), maxPages)
	}
}

// LatestTag ne doit lire qu'UNE page d'UN élément (l'API renvoie les tags du plus récent au plus
// ancien) : on vérifie le tag retourné, et qu'aucune pagination n'a lieu (perf 1.17).
func TestLatestTagSinglePage(t *testing.T) {
	var pages int
	var gotPage, gotLimit string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/repos/apcom/glim/tags") {
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"message":"not found"}`)
			return
		}
		pages++
		gotPage = r.URL.Query().Get("page")
		gotLimit = r.URL.Query().Get("limit")
		io.WriteString(w, `[{"name":"v1.4.0","commit":{"sha":"aaa"}},{"name":"v1.3.0","commit":{"sha":"bbb"}}]`)
	}))
	t.Cleanup(srv.Close)
	c := NewKind(srv.URL, "tok", "test", KindForgejo)

	tag, ok, err := c.LatestTag(context.Background(), "apcom", "glim")
	if err != nil {
		t.Fatalf("LatestTag: %v", err)
	}
	if !ok {
		t.Fatal("ok attendu à true (le dépôt a des tags)")
	}
	if tag.Name != "v1.4.0" {
		t.Fatalf("dernier tag = %q, attendu v1.4.0", tag.Name)
	}
	if pages != 1 {
		t.Fatalf("%d requêtes servies, attendu 1 (aucune pagination)", pages)
	}
	if gotPage != "1" || gotLimit != "1" {
		t.Fatalf("requête page=%q limit=%q, attendu page=1 limit=1", gotPage, gotLimit)
	}
}

// GitHub ne garantit aucun ordre sur /tags : latestSemverTag doit retenir la plus grande VERSION
// sémantique quel que soit l'ordre, en IGNORANT les tags non sémantiques (latest, nightly…) — y compris
// quand un tel tag arrive EN PREMIER (régression 1.18 : il ancrait le résultat et n'était jamais dépassé).
func TestLatestSemverTag(t *testing.T) {
	mk := func(names ...string) []apiTag {
		out := make([]apiTag, len(names))
		for i, n := range names {
			out[i] = apiTag{Name: n}
			out[i].Commit.SHA = n
		}
		return out
	}
	cases := []struct {
		names []string
		want  string
	}{
		{[]string{"latest", "v1.0.0", "v2.0.0"}, "v2.0.0"},     // non-sémantique en tête, ignoré
		{[]string{"nightly", "v1.2.0", "v1.3.0"}, "v1.3.0"},    // idem
		{[]string{"v1.0.0", "latest", "v2.0.0"}, "v2.0.0"},     // non-sémantique au milieu
		{[]string{"v2.0.0", "v1.0.0", "latest"}, "v2.0.0"},     // sémantique en tête, ordre décroissant
		{[]string{"v1.0.0", "v1.0.1-rc1", "v1.1.0"}, "v1.1.0"}, // pré-version < release
		{[]string{"v1.2.0", "v1.2.0-beta"}, "v1.2.0"},          // release l'emporte sur sa pré-version
		{[]string{"latest", "nightly"}, "latest"},              // AUCUN sémantique → repli sur le 1er
	}
	for _, tc := range cases {
		if got := latestSemverTag(mk(tc.names...)); got.Name != tc.want {
			t.Errorf("latestSemverTag(%v) = %q, attendu %q", tc.names, got.Name, tc.want)
		}
	}
}

// Dépôt sans aucun tag : ok=false, sans erreur.
func TestLatestTagNone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `[]`)
	}))
	t.Cleanup(srv.Close)
	c := NewKind(srv.URL, "tok", "test", KindForgejo)

	_, ok, err := c.LatestTag(context.Background(), "apcom", "glim")
	if err != nil {
		t.Fatalf("LatestTag: %v", err)
	}
	if ok {
		t.Fatal("ok attendu à false (aucun tag)")
	}
}

// Le sceau d'origine se détecte sur le message COMPLET (le trailer est sur une ligne après le sujet) :
// sans quoi l'historique en ligne (message tronqué au sujet) ne le montrerait jamais.
func TestHasApprovedTrailer(t *testing.T) {
	signed := "Promotion v1.16.4 (depuis l'atelier)\n\nSituLab-Approved: eyJjZXJ0Ijp..."
	if !hasApprovedTrailer(signed) {
		t.Fatal("trailer d'attestation non détecté sur un message complet signé")
	}
	if hasApprovedTrailer("Un commit ordinaire\n\nsans sceau") {
		t.Fatal("faux positif : trailer détecté sur un message non signé")
	}
	if hasApprovedTrailer("SituLab-Approved juste dans le sujet, pas un trailer") {
		t.Fatal("faux positif : « SituLab-Approved » sans deux-points n'est pas le trailer")
	}
}
