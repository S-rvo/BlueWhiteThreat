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
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Post struct {
	Username string `json:"username"`
	Date     string `json:"date"`
	Title    string `json:"title"`
	Content  string `json:"content"`
}

func getMongoURI() string {
	user := os.Getenv("MONGO_INITDB_ROOT_USERNAME")
	pass := os.Getenv("MONGO_INITDB_ROOT_PASSWORD")
	host := os.Getenv("MONGO_HOST")
	port := os.Getenv("MONGO_PORT")

	if user == "" {
		user = "root"
	}
	if pass == "" {
		pass = "root"
	}
	if host == "" {
		host = "localhost"
	}
	if port == "" {
		port = "27017"
	}

	return fmt.Sprintf("mongodb://%s:%s@%s:%s/?authSource=admin", user, pass, host, port)
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

	log.Println("chromedp context:", ctx)
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
	log.Println("html:", html)

	// Parsing DOM avec GoQuery
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

	// Connexion à MongoDB
	mongoURI := getMongoURI()
	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatal("Erreur de connexion MongoDB:", err)
	}
	defer client.Disconnect(context.Background())

	collection := client.Database("scrapeDB").Collection("posts")

	// Conversion []Post → []interface{}
	var postsAsInterface []interface{}
	for _, post := range posts {
		postsAsInterface = append(postsAsInterface, post)
	}

	// Insertion dans MongoDB
	result, err := collection.InsertMany(context.Background(), postsAsInterface)
	if err != nil {
		log.Fatal("Erreur insertion Mongo:", err)
	}

	fmt.Printf("%d documents insérés dans MongoDB\n", len(result.InsertedIDs))

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

