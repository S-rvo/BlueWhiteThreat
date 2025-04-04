package main

import (
	"fmt"
	"log"

	crawler "github.com/S-rvo/BlueWhiteThreat/internal/crawler"
)

func main() {
	startURL := "http://6nhmgdpnyoljh5uzr5kwlatx2u3diou4ldeommfxjz3wkhalzgjqxzqd.onion"
	depth := 1

	visited, links, statusCode, err := crawler.Crawler(startURL, depth)
	if err != nil {
		log.Fatalf("Erreur dans le crawler : %v", err)
	}

	fmt.Println("✅ Code HTTP :", statusCode)

	fmt.Println("\nURLs visitées :")
	for _, url := range visited {
		fmt.Println(" -", url)
	}

	fmt.Println("\nLiens trouvés sur la page :")
	for _, link := range links {
		fmt.Println(" →", link)
	}
}
