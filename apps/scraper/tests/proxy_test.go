package scraper_tests

import (
	"testing"

	"github.com/S-rvo/BlueWhiteThreat/apps/scraper/internal/sites"
)

func TestNextTorProxy_RotatesThroughAllProxies(t *testing.T) {
	rotator := sites.NewTorProxyRotator([]string{"tor1:9050", "tor2:9050", "tor3:9050"})

	seen := map[string]bool{}
	for i := 0; i < 6; i++ {
		seen[rotator.Next()] = true
	}

	if len(seen) != 3 {
		t.Errorf("expected to see all 3 proxies over 6 calls, got %#v", seen)
	}
}

func TestNextTorProxy_SingleProxyAlwaysReturnsIt(t *testing.T) {
	rotator := sites.NewTorProxyRotator([]string{"tor1:9050"})

	if got := rotator.Next(); got != "tor1:9050" {
		t.Errorf("got %q, want %q", got, "tor1:9050")
	}
}
