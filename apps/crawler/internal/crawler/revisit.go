package crawler

import (
	"strconv"
	"time"
)

// RevisitInterval learns from the previous interval without interpreting a
// temporary network error as content stability.
func RevisitInterval(result Result, metadata map[string]string) time.Duration {
	if result.Denied || result.Status >= 400 {
		return 30 * 24 * time.Hour
	}
	previous := 24 * time.Hour
	if n, err := strconv.ParseInt(metadata["interval_seconds"], 10, 64); err == nil && n >= 3600 && n <= 30*86400 {
		previous = time.Duration(n) * time.Second
	}
	stable := result.Status == 304 || result.Hash != "" && result.Hash == metadata["hash"]
	if stable {
		previous *= 2
	} else if metadata["hash"] != "" {
		previous /= 2
	}
	if previous < time.Hour {
		previous = time.Hour
	}
	if previous > 30*24*time.Hour {
		previous = 30 * 24 * time.Hour
	}
	return previous
}
