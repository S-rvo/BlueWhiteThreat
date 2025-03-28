package crawler

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/gocolly/colly/v2"
	"golang.org/x/net/proxy"
)

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

func scrapper() {
	url := "http://6nhmgdpnyoljh5uzr5kwlatx2u3diou4ldeommfxjz3wkhalzgjqxzqd.onion/" // Remplace par l'URL cible

	// Crée un nouveau collecteur
	c := colly.NewCollector(
		colly.AllowedDomains("http://6nhmgdpnyoljh5uzr5kwlatx2u3diou4ldeommfxjz3wkhalzgjqxzqd.onion/"), // à adapter si nécessaire
	)

	var links []string

	// Callback sur chaque lien trouvé
	c.OnHTML("a[href]", func(e *colly.HTMLElement) {
		link := e.Attr("href")
		links = append(links, link)
		fmt.Println("Lien trouvé :", link)
	})

	// Callback en cas d'erreur
	c.OnError(func(_ *colly.Response, err error) {
		log.Println("Erreur lors du scraping :", err)
	})

	// Lance la visite
	err := c.Visit(url)
	if err != nil {
		log.Fatal("Échec de la visite :", err)
	}
}
