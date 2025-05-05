package crawler

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sync"

	"github.com/gocolly/colly"
	"github.com/gocolly/colly/proxy"
)

func Crawler(startURL string, depthMax int) ([]string, []string, int, error) {
	var visitedUrls []string
	var foundLinks []string
	var statusCode int

	//verrouiller l'accès concurrent à visitedMap, visitedUrls et foundLinks.
	visitedMap := make(map[string]bool)
	var mu sync.Mutex
	//synchroniser les goroutines attend la fin de toutes les visites
	var wg sync.WaitGroup

	proxyList := []string{
		"socks5://tor1:9050",
		"socks5://tor2:9050",
		"socks5://tor3:9050",
	}
	// Rotation des proxy
	rp, err := proxy.RoundRobinProxySwitcher(proxyList...)
	if err != nil {
		return nil, nil, 0, err
	}
	//Appelle de la fonction récursivement
	var crawl func(targetURL string, depth int)
	crawl = func(targetURL string, depth int) {
		defer wg.Done()

		if depth > depthMax {
			return
		}

		mu.Lock()
		if visitedMap[targetURL] {
			mu.Unlock()
			return
		}
		visitedMap[targetURL] = true
		mu.Unlock()

		c := colly.NewCollector()

		c.SetProxyFunc(func(req *http.Request) (*url.URL, error) {
			proxyURL, err := rp(req)
			if err == nil {
				fmt.Println("Proxy utilisé :", proxyURL.String())
			}
			return proxyURL, err
		})

		c.OnRequest(func(r *colly.Request) {
			fmt.Println("Request to:", r.URL.String())
			mu.Lock()
			visitedUrls = append(visitedUrls, r.URL.String())
			mu.Unlock()
		})

		c.OnResponse(func(r *colly.Response) {
			statusCode = r.StatusCode
			fmt.Println("Statut HTTP :", r.StatusCode)
		})
		onionRegex := regexp.MustCompile(`\.onion$`) // pour toute url qui contient un vrai .onion
		c.OnHTML("a[href]", func(e *colly.HTMLElement) {
			link := e.Attr("href")
			absLink := e.Request.AbsoluteURL(link)

			// Ajouter seulement si c'est un lien .onion
			if onionRegex.MatchString(absLink) {
				mu.Lock()
				foundLinks = append(foundLinks, absLink)
				mu.Unlock()

				wg.Add(1)
				go crawl(absLink, depth+1)
			}
		})

		err := c.Visit(targetURL)
		if err != nil {
			fmt.Println("Erreur de visite :", err)
			return
		}
	}

	wg.Add(1)
	go crawl(startURL, 0)

	wg.Wait()

	return visitedUrls, foundLinks, statusCode, nil
}
