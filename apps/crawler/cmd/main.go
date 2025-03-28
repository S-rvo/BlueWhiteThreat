package main

import (
	"fmt"
	"log"

	crawler "github.com/S-rvo/BlueWhiteThreat/internal/crawler"
)

func main() {
	visited, links, statusCode, err := crawler.Crawler()
	if err != nil {
		log.Fatalf("Erreur dans le crawler : %v", err)
	}

	fmt.Println("Code HTTP :", statusCode)

	fmt.Println("URLs visitées :")
	for _, url := range visited {
		fmt.Println(" -", url)
	}

	fmt.Println("Liens trouvés sur la page :")
	for _, link := range links {
		fmt.Println(" →", link)
	}
}
