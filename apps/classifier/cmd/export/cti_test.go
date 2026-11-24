package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

const testKEV = `{
  "dateReleased":"2026-11-23T19:43:55Z",
  "count":1,
  "vulnerabilities":[{"cveID":"CVE-2000-1234"}]
}`

func TestReviewContextEnrichesOnlyFlaggedCandidates(t *testing.T) {
	kev, err := decodeKEV(strings.NewReader(testKEV))
	if err != nil {
		t.Fatal(err)
	}
	c := ReviewCandidate{
		ID: "fixture", URL: "https://example.invalid/report",
		Content: "cve-2000-1234, CVE-2000-1234; (CVE-2001-12345), badCVE-2002-1234, " +
			"CVE-2003-1111bad et certfr-2000-ale-001, CERTFR-2001-AVI-0002",
		HeuristicLabel: 1, ModelScore: 0.48, NeedsReview: true,
	}
	c.CTIContext = contextForReview(c, kev)
	want := &reviewContext{
		CVEs:            []cveReference{{"CVE-2000-1234", true}, {"CVE-2001-12345", false}},
		KEVReleasedAt:   time.Date(2026, 11, 23, 19, 43, 55, 0, time.UTC),
		CERTFRReviewURL: "https://cert.cyber.gouv.fr/avis/",
		CERTFRRefs: []certFRReference{
			{"CERTFR-2000-ALE-001", "https://cert.cyber.gouv.fr/alerte/CERTFR-2000-ALE-001/"},
			{"CERTFR-2001-AVI-0002", "https://cert.cyber.gouv.fr/avis/CERTFR-2001-AVI-0002/"},
		},
	}
	if !reflect.DeepEqual(c.CTIContext, want) {
		t.Fatalf("got %+v, want %+v", c.CTIContext, want)
	}
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"listed_in_kev":false`) {
		t.Fatal("KEV absence must be an explicit false")
	}
	var decoded ReviewCandidate
	if err := json.Unmarshal(raw, &decoded); err != nil || !reflect.DeepEqual(c, decoded) {
		t.Fatalf("round trip changed the review candidate: %v", err)
	}
	if got := contextForReview(c, nil); got != nil {
		t.Fatal("disabled option returned context")
	}
	c.NeedsReview = false
	if got := contextForReview(c, kev); got != nil {
		t.Fatal("unflagged candidate returned context")
	}
	c.NeedsReview, c.Content = true, "Sans référence"
	if got := contextForReview(c, kev); got != nil {
		t.Fatal("candidate without references returned context")
	}
}

func TestReviewIDsAreBounded(t *testing.T) {
	body := strings.Builder{}
	body.WriteString("cve-2000-1000 CVE-2000-1000 ")
	for i := 1000; i < 1025; i++ {
		fmt.Fprintf(&body, "CVE-2000-%d ", i)
	}
	ids := reviewIDs(cveMention, body.String())
	if len(ids) != maxReviewReferences || ids[0] != "CVE-2000-1000" || ids[19] != "CVE-2000-1019" {
		t.Fatalf("unexpected ids: %v", ids)
	}
}

func TestReadKEVLocalSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kev.json")
	if _, err := readKEV(context.Background(), path); !os.IsNotExist(err) {
		t.Fatalf("missing local file must not fall back to the network: %v", err)
	}
	if err := os.WriteFile(path, []byte(testKEV), 0o600); err != nil {
		t.Fatal(err)
	}
	kev, err := readKEV(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := kev.listed["CVE-2000-1234"]; !ok || len(kev.listed) != 1 {
		t.Fatal("local snapshot did not load the fixture CVE")
	}
}

func TestReviewContextSupportsCERTFROnly(t *testing.T) {
	kev, err := decodeKEV(strings.NewReader(testKEV))
	if err != nil {
		t.Fatal(err)
	}
	candidate := ReviewCandidate{NeedsReview: true, Content: "CERTFR-2000-ALE-001; certfr-2000-ale-001"}
	got := contextForReview(candidate, kev)
	if got == nil || len(got.CVEs) != 0 || len(got.CERTFRRefs) != 1 ||
		got.CERTFRRefs[0].URL != "https://cert.cyber.gouv.fr/alerte/CERTFR-2000-ALE-001/" {
		t.Fatalf("unexpected CERT-FR-only context: %+v", got)
	}
}

func TestKEVRejectsInvalidSnapshots(t *testing.T) {
	inputs := map[string]string{
		"invalid JSON":   "{",
		"count mismatch": strings.Replace(testKEV, `"count":1`, `"count":2`, 1),
		"invalid ID":     strings.Replace(testKEV, "CVE-2000-1234", "CVE-2000-1234/x", 1),
		"duplicate":      `{"dateReleased":"2026-11-23T19:43:55Z","count":2,"vulnerabilities":[{"cveID":"cve-2000-1234"},{"cveID":"CVE-2000-1234"}]}`,
		"too large":      strings.Repeat(" ", maxKEVBytes+1),
	}
	for name, body := range inputs {
		t.Run(name, func(t *testing.T) {
			if kev, err := decodeKEV(strings.NewReader(body)); err == nil || kev != nil {
				t.Fatal("invalid catalogue accepted")
			}
		})
	}
}

func TestKEVPublisherSnapshot(t *testing.T) {
	if os.Getenv("BWT_TEST_PUBLIC_KEV") != "1" {
		t.Skip("public feed test not requested")
	}
	kev, err := readKEV(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("official KEV snapshot: %d entries, released %s", len(kev.listed), kev.releasedAt.Format(time.RFC3339))
}
