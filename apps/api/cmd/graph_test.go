package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeGraph struct {
	rows    []graphArticle
	limit   int
	failure error
}

func (g *fakeGraph) articles(_ context.Context, limit int) ([]graphArticle, error) {
	g.limit = limit
	return g.rows, g.failure
}

func getGraph(t *testing.T, h http.Handler, path, origin string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestGraphShapeBoundsAndFiltering(t *testing.T) {
	many := make([]graphEntity, 0, maxEntitiesPerArticle+5)
	for i := 0; i < maxEntitiesPerArticle+5; i++ {
		many = append(many, graphEntity{Kind: "Hash", Value: strings.Repeat("a", i+1)})
	}
	graph := &fakeGraph{rows: []graphArticle{
		{ID: "a2", Entities: []graphEntity{
			{Kind: "IP", Value: "192.0.2.1"},
			{Kind: "URLIndicator", Value: "http://hidden.invalid"},
			{Kind: "URL", Value: "http://crawler.invalid"},
			{Kind: "CVE", Value: ""},
		}},
		{ID: "a1", Entities: []graphEntity{{Kind: "IP", Value: "192.0.2.1"}}},
		{ID: "a0", Entities: many},
	}}
	h := newHandler(&fakeStore{}, graph)
	w := getGraph(t, h, "/graph?limit=2", "http://127.0.0.1:8080")
	if w.Code != http.StatusOK || graph.limit != 2 ||
		w.Header().Get("Access-Control-Allow-Origin") != "http://127.0.0.1:8080" ||
		w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("bad response: %d %v", w.Code, w.Header())
	}
	if strings.Contains(w.Body.String(), "invalid") {
		t.Fatalf("URL leaked: %s", w.Body)
	}
	var got graphResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	// The store returned 3 articles for limit=2: the handler must cut it.
	want := graphResponse{
		Nodes: []graphNode{
			{ID: "Article:a2", Kind: "Article", Label: "a2"},
			{ID: "IP:192.0.2.1", Kind: "IP", Label: "192.0.2.1"},
			{ID: "Article:a1", Kind: "Article", Label: "a1"},
		},
		Edges: []graphEdge{{From: "IP:192.0.2.1", To: "Article:a2"}, {From: "IP:192.0.2.1", To: "Article:a1"}},
	}
	if len(got.Nodes) != len(want.Nodes) || len(got.Edges) != len(want.Edges) {
		t.Fatalf("got %+v; want %+v", got, want)
	}
	for i := range want.Nodes {
		if got.Nodes[i] != want.Nodes[i] {
			t.Fatalf("node %d = %+v; want %+v", i, got.Nodes[i], want.Nodes[i])
		}
	}
	for i := range want.Edges {
		if got.Edges[i] != want.Edges[i] {
			t.Fatalf("edge %d = %+v; want %+v", i, got.Edges[i], want.Edges[i])
		}
	}

	if g := buildGraph(graph.rows, 3); len(g.Edges) != 2+maxEntitiesPerArticle {
		t.Fatalf("per-article cap not applied: %d edges", len(g.Edges))
	}

	getGraph(t, h, "/graph", "")
	if graph.limit != defaultGraphArticles {
		t.Fatalf("default limit = %d", graph.limit)
	}
	for _, path := range []string{"/graph?limit=0", "/graph?limit=201", "/graph?limit=x",
		"/graph?limit=1&limit=2", "/graph?limit=%zz"} {
		if w := getGraph(t, h, path, ""); w.Code != http.StatusBadRequest {
			t.Errorf("%s status = %d", path, w.Code)
		}
	}

	graph.rows = nil
	w = getGraph(t, h, "/graph", "https://unrelated.invalid")
	if w.Body.String() != `{"nodes":[],"edges":[]}`+"\n" || w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("empty graph: %q %v", w.Body, w.Header())
	}
}

func TestGraphUnavailable(t *testing.T) {
	for _, graph := range []graphStore{unavailableGraph{}, &fakeGraph{failure: errors.New("bolt: secret details")}} {
		w := getGraph(t, newHandler(&fakeStore{}, graph), "/graph", "")
		if w.Code != http.StatusServiceUnavailable || strings.Contains(w.Body.String(), "secret") {
			t.Fatalf("%T: %d %s", graph, w.Code, w.Body)
		}
	}
}
