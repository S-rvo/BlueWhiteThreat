package main

import (
	"fmt"

	crawler "github.com/S-rvo/BlueWhiteThreat/internal/crawler"
)

func main() {
	content, err := crawler.TorClient()
	if err != nil {
		fmt.Println("Erreur:", err)
		return
	}
	fmt.Println("Réponse via Tor:")
	fmt.Println(content)

	content2, err := crawler.scrapper()
	if err != nil {
		fmt.Println("Erreur:", err)
		return

	}
	fmt.Println(content2)
}
