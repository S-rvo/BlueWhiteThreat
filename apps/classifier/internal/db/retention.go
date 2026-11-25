package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// RetentionReport describes a simulation, not changes applied to documents.
type RetentionReport struct {
	ModelVersion  string `json:"model_version"`
	PolicyVersion string `json:"policy_version"`
	Total         int64  `json:"total"`
	Keep          int64  `json:"keep"`
	Review        int64  `json:"review"`
	Discard       int64  `json:"discard"`
	Pending       int64  `json:"pending"`
}

// BuildRetentionReport reads current verdict metadata only and returns counts.
func BuildRetentionReport(ctx context.Context, client *mongo.Client, modelVersion string) (RetentionReport, error) {
	return buildRetentionReport(ctx, articleCollection(client), modelVersion)
}

func buildRetentionReport(ctx context.Context, coll *mongo.Collection, modelVersion string) (RetentionReport, error) {
	if strings.TrimSpace(modelVersion) == "" {
		return RetentionReport{}, fmt.Errorf("db: model version is required for a retention report")
	}
	// Missing, stale and malformed verdicts stay pending. Only an explicit,
	// current negative with needs_review=false can count as discard.
	eq := func(left, right any) bson.M { return bson.M{"$eq": bson.A{left, right}} }
	typeIsBool := func(path string) bson.M { return eq(bson.M{"$type": path}, "bool") }
	valid := bson.M{"$and": bson.A{
		eq("$relevance.model_version", bson.M{"$literal": modelVersion}),
		eq("$relevance.policy_version", ClassificationPolicyVersion),
		typeIsBool("$relevance.relevant"),
		typeIsBool("$relevance.needs_review"),
	}}
	decision := bson.M{"$switch": bson.M{
		"branches": bson.A{
			bson.M{"case": bson.M{"$not": bson.A{valid}}, "then": "pending"},
			bson.M{"case": eq("$relevance.needs_review", true), "then": "review"},
			bson.M{"case": eq("$relevance.relevant", true), "then": "keep"},
		},
		"default": "discard",
	}}
	pipeline := mongo.Pipeline{
		{{Key: "$project", Value: bson.M{"_id": 0, "decision": decision}}},
		{{Key: "$group", Value: bson.M{"_id": "$decision", "count": bson.M{"$sum": 1}}}},
	}
	cur, err := coll.Aggregate(ctx, pipeline, options.Aggregate().SetMaxTime(15*time.Second).SetAllowDiskUse(false))
	if err != nil {
		return RetentionReport{}, fmt.Errorf("db: aggregate retention decisions: %w", err)
	}
	var counts []struct {
		Decision string `bson:"_id"`
		Count    int64  `bson:"count"`
	}
	if err := cur.All(ctx, &counts); err != nil {
		return RetentionReport{}, fmt.Errorf("db: read retention decisions: %w", err)
	}
	r := RetentionReport{ModelVersion: modelVersion, PolicyVersion: ClassificationPolicyVersion}
	for _, bucket := range counts {
		switch bucket.Decision {
		case "keep":
			r.Keep = bucket.Count
		case "review":
			r.Review = bucket.Count
		case "discard":
			r.Discard = bucket.Count
		case "pending":
			r.Pending = bucket.Count
		default:
			return RetentionReport{}, fmt.Errorf("db: unexpected retention decision")
		}
		r.Total += bucket.Count
	}
	return r, nil
}
