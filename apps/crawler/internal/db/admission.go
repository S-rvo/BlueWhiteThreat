package db

import (
	"net/url"
	"strings"
)

// discoveryKey rejects common crawl traps without stripping meaningful query
// parameters. Rejected URLs are counted, not silently reported as duplicates.
func discoveryKey(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") || !strings.HasSuffix(strings.ToLower(u.Hostname()), ".onion") {
		return "", false
	}
	if len(raw) > 4096 || len(u.Query()) > 12 {
		return "", false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) > 24 {
		return "", false
	}
	counts := map[string]int{}
	for _, part := range parts {
		if part != "" {
			counts[part]++
			if counts[part] > 4 {
				return "", false
			}
		}
	}
	for key := range u.Query() {
		switch strings.ToLower(key) {
		case "phpsessid", "jsessionid", "sessionid":
			return "", false
		}
	}
	return strings.ToLower(u.Host), true
}
