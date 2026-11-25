package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// collectSearch uses only the documented search API. A hard request budget
// applies across queries; errors (including 429) stop this provider for the run.
func collectSearch(ctx context.Context, client *http.Client, key string, queries []string, budget int, interval time.Duration, found map[string]bool) error {
	if key == "" {
		return nil
	}
	if budget < 1 || budget > 100 {
		return fmt.Errorf("search budget must be 1..100")
	}
	// Do not forward the API credential across redirects, including same-host ones.
	private := *client
	private.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	used := 0
	done := make(map[int]bool)
	for page := 1; used < budget && len(done) < len(queries); page++ {
		for i, query := range queries {
			if done[i] {
				continue
			}
			if used > 0 {
				timer := time.NewTimer(interval)
				select {
				case <-ctx.Done():
					timer.Stop()
					return ctx.Err()
				case <-timer.C:
				}
			}
			q := url.Values{"q": {query}, "page": {strconv.Itoa(page)}}
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://onionsearchengine.com/api.php?"+q.Encode(), nil)
			if err != nil {
				return err
			}
			req.Header.Set("X-API-Key", key)
			resp, err := private.Do(req)
			used++
			if err != nil {
				return fmt.Errorf("search transport failed: %w", err)
			}
			body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
			readErr = errors.Join(readErr, resp.Body.Close())
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("search HTTP %d; provider stopped for this run", resp.StatusCode)
			}
			if readErr != nil {
				return readErr
			}
			if len(body) > maxBodyBytes {
				return fmt.Errorf("search response exceeds body limit")
			}
			var result struct {
				Status     string `json:"status"`
				Pagination struct {
					TotalPages int `json:"total_pages"`
				} `json:"pagination"`
				Results []struct {
					URL string `json:"url"`
				} `json:"results"`
			}
			if err := json.Unmarshal(body, &result); err != nil {
				return fmt.Errorf("invalid search JSON: %w", err)
			}
			if result.Status != "success" {
				return fmt.Errorf("search API reported failure")
			}
			for _, r := range result.Results {
				extractSeeds(r.URL, found)
			}
			if len(result.Results) == 0 || page >= result.Pagination.TotalPages {
				done[i] = true
			}
			if used >= budget {
				break
			}
		}
	}
	return nil
}
