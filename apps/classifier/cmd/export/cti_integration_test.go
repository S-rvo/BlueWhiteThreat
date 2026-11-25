//go:build integration

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestSmokeCTIReviewExport(t *testing.T) {
	if os.Getenv("BWT_DISPOSABLE_PIPELINE") != "1" || os.Getenv("MONGO_HOST") != "127.0.0.1" ||
		os.Getenv("MONGO_PORT") == "" || os.Getenv("MONGO_PORT") == "27017" {
		t.Skip("requires tests/pipeline/smoke.py with disposable MongoDB on loopback")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client, err := db.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Disconnect(ctx) }()
	name := "cti_review_smoke_" + primitive.NewObjectID().Hex()
	database := client.Database(name)
	admin := client.Database("admin")
	password := primitive.NewObjectID().Hex()
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = admin.RunCommand(cleanup, bson.D{{Key: "dropUser", Value: name}}).Err()
		_ = database.Drop(cleanup)
	}()
	if err := admin.RunCommand(ctx, bson.D{
		{Key: "createUser", Value: name},
		{Key: "pwd", Value: password},
		{Key: "roles", Value: bson.A{bson.M{"role": "read", "db": name}}},
	}).Err(); err != nil {
		t.Fatal("create fixture read-only user:", err)
	}

	content := "This synthetic ransomware report mentions CVE-2000-1234 and CERTFR-2000-ALE-001."
	_, err = database.Collection("article").InsertMany(ctx, []any{
		bson.M{"_id": "z-flagged", "url": "https://example.invalid/flagged", "content": content,
			"relevance": bson.M{"needs_review": true, "relevant": false}},
		bson.M{"_id": "a-unflagged", "url": "https://example.invalid/unflagged", "content": content,
			"relevance": bson.M{"needs_review": false, "relevant": true}},
	})
	if err != nil {
		t.Fatal(err)
	}

	work := t.TempDir()
	write := func(file, content string) string {
		t.Helper()
		path := filepath.Join(work, file)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	kevPath := write("kev.json", testKEV)
	// Both candidates meet the existing bootstrap gate without negative sampling.
	keywords := write("keywords.json", `["ransomware","report","cve"]`)
	// Use the fixed one-token model in all modes, never a checkout artifact.
	modelPath, err := filepath.Abs("../../../../tests/pipeline/model.fixture.json")
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(work, "export")
	if out, err := exec.CommandContext(ctx, "go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build export: %v\n%s", err, out)
	}

	run := func(output string, args ...string) (string, error) {
		t.Helper()
		cmd := exec.CommandContext(ctx, binary, args...)
		cmd.Dir = work
		cmd.Env = append(os.Environ(),
			"MONGO_DB="+name, "MONGO_INITDB_ROOT_USERNAME="+name, "MONGO_INITDB_ROOT_PASSWORD="+password,
			"MODEL_PATH="+modelPath, "SEED_KEYWORDS_PATH="+keywords,
			"REVIEWED_LABELS_PATH="+filepath.Join(work, "nonexistent-reviewed.csv"),
			"EXCLUDED_IDS_PATH="+filepath.Join(work, "nonexistent-excluded.csv"),
			"EXPORT_NEGATIVE_SAMPLE=0", "EXPORT_OUTPUT="+output,
		)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	read := func(name string) ([]byte, []ReviewCandidate) {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(work, name))
		if err != nil {
			t.Fatal(err)
		}
		var records []ReviewCandidate
		decoder := json.NewDecoder(bytes.NewReader(raw))
		for {
			var record ReviewCandidate
			if err := decoder.Decode(&record); err != nil {
				if err == io.EOF {
					break
				}
				t.Fatal(err)
			}
			records = append(records, record)
		}
		return raw, records
	}
	const baseline = "plain.jsonl"
	const enriched = "with-context.jsonl"
	if out, err := run(baseline); err != nil {
		t.Fatalf("plain export: %v\n%s", err, out)
	}
	if out, err := run(enriched, "-cti-context", "-kev-file", kevPath); err != nil {
		t.Fatalf("CTI export: %v\n%s", err, out)
	}
	rawBase, base := read(baseline)
	rawCTI, got := read(enriched)
	if len(base) != 2 || len(got) != 2 {
		t.Fatalf("expected two rows in each export: baseline=%d enriched=%d", len(base), len(got))
	}
	if base[0].ID != "z-flagged" || base[1].ID != "a-unflagged" ||
		base[0].ModelScore == 0 || bytes.Contains(rawBase, []byte(`"cti_context"`)) {
		t.Fatal("baseline must prioritize the flagged document and score it without CTI context")
	}
	reviewCTI := got[0].CTIContext
	if reviewCTI == nil || !reflect.DeepEqual(reviewCTI.CVEs, []cveReference{{"CVE-2000-1234", true}}) ||
		len(reviewCTI.CERTFRRefs) != 1 || got[1].CTIContext != nil {
		t.Fatal("unexpected enrichment: want only the flagged document with one CVE and one CERT-FR reference")
	}
	got[0].CTIContext = nil
	if !reflect.DeepEqual(base, got) {
		t.Fatal("context changed existing review fields or document order")
	}
	info, err := os.Stat(filepath.Join(work, enriched))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("enriched export must have mode 0600: %v, %v", info, err)
	}
	if _, err := run(enriched, "-cti-context", "-kev-file", kevPath); err == nil {
		t.Fatal("an existing enriched export was accepted as output")
	}
	if after, _ := read(enriched); !bytes.Equal(rawCTI, after) {
		t.Fatal("existing export changed after an attempt to overwrite it")
	}
}
