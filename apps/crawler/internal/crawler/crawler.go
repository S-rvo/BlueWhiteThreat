package crawler

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/gocolly/colly"
	"github.com/gocolly/colly/proxy"
)

/*
func TorClient() (string, error) {
	proxyAddr := os.Getenv("TOR_PROXY")
	if proxyAddr == "" {
		proxyAddr = "socks5://172.18.0.2:9050"
	}

	dialer, err := proxy.SOCKS5("tcp", "172.18.0.2:9050", nil, proxy.Direct)
	if err != nil {
		panic(err)
	}

	client := &http.Client{
		Transport: &http.Transport{
			Dial: dialer.Dial,
		},
	}

	resp, err := client.Get("http://6nhmgdpnyoljh5uzr5kwlatx2u3diou4ldeommfxjz3wkhalzgjqxzqd.onion/")
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Println(string(body))

	return string(body), nil
}
*/

func Crawler() ([]string, []string, int, error) {
	c := colly.NewCollector()

	proxyList := []string{
		"socks5://172.18.0.2:9050",
	}

	rp, err := proxy.RoundRobinProxySwitcher(proxyList...)
	if err != nil {
		return nil, nil, 0, err
	}

	c.SetProxyFunc(func(req *http.Request) (*url.URL, error) {
		proxyURL, err := rp(req)
		if err == nil {
			fmt.Println("🛡️  Proxy utilisé :", proxyURL.String())
		}
		return proxyURL, err
	})

	var visitedUrls []string
	var foundLinks []string
	var statusCode int

	c.OnRequest(func(r *colly.Request) {
		fmt.Println("🌐 Envoi de la requête vers :", r.URL.String())
		visitedUrls = append(visitedUrls, r.URL.String())
	})

	c.OnResponse(func(r *colly.Response) {
		statusCode = r.StatusCode
		fmt.Println("📥 Statut HTTP :", r.StatusCode)
	})

	// 🔍 Extraction des liens <a href="...">
	c.OnHTML("a[href]", func(e *colly.HTMLElement) {
		link := e.Attr("href")
		absLink := e.Request.AbsoluteURL(link)
		fmt.Printf("🔗 Lien trouvé: %q -> %s\n", e.Text, absLink)

		// Ajouter au tableau si non vide
		if absLink != "" {
			foundLinks = append(foundLinks, absLink)
		}

		// (Optionnel) Visite automatique des liens internes
		// c.Visit(absLink)
	})

	err = c.Visit("http://6nhmgdpnyoljh5uzr5kwlatx2u3diou4ldeommfxjz3wkhalzgjqxzqd.onion") // page HTML simple avec liens
	if err != nil {
		return visitedUrls, foundLinks, statusCode, err
	}

	return visitedUrls, foundLinks, statusCode, nil
}
