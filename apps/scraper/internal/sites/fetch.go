package sites

import (
	"context"
	"fmt"
	"os"

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
	return append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("blink-settings", "imagesEnabled=false"),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.ProxyServer(ProxyServerFlagValue(torProxy)),
	)
}

// newChromedpContext crée un contexte chromedp configuré avec le proxy Tor donné.
// L'appelant doit appeler la fonction cancel retournée pour libérer les ressources.
func newChromedpContext(parent context.Context, torProxy string) (context.Context, context.CancelFunc) {
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(parent, BuildChromedpOptions(torProxy)...)
	ctx, cancelCtx := chromedp.NewContext(allocCtx)
	return ctx, func() {
		cancelCtx()
		cancelAlloc()
	}
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
	if cfg != nil {
		username := envOrEmpty(cfg.UsernameEnv)
		password := envOrEmpty(cfg.PasswordEnv)
		if err := chromedp.Run(ctx,
			chromedp.Navigate(cfg.LoginURL),
			chromedp.WaitVisible(cfg.UsernameSelector, chromedp.ByQuery),
			chromedp.SendKeys(cfg.UsernameSelector, username, chromedp.ByQuery),
			chromedp.SendKeys(cfg.PasswordSelector, password, chromedp.ByQuery),
			chromedp.Click(cfg.SubmitSelector, chromedp.ByQuery),
		); err != nil {
			return "", fmt.Errorf("login failed for %s: %w", targetURL, err)
		}
	}

	var content string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(targetURL),
		chromedp.Text("body", &content, chromedp.ByQuery),
	); err != nil {
		return "", fmt.Errorf("fetch failed for %s: %w", targetURL, err)
	}
	return content, nil
}
