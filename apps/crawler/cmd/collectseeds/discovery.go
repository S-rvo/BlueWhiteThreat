package main

import (
	"html"
	"net/url"
	"regexp"
	"strings"
)

var fullOnionURL = regexp.MustCompile(`(?i)https?://[a-z2-7]{56}\.onion(?::[0-9]+)?(?:[/?#][^\s<>"']*)?`)
var bareOnion = regexp.MustCompile(`(?i)\b[a-z2-7]{56}\.onion\b`)

// extractSeeds preserves paths, query strings and HTTPS. Bare mentions are
// only promoted to roots when that host has no explicit URL in the input.
func extractSeeds(text string, found map[string]bool) {
	text = html.UnescapeString(strings.ReplaceAll(text, `\/`, `/`))
	hosts := map[string]bool{}
	for _, raw := range fullOnionURL.FindAllString(text, -1) {
		raw = strings.TrimRight(raw, ").,;]}")
		u, err := url.Parse(raw)
		if err != nil || u.User != nil {
			continue
		}
		u.Scheme = strings.ToLower(u.Scheme)
		u.Host = strings.ToLower(u.Host)
		u.Fragment = ""
		if u.Path == "" {
			u.Path = "/"
		}
		hosts[u.Hostname()] = true
		found[u.String()] = true
	}
	for _, host := range bareOnion.FindAllString(text, -1) {
		host = strings.ToLower(host)
		if !hosts[host] {
			found["http://"+host+"/"] = true
		}
	}
}
