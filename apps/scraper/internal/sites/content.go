package sites

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// noisyTags are structural chrome that is never the actual article content,
// repeats on every page of a site, and would otherwise dominate the text
// used for classification and deduplication.
var noisyTags = []string{
	"script", "style", "noscript", "iframe", "svg", "form", "button",
	"nav", "header", "footer", "aside",
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

func hasNoiseToken(attrValue string) bool {
	for _, tok := range classTokenSplit.Split(strings.ToLower(attrValue), -1) {
		if noiseClassTokens[tok] {
			return true
		}
	}
	return false
}

var whitespaceRun = regexp.MustCompile(`[ \t]+`)

// ExtractMainContent parses raw page HTML and returns its visible text with
// navigation, ads, scripts, and other non-article chrome stripped out. A
// bare chromedp.Text("body", ...) grab (the previous behavior) includes
// every menu/footer/ad on the page, which swamps the real content used for
// classification and deduplication.
func ExtractMainContent(html string) (string, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return "", fmt.Errorf("parse html: %w", err)
	}

	doc.Find(strings.Join(noisyTags, ", ")).Remove()
	doc.Find("*").FilterFunction(func(_ int, s *goquery.Selection) bool {
		class, _ := s.Attr("class")
		id, _ := s.Attr("id")
		return hasNoiseToken(class) || hasNoiseToken(id)
	}).Remove()

	text := doc.Find("body").Text()
	text = whitespaceRun.ReplaceAllString(text, " ")

	var lines []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n"), nil
}
