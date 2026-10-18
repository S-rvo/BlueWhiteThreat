package sites

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
)

// noisyTags are structural chrome that is never the actual article content,
// repeats on every page of a site, and would otherwise dominate the text
// used for classification and deduplication.
var noisyTags = []string{
	"script", "style", "noscript", "iframe", "svg", "form", "button",
	"nav", "footer", "aside",
}

// noiseClassTokens are class/id tokens that mark an element as boilerplate
// (navigation, ads, cookie banners, etc.) rather than content. Matching is
// done per-token (see hasNoiseToken), not by substring, so a class like
// "shadow-gradient-box" isn't wrongly treated as an ad just because it
// contains the letters "ad".
var noiseClassTokens = map[string]bool{
	"nav": true, "navbar": true, "navigation": true,
	"menu": true, "sidebar": true, "footer": true, "header": true,
	"cookie": true, "cookies": true,
	"ad": true, "ads": true, "advert": true, "advertisement": true,
	"banner": true, "popup": true, "modal": true,
	"breadcrumb": true, "breadcrumbs": true,
}

var classTokenSplit = regexp.MustCompile(`[^a-zA-Z0-9]+`)

func hasNoiseToken(attrValue string, inContent, containsArticleBody bool) bool {
	for _, tok := range classTokenSplit.Split(strings.ToLower(attrValue), -1) {
		if noiseClassTokens[tok] && !(inContent && tok == "header") && !(containsArticleBody && tok == "sidebar") {
			return true
		}
	}
	return false
}

func containsArticleBody(s *goquery.Selection) bool {
	const selectors = ".entry-content, .wp-block-post-content, [itemprop='articleBody'], .c-layout-main"
	return s.Is(selectors) || s.Find(selectors).Length() > 0
}

// Block boundaries must survive compact HTML; inline markup must not split
// words (e.g. ransom<strong>ware</strong>).
func appendContentText(out *strings.Builder, node *html.Node) {
	if node.Type == html.TextNode {
		out.WriteString(node.Data)
		return
	}
	block := false
	if node.Type == html.ElementNode {
		switch node.Data {
		case "address", "article", "blockquote", "br", "dd", "div", "dl", "dt",
			"fieldset", "figcaption", "figure", "h1", "h2", "h3", "h4", "h5", "h6",
			"header", "hr", "li", "main", "ol", "p", "pre", "section", "table",
			"tbody", "td", "th", "thead", "tr", "ul":
			block = true
		}
	}
	if block {
		out.WriteByte('\n')
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		appendContentText(out, child)
	}
	if block {
		out.WriteByte('\n')
	}
}

// ExtractMainContent parses raw page HTML and returns its text with
// navigation, ads, scripts, and other non-article chrome stripped out. A
// bare chromedp.Text("body", ...) grab (the previous behavior) includes
// every menu/footer/ad on the page, which swamps the real content used for
// classification and deduplication.
func ExtractMainContent(html string) (string, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return "", fmt.Errorf("parse html: %w", err)
	}

	doc.Find(strings.Join(noisyTags, ", ")).ReplaceWithHtml("\n")
	doc.Find("[hidden], [aria-hidden='true']").ReplaceWithHtml("\n")
	// ARIA landmarks are also used on generic divs. Preserve wrappers that
	// contain an explicit article body: some layouts misuse complementary.
	doc.Find("[role]").FilterFunction(func(_ int, s *goquery.Selection) bool {
		role, _ := s.Attr("role")
		inContent := s.ParentsFiltered("article, main, [role='main']").Length() > 0
		for _, token := range strings.Fields(strings.ToLower(role)) {
			switch token {
			case "navigation", "contentinfo":
				return !containsArticleBody(s)
			case "complementary":
				return !containsArticleBody(s)
			case "banner":
				return !inContent && !containsArticleBody(s)
			}
		}
		return false
	}).ReplaceWithHtml("\n")
	doc.Find("header").FilterFunction(func(_ int, s *goquery.Selection) bool {
		return s.ParentsFiltered("article, main, [role='main']").Length() == 0
	}).ReplaceWithHtml("\n")
	doc.Find("*").FilterFunction(func(_ int, s *goquery.Selection) bool {
		if s.Is("html, body") {
			return false
		}
		class, _ := s.Attr("class")
		id, _ := s.Attr("id")
		inContent := s.ParentsFiltered("article, main, [role='main']").Length() > 0
		containsBody := containsArticleBody(s)
		return hasNoiseToken(class, inContent, containsBody) || hasNoiseToken(id, inContent, containsBody)
	}).ReplaceWithHtml("\n")

	var text strings.Builder
	for _, node := range doc.Find("body").Nodes {
		appendContentText(&text, node)
	}

	var lines []string
	for _, line := range strings.Split(text.String(), "\n") {
		line = strings.Join(strings.Fields(line), " ")
		if line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n"), nil
}
