//go:build integration

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// Runs after the parser smoke projected 192.0.2.42 onto the positive article.
func TestSmokeGraphContract(t *testing.T) {
	if os.Getenv("BWT_DISPOSABLE_PIPELINE") != "1" {
		t.Skip("run tests/pipeline/smoke.py against disposable databases")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	driver, err := neo4j.NewDriverWithContext(os.Getenv("NEO4J_URI"),
		neo4j.BasicAuth("neo4j", os.Getenv("NEO4J_PASSWORD"), ""))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = driver.Close(context.Background()) }()

	// URL indicators and crawler URL nodes linked to an article must never be exposed.
	if _, err := neo4j.ExecuteQuery(ctx, driver, `
		MATCH (a:Article)<-[:MENTIONED_IN]-(:IP {value: '192.0.2.42'})
		MERGE (u:URLIndicator {value: 'http://indicator.pipeline.invalid'})-[:MENTIONED_IN]->(a)
		MERGE (c:URL {url: 'http://crawler.pipeline.invalid'})-[:MENTIONED_IN]->(a)`,
		nil, neo4j.EagerResultTransformer); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = neo4j.ExecuteQuery(context.Background(), driver, `
			MATCH (n) WHERE n.value = 'http://indicator.pipeline.invalid' OR n.url = 'http://crawler.pipeline.invalid'
			DETACH DELETE n`, nil, neo4j.EagerResultTransformer)
	}()

	handler := newHandler(&fakeStore{}, neo4jGraph{driver})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/graph?limit=10", nil))
	if w.Code != http.StatusOK || strings.Contains(w.Body.String(), "pipeline.invalid") {
		t.Fatalf("/graph: %d %s", w.Code, w.Body)
	}
	var got graphResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	var ip, article string
	for _, n := range got.Nodes {
		switch n.Kind {
		case "IP":
			if n.Label == "192.0.2.42" {
				ip = n.ID
			}
		case "Article":
			article = n.ID
		}
	}
	if ip == "" || article == "" || len(got.Edges) != 1 || got.Edges[0] != (graphEdge{From: ip, To: article}) {
		t.Fatalf("projected IOC missing from graph: %s", w.Body)
	}
}
