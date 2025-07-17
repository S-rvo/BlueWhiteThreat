package sites

import (
	"context"
	//	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/chromedp"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/S-rvo/BlueWhiteThreat/apps/scraper/internal/db"
)

type Article struct {
	Username string `json:"username"`
	Date     string `json:"date"`
	Title    string `json:"title"`
	Content  string `json:"content"`
}

func RunScraper() {
	// Configuration de chromedp
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("blink-settings", "imagesEnabled=false"),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
	)

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	// Navigation et extraction HTML
	var html string
	err := chromedp.Run(ctx,
		chromedp.Navigate("http://localhost:8081"),
		chromedp.WaitVisible(`div.card`, chromedp.ByQuery),
		chromedp.OuterHTML("body", &html),
	)
	if err != nil {
		log.Fatal(err)
	}

	// Parsing DOM avec GoQuery
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		log.Fatal(err)
	}

	var posts []Article
	doc.Find("div.card").Each(func(i int, s *goquery.Selection) {
		username := strings.TrimSpace(s.Find(".card-header-info h3").Text())
		date := strings.TrimSpace(s.Find(".card-header-info").Contents().Last().Text())
		title := strings.TrimSpace(s.Find(".card-body h3").Text())
		content := strings.TrimSpace(s.Find(".card-body p").Text())

		posts = append(posts, Article{
			Username: username,
			Date:     date,
			Title:    title,
			Content:  content,
		})
	})

	// Connexion à MongoDB
	client, err := db.ConnectMongo()
	if err != nil {
		log.Fatal("Erreur de connexion MongoDB:", err)
	}
	defer client.Disconnect(context.Background())
	collection := client.Database("BlueWhiteThreat").Collection("article")

	// Ajoute un index unique dans MongoDB
	indexModel := mongo.IndexModel{
		Keys: bson.D{
			{Key: "username", Value: 1},
			{Key: "date", Value: 1},
			{Key: "title", Value: 1},
		},
		Options: options.Index().SetUnique(true),
	}
	_, err = collection.Indexes().CreateOne(context.Background(), indexModel)
	if err != nil {
		log.Println("Erreur création index unique:", err)
	}

	// Insertion dans MongoDB
	inserted, err := db.SaveAllEntries("BlueWhiteThreat", "article", posts, []string{"Username", "Date", "Title", "Content"})
	if err != nil {
		log.Fatal("Erreur lors de l'upsert des posts:", err)
	}
	fmt.Printf("%d nouveaux documents insérés dans MongoDB\n", inserted)

	// Screenshot
	var buf []byte
	err = chromedp.Run(ctx,
		chromedp.FullScreenshot(&buf, 90),
	)
	if err != nil {
		log.Fatal(err)
	}

	err = os.WriteFile("screenshot.png", buf, 0600)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Screenshot enregistré sous screenshot.png")
}

