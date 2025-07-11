package sites

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
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Post struct {
	Username string `json:"username"`
	Date     string `json:"date"`
	Title    string `json:"title"`
	Content  string `json:"content"`
}

func DarkThreat() {
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

		// Connexion à MongoDB
	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		log.Fatal("Erreur de connexion MongoDB:", err)
	}
	defer client.Disconnect(context.Background())

	// Choisir base + collection
	collection := client.Database("scrapeDB").Collection("posts")

	// Lire le fichier JSON généré
	fileContent, err := os.ReadFile("internal/db/output.json")
	if err != nil {
		log.Fatal("Erreur lecture JSON:", err)
	}

	var postsFromFile []interface{}
	err = json.Unmarshal(fileContent, &postsFromFile)
	if err != nil {
		log.Fatal("Erreur parsing JSON:", err)
	}

	// Insérer les documents dans MongoDB
	result, err := collection.InsertMany(context.Background(), postsFromFile)
	if err != nil {
		log.Fatal("Erreur insertion Mongo:", err)
	}

	fmt.Printf("✅ %d documents insérés dans MongoDB\n", len(result.InsertedIDs))


	var buf []byte
	err = chromedp.Run(ctx,
		chromedp.FullScreenshot(&buf, 90),
	)
	if err != nil {
		log.Fatal(err)
	}

	err = os.WriteFile("screenshot.png", buf, 0644)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Screenshot enregistré sous screenshot.png")
}

