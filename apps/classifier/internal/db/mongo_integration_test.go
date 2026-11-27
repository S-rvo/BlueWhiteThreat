package db

import (
	"context"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

func TestUpdateRelevanceRejectsStaleContent(t *testing.T) {
	if os.Getenv("BWT_PIPELINE_TEST") != "1" {
		t.Skip("opt-in MongoDB integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client, err := Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Disconnect(context.Background())
	coll := articleCollection(client)
	inserted, err := coll.InsertOne(ctx, bson.M{"content": "old content"})
	if err != nil {
		t.Fatal(err)
	}
	defer coll.DeleteOne(ctx, bson.M{"_id": inserted.InsertedID})
	stale := ArticleContent{ID: inserted.InsertedID, Content: "old content"}
	if _, err := coll.UpdateOne(ctx, bson.M{"_id": inserted.InsertedID}, bson.M{"$set": bson.M{"content": "new content"}}); err != nil {
		t.Fatal(err)
	}
	verdict := Relevance{Relevant: true, Score: .9, ModelVersion: "integration-model", PolicyVersion: ClassificationPolicyVersion}
	if err := UpdateRelevance(ctx, client, stale, verdict); err == nil {
		t.Fatal("stale inference was accepted")
	}
	var doc bson.M
	if err := coll.FindOne(ctx, bson.M{"_id": inserted.InsertedID}).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	if _, ok := doc["relevance"]; ok {
		t.Fatal("stale relevance persisted")
	}
	current := ArticleContent{ID: inserted.InsertedID, Content: "new content"}
	if err := UpdateRelevance(ctx, client, current, verdict); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Relevance Relevance `bson:"relevance"`
	}
	if err := coll.FindOne(ctx, bson.M{"_id": inserted.InsertedID}).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Relevance != verdict {
		t.Fatalf("current verdict not stored: %+v", result.Relevance)
	}
}

func TestHistoricalVectorizerResultsAreReclassified(t *testing.T) {
	if os.Getenv("BWT_PIPELINE_TEST") != "1" {
		t.Skip("opt-in MongoDB integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client, err := Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Disconnect(context.Background())
	coll := articleCollection(client)
	inserted, err := coll.InsertOne(ctx, bson.M{"content": "integration fixture", "relevance": Relevance{ModelVersion: "unchanged-model", PolicyVersion: "review-reasons-v2"}})
	if err != nil {
		t.Fatal(err)
	}
	defer coll.DeleteOne(ctx, bson.M{"_id": inserted.InsertedID})
	filter := bson.M{"$and": bson.A{bson.M{"_id": inserted.InsertedID}, classificationFilter("unchanged-model")}}
	count, err := coll.CountDocuments(ctx, filter)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("historical scores were not selected for recalculation")
	}
	if err = UpdateRelevance(ctx, client, ArticleContent{ID: inserted.InsertedID, Content: "integration fixture"}, Relevance{ModelVersion: "unchanged-model", PolicyVersion: ClassificationPolicyVersion}); err != nil {
		t.Fatal(err)
	}
	count, err = coll.CountDocuments(ctx, filter)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("current policy result was unnecessarily selected again")
	}
}

func TestFetchNeedingClassificationPagesPastSkippedArticles(t *testing.T) {
	if os.Getenv("BWT_PIPELINE_TEST") != "1" {
		t.Skip("opt-in MongoDB integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client, err := Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Disconnect(context.Background())
	coll := articleCollection(client)
	const model = "paging-integration-model"
	// Un article sans contenu n'est jamais classé : il reste éligible et ne doit
	// pas bloquer la pagination.
	docs := []any{
		bson.M{"content": "", "entities": bson.M{"cves": bson.A{"CVE-2024-0001"}}},
		bson.M{"content": "second"},
		bson.M{"content": "third"},
	}
	res, err := coll.InsertMany(ctx, docs)
	if err != nil {
		t.Fatal(err)
	}
	defer coll.DeleteMany(context.Background(), bson.M{"_id": bson.M{"$in": res.InsertedIDs}})
	mine := map[interface{}]bool{}
	for _, id := range res.InsertedIDs {
		mine[id] = true
	}

	var seen []interface{}
	var after interface{}
	for i := 0; i < 10_000; i++ {
		batch, err := FetchNeedingClassification(ctx, client, model, after, 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(batch) == 0 {
			break
		}
		if len(batch) > 1 {
			t.Fatalf("limit ignored: %d articles", len(batch))
		}
		if mine[batch[0].ID] {
			seen = append(seen, batch[0].ID)
		}
		after = batch[0].ID
	}
	if len(seen) != len(res.InsertedIDs) {
		t.Fatalf("paged over %d of %d fixtures: %v", len(seen), len(res.InsertedIDs), seen)
	}
	if _, err := FetchNeedingClassification(ctx, client, model, nil, 0); err == nil {
		t.Fatal("unbounded fetch accepted")
	}
}
