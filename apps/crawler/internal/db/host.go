package db

import (
	"fmt"
	"net/url"
	"strings"
)

// HostKey intentionally groups schemes and explicit default ports conservatively.
func HostKey(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("invalid HTTP URL")
	}
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	if port != "" && port != "80" && port != "443" {
		host += ":" + port
	}
	return host, nil
}
