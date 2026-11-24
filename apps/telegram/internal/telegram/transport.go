package telegram

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"golang.org/x/net/proxy"
)

const DefaultTorProxy = "socks5h://127.0.0.1:9051"

// NewTorClient routes every HTTP(S) connection through the configured SOCKS5
// endpoint. Destination hostnames are sent to the proxy, never resolved locally.
// There is deliberately no direct mode or environment-proxy fallback.
func NewTorClient(address string) (*http.Client, error) {
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "socks5h" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
		return nil, fmt.Errorf("TOR_PROXY_URL must be socks5h://host:port (without credentials or path)")
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 {
		return nil, fmt.Errorf("TOR_PROXY_URL requires a valid explicit port")
	}
	dialer, err := proxy.SOCKS5("tcp", net.JoinHostPort(u.Hostname(), u.Port()), nil, &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("configure Tor SOCKS5: %w", err)
	}
	contextDialer, ok := dialer.(proxy.ContextDialer)
	if !ok {
		return nil, fmt.Errorf("SOCKS5 dialer does not support cancellation")
	}
	transport := &http.Transport{
		Proxy:               nil,
		DialContext:         contextDialer.DialContext,
		TLSHandshakeTimeout: 20 * time.Second,
		// The overall client timeout bounds header and body waits.
		IdleConnTimeout: 30 * time.Second,
		MaxIdleConns:    4,
	}
	return &http.Client{Transport: transport, Timeout: 60 * time.Second}, nil
}
