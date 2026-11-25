package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type mongoStore struct{ articles *mongo.Collection }

func (s mongoStore) list(ctx context.Context, after primitive.ObjectID, limit int64) ([]summary, error) {
	id := bson.M{"$type": "objectId"}
	if !after.IsZero() {
		id["$lt"] = after
	}
	cursor, err := s.articles.Find(ctx, bson.M{"_id": id, "relevance.relevant": true},
		options.Find().SetSort(bson.D{{Key: "_id", Value: -1}}).SetLimit(limit).SetMaxTime(3*time.Second).
			SetProjection(bson.M{"_id": 1, "source_type": 1, "scraped_at": 1}))
	if err != nil {
		return nil, err
	}
	var rows []summary
	err = cursor.All(ctx, &rows)
	return rows, err
}

func (s mongoStore) ready(ctx context.Context) error {
	// Verify collection read permission as well as connectivity, without returning any data.
	err := s.articles.FindOne(ctx, bson.M{}, options.FindOne().SetProjection(bson.M{"_id": 1}).SetMaxTime(2*time.Second)).Err()
	if err == mongo.ErrNoDocuments {
		return nil
	}
	return err
}

type pipelineMetrics struct {
	Articles          int64      `bson:"articles"`
	Classified        int64      `bson:"classified"`
	Relevant          int64      `bson:"relevant"`
	ReviewPending     int64      `bson:"review_pending"`
	ExtractionPending int64      `bson:"extraction_pending"`
	ProjectionPending int64      `bson:"projection_pending"`
	LatestScrape      *time.Time `bson:"latest_scrape"`
}

func (s mongoStore) metrics(ctx context.Context) (pipelineMetrics, error) {
	sum := func(condition any) bson.M { return bson.M{"$sum": bson.M{"$cond": bson.A{condition, 1, 0}}} }
	isType := func(field, kind string) bson.M { return bson.M{"$eq": bson.A{bson.M{"$type": field}, kind}} }
	relevant := bson.M{"$eq": bson.A{"$relevance.relevant", true}}
	review := bson.M{"$eq": bson.A{bson.M{"$ifNull": bson.A{"$relevance.needs_review", false}}, true}}
	eligible := bson.M{"$and": bson.A{relevant, bson.M{"$not": bson.A{review}}}}
	parsed := isType("$entities", "object")
	cursor, err := s.articles.Aggregate(ctx, mongo.Pipeline{{{Key: "$group", Value: bson.M{
		"_id":                nil,
		"articles":           bson.M{"$sum": 1},
		"classified":         sum(isType("$relevance.relevant", "bool")),
		"relevant":           sum(relevant),
		"review_pending":     sum(review),
		"extraction_pending": sum(bson.M{"$and": bson.A{eligible, isType("$entities", "missing")}}),
		"projection_pending": sum(bson.M{"$and": bson.A{eligible, parsed, isType("$graph_projected_at", "missing")}}),
		"latest_scrape":      bson.M{"$max": bson.M{"$cond": bson.A{isType("$scraped_at", "date"), "$scraped_at", nil}}},
	}}}}, options.Aggregate().SetMaxTime(3*time.Second))
	if err != nil {
		return pipelineMetrics{}, err
	}
	var rows []pipelineMetrics
	if err := cursor.All(ctx, &rows); err != nil {
		return pipelineMetrics{}, err
	}
	if len(rows) == 0 {
		return pipelineMetrics{}, nil
	}
	return rows[0], nil
}

func (m pipelineMetrics) render() string {
	var out strings.Builder
	gauge := func(name, help string, value int64) {
		fmt.Fprintf(&out, "# HELP %[1]s %[2]s\n# TYPE %[1]s gauge\n%[1]s %[3]d\n", name, help, value)
	}
	gauge("bwt_articles", "Stored articles.", m.Articles)
	gauge("bwt_classified_articles", "Articles with a classification verdict.", m.Classified)
	gauge("bwt_relevant_articles", "Articles marked relevant.", m.Relevant)
	gauge("bwt_review_pending_articles", "Classified articles flagged for review.", m.ReviewPending)
	gauge("bwt_extraction_pending_articles", "Relevant articles awaiting entity extraction.", m.ExtractionPending)
	gauge("bwt_projection_pending_articles", "Relevant parsed articles awaiting graph projection.", m.ProjectionPending)
	if m.LatestScrape != nil {
		gauge("bwt_latest_scrape_timestamp_seconds", "Latest stored scrape timestamp.", m.LatestScrape.Unix())
	}
	return out.String()
}
