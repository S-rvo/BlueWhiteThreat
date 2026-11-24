package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

const (
	kevURL              = "https://raw.githubusercontent.com/cisagov/kev-data/develop/known_exploited_vulnerabilities.json"
	certFR              = "https://cert.cyber.gouv.fr"
	maxKEVBytes         = 4 << 20
	maxReviewReferences = 20
)

var (
	cveMention    = regexp.MustCompile(`(?i)\bCVE-[0-9]{4}-[0-9]{4,19}\b`)
	certFRMention = regexp.MustCompile(`(?i)\bCERTFR-[0-9]{4}-(?:ALE|AVI)-[0-9]{3,8}\b`)
)

type kevSnapshot struct {
	releasedAt time.Time
	listed     map[string]struct{}
}

type cveReference struct {
	ID          string `json:"id"`
	ListedInKEV bool   `json:"listed_in_kev"`
}

type certFRReference struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

type reviewContext struct {
	CVEs            []cveReference    `json:"cves,omitempty"`
	KEVReleasedAt   time.Time         `json:"kev_released_at"`
	CERTFRReviewURL string            `json:"cert_fr_review_url"`
	CERTFRRefs      []certFRReference `json:"cert_fr_references,omitempty"`
}

func readKEV(ctx context.Context, path string) (*kevSnapshot, error) {
	if path != "" {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		return decodeKEV(f)
	}

	// One fixed publisher request, no queries with identifiers from the export.
	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, kevURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("catalogue HTTP %d", resp.StatusCode)
	}
	return decodeKEV(resp.Body)
}

func decodeKEV(r io.Reader) (*kevSnapshot, error) {
	body, err := io.ReadAll(io.LimitReader(r, maxKEVBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxKEVBytes {
		return nil, fmt.Errorf("catalogue de plus de %d octets", maxKEVBytes)
	}
	var catalog struct {
		ReleasedAt time.Time `json:"dateReleased"`
		Count      int       `json:"count"`
		Entries    []struct {
			ID string `json:"cveID"`
		} `json:"vulnerabilities"`
	}
	if err := json.Unmarshal(body, &catalog); err != nil {
		return nil, fmt.Errorf("catalogue JSON invalide : %w", err)
	}
	if catalog.ReleasedAt.IsZero() || catalog.Count == 0 || catalog.Count != len(catalog.Entries) {
		return nil, fmt.Errorf("date ou nombre d’entrées KEV invalide")
	}

	out := &kevSnapshot{releasedAt: catalog.ReleasedAt.UTC(), listed: make(map[string]struct{}, catalog.Count)}
	for _, entry := range catalog.Entries {
		id := strings.ToUpper(entry.ID)
		if cveMention.FindString(id) != id || id == "" {
			return nil, fmt.Errorf("identifiant CVE invalide dans KEV")
		}
		out.listed[id] = struct{}{}
	}
	if len(out.listed) != catalog.Count {
		return nil, fmt.Errorf("identifiants en double dans KEV")
	}
	return out, nil
}

func contextForReview(candidate ReviewCandidate, kev *kevSnapshot) *reviewContext {
	if kev == nil || !candidate.NeedsReview {
		return nil
	}
	cves := reviewIDs(cveMention, candidate.Content)
	references := reviewIDs(certFRMention, candidate.Content)
	if len(cves) == 0 && len(references) == 0 {
		return nil
	}

	result := &reviewContext{KEVReleasedAt: kev.releasedAt, CERTFRReviewURL: certFR + "/avis/"}
	for _, id := range cves {
		_, listed := kev.listed[id]
		result.CVEs = append(result.CVEs, cveReference{ID: id, ListedInKEV: listed})
	}
	for _, id := range references {
		path := "/avis/"
		if strings.Contains(id, "-ALE-") {
			path = "/alerte/"
		}
		result.CERTFRRefs = append(result.CERTFRRefs, certFRReference{ID: id, URL: certFR + path + id + "/"})
	}
	return result
}

func reviewIDs(pattern *regexp.Regexp, body string) []string {
	var ids []string
	seen := make(map[string]bool)
	// Bound raw matches as well as the number of distinct references emitted.
	for _, raw := range pattern.FindAllString(body, 1024) {
		id := strings.ToUpper(raw)
		if !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
		if len(ids) == maxReviewReferences {
			break
		}
	}
	return ids
}
