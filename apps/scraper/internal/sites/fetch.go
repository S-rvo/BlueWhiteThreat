package sites

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/chromedp/chromedp"
)

// ProxyServerFlagValue construit la valeur du flag --proxy-server pour chromedp
// à partir d'un proxy Tor "host:port".
func ProxyServerFlagValue(torProxy string) string {
	return "socks5://" + torProxy
}

// BuildChromedpOptions construit les options d'allocation chromedp avec le
// proxy Tor donné, en mode headless (même pattern que DarkThreat.go).
func BuildChromedpOptions(torProxy string) []chromedp.ExecAllocatorOption {
	// Keep Chromium defaults, including its disabled features, while preventing
	// silent HTTP -> HTTPS rewrites that violate the renderer's pinned origin.
	// Explicit HTTPS URLs and server redirects retain their existing policy.
	opts := append([]chromedp.ExecAllocatorOption(nil), chromedp.DefaultExecAllocatorOptions[:]...)
	return append(opts,
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("disable-features", "site-per-process,Translate,BlinkGenPropertyTrees,HttpsUpgrades"),
		chromedp.Flag("blink-settings", "imagesEnabled=false"),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.ProxyServer(ProxyServerFlagValue(torProxy)),
	)
}

// newChromedpContext crée un contexte chromedp configuré avec le proxy Tor donné.
// L'appelant doit appeler la fonction cancel retournée pour libérer les ressources.
func newChromedpContext(parent context.Context, torProxy string) (context.Context, context.CancelFunc) {
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.WithValue(parent, crawlProxyKey{}, torProxy), BuildChromedpOptions(torProxy)...)
	ctx, cancelCtx := chromedp.NewContext(allocCtx)
	return ctx, func() {
		cancelCtx()
		cancelAlloc()
	}
}

// pooledBrowser garde un process Chrome vivant pour tout un cycle de scraping
// (un par worker) au lieu d'en relancer un neuf à chaque URL. Un process
// Chrome + négociation SOCKS5 complète par URL est l'une des causes
// principales du taux d'échec Tor plus élevé côté scraper que côté crawler
// (qui fait un simple fetch HTTP via Colly). rebuild remplace le process --
// nécessaire après un échec (le process peut être dans un état incertain) ou
// pour retenter via un proxy différent.
type pooledBrowser struct {
	proxy  string
	ctx    context.Context
	cancel context.CancelFunc
}

func newPooledBrowser(proxy string) *pooledBrowser {
	ctx, cancel := newChromedpContext(context.Background(), proxy)
	return &pooledBrowser{proxy: proxy, ctx: ctx, cancel: cancel}
}

func (b *pooledBrowser) rebuild(proxy string) {
	b.cancel()
	b.proxy = proxy
	b.ctx, b.cancel = newChromedpContext(context.Background(), proxy)
}

func (b *pooledBrowser) close() {
	b.cancel()
}

func envOrEmpty(name string) string {
	if name == "" {
		return ""
	}
	return os.Getenv(name)
}

// FetchPage navigue vers targetURL via Tor (proxy déjà posé dans ctx par
// BuildChromedpOptions au niveau de l'appelant), effectue un login optionnel
// si cfg est non-nil, puis renvoie le texte brut du body.
func FetchPage(ctx context.Context, targetURL string, cfg *SiteConfig) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	if cfg == nil {
		release, err := preparePublicRender(ctx, targetURL)
		if err != nil {
			return "", err
		}
		defer release()
	}

	if cfg != nil {
		release, err := prepareAccess(ctx, targetURL, cfg)
		if err != nil {
			return "", err
		}
		defer release()
	}

	var html string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(targetURL),
		chromedp.OuterHTML("body", &html, chromedp.ByQuery),
	); err != nil {
		return "", fmt.Errorf("fetch failed for %s: %w", targetURL, err)
	}

	if cfg != nil {
		if err := checkChallenge(ctx, cfg.CAPTCHA); err != nil {
			return "", err
		}
		// A standalone challenge may have navigated to the actual content.
		if err := chromedp.Run(ctx, chromedp.OuterHTML("body", &html, chromedp.ByQuery)); err != nil {
			return "", err
		}
		if err := saveSession(ctx, cfg.StateFile); err != nil {
			return "", err
		}
	}
	content, err := ExtractMainContent(html)
	if err != nil {
		return "", fmt.Errorf("extract content for %s: %w", targetURL, err)
	}
	// Only public pages contribute links; authenticated sessions may expose private URLs.
	if cfg == nil {
		var links []string
		if e := chromedp.Run(ctx, chromedp.Evaluate(`Array.from(new Set(Array.from(document.querySelectorAll('a[href]'),a=>a.href))).slice(0,5000)`, &links)); e != nil {
			return "", fmt.Errorf("rendered discovery: %w", e)
		} else if len(links) > 0 {
			e = publishRenderedLinks(ctx, targetURL, links)
			if e != nil {
				return "", fmt.Errorf("rendered discovery: %w", e)
			}
		}
	}
	return content, nil
}
