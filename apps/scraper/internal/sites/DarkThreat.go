package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	//"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/chromedp" //pour l'exécution de JS
)

type Post struct {
	Username string `json:"username"`
	Date     string `json:"date"`
	Title    string `json:"title"`
	Content  string `json:"content"`
}

func main() {
	//ctx, cancel := chromedp.NewContext(context.Background()) trop long
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),                        // pas d'interface
		chromedp.Flag("disable-gpu", true),                     // pas de rendu matériel
		chromedp.Flag("blink-settings", "imagesEnabled=false"), // pas d'images
		chromedp.Flag("no-sandbox", true),                      // plus stable en conteneur
		chromedp.Flag("disable-dev-shm-usage", true),
	)

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	// Lancer la page
	var html string
	err := chromedp.Run(ctx,
		chromedp.Navigate("http://localhost:5173"),
		chromedp.WaitVisible(`div.card`, chromedp.ByQuery), // attendre le rendu JS
		chromedp.OuterHTML("body", &html),
	)
	if err != nil {
		log.Fatal(err)
	}

	// Parse avec GoQuery (DOM)
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		log.Fatal(err)
	}

	var posts []Post
	doc.Find("div.card").Each(func(i int, s *goquery.Selection) {
		username := strings.TrimSpace(s.Find(".card-header-info h3").Text())
		date := strings.TrimSpace(s.Find(".card-header-info").Contents().Last().Text())
		title := strings.TrimSpace(s.Find(".card-body h3").Text())
		content := strings.TrimSpace(s.Find(".card-body p").Text())

		posts = append(posts, Post{
			Username: username,
			Date:     date,
			Title:    title,
			Content:  content,
		})
	})

	// Sauvegarde JSON
	file, _ := os.Create("internal/db/output.json")
	defer file.Close()
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	encoder.Encode(posts)

	fmt.Println("JS exécuté et output.json généré !")
}
