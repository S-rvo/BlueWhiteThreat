// exportpipeline freezes an unlabeled corpus from the actual article pipeline.
// It does not export existing NLP verdicts, so downstream annotation stays blind.
package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"sort"
	"time"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/bootstrap"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/db"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/dedup"
	"github.com/joho/godotenv"
)

type document struct {
	ID          string `json:"id"`
	Group       string `json:"group"`
	Text        string `json:"text"`
	URL         string `json:"url"`
	Source      string `json:"source"`
	SourceType  string `json:"source_type"`
	CollectedAt string `json:"collected_at,omitempty"`
}

func readExcludedNormalized(path string) (map[string]bool, error) {
	excluded := map[string]bool{}
	if path == "" {
		return excluded, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	for {
		var d struct {
			Text string `json:"text"`
		}
		if err := dec.Decode(&d); err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		if normalized := dedup.Normalize(d.Text); normalized != "" {
			excluded[normalized] = true
		}
	}
	return excluded, nil
}

func pipelineDocument(article db.ArticleContent) (document, bool) {
	if article.Content == "" || bootstrap.IsLinkDirectory(article.Content) || bootstrap.IsGatedLoginPage(article.Content) {
		return document{}, false
	}
	sourceType := article.SourceType
	if sourceType == "" {
		sourceType = "darkweb_page"
	}
	host := "unknown"
	if parsed, err := url.Parse(article.URL); err == nil && parsed.Hostname() != "" {
		host = parsed.Hostname()
	}
	digest := sha256.Sum256([]byte(article.URL + "\x00" + dedup.Normalize(article.Content)))
	d := document{ID: "pipeline:" + hex.EncodeToString(digest[:12]), Group: "host:" + host,
		Text: article.Content, URL: article.URL, Source: sourceType, SourceType: sourceType}
	if !article.ScrapedAt.IsZero() {
		d.CollectedAt = article.ScrapedAt.UTC().Format(time.RFC3339Nano)
	}
	return d, true
}

func main() {
	// Match the classifier service: load credentials from the local project
	// file while preserving any explicit environment overrides (notably
	// MONGO_HOST=localhost when this command runs outside Compose).
	_ = godotenv.Load()
	out := flag.String("out", "", "new JSONL output path")
	limit := flag.Int("limit", 0, "maximum documents after filtering; zero keeps all")
	exclude := flag.String("exclude", "", "JSONL corpus whose normalized texts must be excluded")
	flag.Parse()
	if *out == "" || *limit < 0 {
		fmt.Fprintln(os.Stderr, "valid -out and nonnegative -limit required")
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := db.Connect(ctx)
	if err != nil {
		panic(err)
	}
	defer client.Disconnect(ctx)
	articles, err := db.FetchAllContent(ctx, client)
	if err != nil {
		panic(err)
	}
	excluded, err := readExcludedNormalized(*exclude)
	if err != nil {
		panic(err)
	}
	documents := make([]document, 0, len(articles))
	seen := map[string]bool{}
	for _, article := range articles {
		d, ok := pipelineDocument(article)
		fingerprint := dedup.Normalize(article.Content)
		if !ok || seen[fingerprint] || excluded[fingerprint] {
			continue
		}
		seen[fingerprint] = true
		documents = append(documents, d)
	}
	sort.Slice(documents, func(i, j int) bool {
		if documents[i].CollectedAt != documents[j].CollectedAt {
			return documents[i].CollectedAt < documents[j].CollectedAt
		}
		return documents[i].ID < documents[j].ID
	})
	if *limit > 0 && len(documents) > *limit {
		documents = documents[len(documents)-*limit:]
	}
	f, err := os.OpenFile(*out, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	enc := json.NewEncoder(w)
	for _, d := range documents {
		if err := enc.Encode(d); err != nil {
			panic(err)
		}
	}
	if err := w.Flush(); err != nil {
		panic(err)
	}
	fmt.Printf("{\"exported\":%d,\"excluded_fingerprints\":%d,\"labels_exported\":false}\n", len(documents), len(excluded))
}
