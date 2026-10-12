package scraper_tests

import (
	"testing"

	"github.com/S-rvo/BlueWhiteThreat/apps/scraper/internal/sites"
)

func TestProxyServerFlagValue(t *testing.T) {
	got := sites.ProxyServerFlagValue("tor1:9050")
	want := "socks5://tor1:9050"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
