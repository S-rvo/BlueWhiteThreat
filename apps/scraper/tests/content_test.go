package scraper_tests

import (
	"strings"
	"testing"

	"github.com/S-rvo/BlueWhiteThreat/apps/scraper/internal/sites"
)

func TestExtractMainContentStripsBoilerplateTags(t *testing.T) {
	html := `<html><body>
		<nav>Home | Forums | Market</nav>
		<header>Site Title</header>
		<script>trackVisit();</script>
		<style>.x{color:red}</style>
		<article><p>Victim data from a healthcare provider was leaked after a ransomware attack.</p></article>
		<footer>Copyright 2026</footer>
	</body></html>`

	got, err := sites.ExtractMainContent(html)
	if err != nil {
		t.Fatalf("ExtractMainContent returned error: %v", err)
	}

	if !strings.Contains(got, "ransomware attack") {
		t.Errorf("expected article content to be kept, got %q", got)
	}
	for _, noise := range []string{"Forums", "Market", "Site Title", "trackVisit", "Copyright 2026"} {
		if strings.Contains(got, noise) {
			t.Errorf("expected boilerplate %q to be stripped, got %q", noise, got)
		}
	}
}

func TestExtractMainContentStripsNoiseByClassToken(t *testing.T) {
	html := `<html><body>
		<div class="ad-banner">Buy credentials now, 50% off</div>
		<div class="site-sidebar">Related posts</div>
		<div class="shadow-gradient-box">A new exploit targets unpatched routers.</div>
	</body></html>`

	got, err := sites.ExtractMainContent(html)
	if err != nil {
		t.Fatalf("ExtractMainContent returned error: %v", err)
	}

	if !strings.Contains(got, "exploit targets unpatched routers") {
		t.Errorf("expected content in a class merely containing 'ad'/'gradient' substrings to be kept, got %q", got)
	}
	for _, noise := range []string{"Buy credentials", "Related posts"} {
		if strings.Contains(got, noise) {
			t.Errorf("expected noise-token content %q to be stripped, got %q", noise, got)
		}
	}
}

func TestExtractMainContentCollapsesWhitespace(t *testing.T) {
	html := `<html><body>
		<p>Line one.</p>


		<p>Line two.</p>
	</body></html>`

	got, err := sites.ExtractMainContent(html)
	if err != nil {
		t.Fatalf("ExtractMainContent returned error: %v", err)
	}
	if strings.Contains(got, "\n\n\n") {
		t.Errorf("expected collapsed whitespace, got %q", got)
	}
	if !strings.Contains(got, "Line one.") || !strings.Contains(got, "Line two.") {
		t.Errorf("expected both lines retained, got %q", got)
	}
}
