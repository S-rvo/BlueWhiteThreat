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

func TestExtractMainContentPreservesTextStructure(t *testing.T) {
	cases := []struct{ name, html, want string }{
		{"compact paragraphs", `<body><p>data</p><p>breach</p></body>`, "data\nbreach"},
		{"inline word markup", `<body><p>ransom<strong>ware</strong> attack</p></body>`, "ransomware attack"},
		{"line breaks and cells", `<body>one<br>two<table><tr><td>three</td><td>four</td></tr></table></body>`, "one\ntwo\nthree\nfour"},
		{"article header survives", `<body><header>Site menu</header><article><header class="article-header"><h1>Victim breach</h1></header><p>Details</p></article></body>`, "Victim breach\nDetails"},
		{"main heading survives", `<body><main><div class="post-header"><h1>New exploit</h1></div><p>Details</p></main></body>`, "New exploit\nDetails"},
		{"sidebar layout preserves nested article body", `<body><main><div class="wp-block-columns is-style-sidebar"><aside>Related links</aside><div class="entry-content"><p>Observed campaign details.</p><p>Indicators and mitigation.</p></div></div></main></body>`, "Observed campaign details.\nIndicators and mitigation."},
		{"NCSC sidebar layout preserves main text", `<body><main><div class="c-layout-sidebar-right"><div class="c-layout-main"><section><p>Threat assessment evidence.</p></section></div><div class="c-layout-sidebar"><p>Related navigation.</p></div><aside>More links</aside></div></main></body>`, "Threat assessment evidence."},
		{"article ads still removed", `<body><article><header><h1>Report</h1><div class="ad-header">Buy now</div></header></article></body>`, "Report"},
		{"removal does not join words", `<body>data<nav>Menu</nav>breach</body>`, "data\nbreach"},
		{"explicitly hidden content", `<body><p>Report</p><div hidden>spam</div><div aria-hidden="true">spam</div></body>`, "Report"},
		{"body banner class preserves main content", `<body class="full-width has-alert-banner"><main><article><p>Security alert details.</p></article></main></body>`, "Security alert details."},
		{"unicode whitespace", `<body><p>data&nbsp; breach</p></body>`, "data breach"},
		{"ARIA div navigation and complementary", `<body><div role="navigation">Other topics</div><div role="complementary">Desktop menu</div><main><p>Observed incident.</p></main><div role="contentinfo">Corporate links</div></body>`, "Observed incident."},
		{"ARIA banner keeps article heading", `<body><div role="banner">Site heading</div><article><div role="banner"><h1>Campaign report</h1></div><p>Evidence.</p></article></body>`, "Campaign report\nEvidence."},
		{"misused complementary preserves explicit article body", `<body><div role="complementary"><div itemprop="articleBody"><p>Original evidence.</p></div></div></body>`, "Original evidence."},
		{"ARIA main and unknown roles preserved", `<body><div role="main"><h1>Report</h1><div role="region">Findings</div></div></body>`, "Report\nFindings"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := sites.ExtractMainContent(tc.html)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
