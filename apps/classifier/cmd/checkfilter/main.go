package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/db"
	"github.com/joho/godotenv"
)

var suspectTerms = []string{
	"loliporn", "pedo porn", "pedomom", "kidsparty", "diamond porn links", "child porno",
}

func main() {
	godotenv.Load()
	ctx := context.Background()
	client, err := db.Connect(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Disconnect(ctx)
	articles, err := db.FetchAllContent(ctx, client)
	if err != nil {
		log.Fatal(err)
	}
	count := 0
	for _, a := range articles {
		low := strings.ToLower(a.Content)
		for _, term := range suspectTerms {
			if strings.Contains(low, term) {
				count++
				fmt.Printf("MATCH term=%q id=%v\n", term, a.ID)
				break
			}
		}
	}
	fmt.Println("total matching docs:", count)
}
