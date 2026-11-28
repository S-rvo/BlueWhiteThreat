package sites

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/S-rvo/BlueWhiteThreat/apps/scraper/internal/db"
	"github.com/S-rvo/BlueWhiteThreat/pkg/csamfilter"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ErrBlockedContent marque un rejet CSAM : permanent, une retentative ne
// changera jamais le verdict, contrairement à un échec réseau/Tor transitoire.
var ErrBlockedContent = errors.New("contenu bloqué")

// ScrapedPage est le contenu brut d'une page .onion récupérée par le scraper,
// avec sa source conservée pour vérification par un analyste CTI.
type ScrapedPage struct {
	URL        string    `bson:"url"`
	Source     string    `bson:"source"`
	SourceType string    `bson:"source_type"`
	ScrapedAt  time.Time `bson:"scraped_at"`
	Content    string    `bson:"content"`
}

const (
	SourceTypeDarkwebPage = "darkweb_page"
	SourceTypeUnknown     = "unknown"
)

// SaveScrapedPage upsert une page scrapée dans MongoDB.article, par URL unique.
// Les pages indexant du CSAM sont rejetées avant toute écriture.
func SaveScrapedPage(page ScrapedPage) (int, error) {
	return saveScrapedPage(context.Background(), nil, page)
}

// contentDerivedFields are computed downstream from the article text
// (classifier, parser) and become stale as soon as that text changes.
var contentDerivedFields = []string{
	"relevance",
	"entities",
	"graph_projection_queued_at",
	"extraction_failures",
	"extraction_retry_at",
	"extraction_last_error",
}

// A production cycle shares one Mongo client across all workers.
func saveScrapedPage(parent context.Context, client *mongo.Client, page ScrapedPage) (int, error) {
	if page.SourceType == "" {
		page.SourceType = SourceTypeUnknown
	}
	if csamfilter.IsBlockedURL(page.URL) || csamfilter.ContainsBlockedContent(page.Content) {
		log.Printf("Contenu bloqué (CSAM) détecté sur %s — page rejetée, non persistée", page.URL)
		return 0, fmt.Errorf("%w: %s", ErrBlockedContent, page.URL)
	}
	if err := parent.Err(); err != nil {
		return 0, err
	}
	if client == nil {
		var err error
		client, err = db.ConnectMongo()
		if err != nil {
			return 0, err
		}
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = client.Disconnect(ctx)
		}()
	}
	ctx, cancel := context.WithTimeout(parent, 15*time.Second)
	defer cancel()
	// Evaluate against the old content in the same atomic update. A new text
	// must not inherit anything derived from the previous version (verdict,
	// entities, extraction retries); an unchanged page keeps it all.
	// graph_projected_at is deliberately kept: it tells the parser the graph
	// still holds this article, so it can retract or replace it.
	set := bson.M{
		"url":         bson.M{"$literal": page.URL},
		"source":      bson.M{"$literal": page.Source},
		"source_type": bson.M{"$literal": page.SourceType},
		"scraped_at":  page.ScrapedAt,
		"content":     bson.M{"$literal": page.Content},
	}
	sameContent := bson.M{"$eq": bson.A{"$content", bson.M{"$literal": page.Content}}}
	for _, field := range contentDerivedFields {
		set[field] = bson.M{"$cond": bson.A{sameContent, "$" + field, "$$REMOVE"}}
	}
	update := mongo.Pipeline{bson.D{{Key: "$set", Value: set}}}
	result, err := client.Database("BlueWhiteThreat").Collection("article").UpdateOne(
		ctx, bson.M{"url": page.URL}, update, options.Update().SetUpsert(true))
	if err != nil {
		return 0, err
	}
	return int(result.UpsertedCount), nil
}
