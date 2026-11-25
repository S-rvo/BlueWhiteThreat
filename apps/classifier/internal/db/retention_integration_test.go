//go:build integration

package db

import (
	"context"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestSmokeRetentionReport(t *testing.T) {
	if os.Getenv("BWT_DISPOSABLE_PIPELINE") != "1" || os.Getenv("MONGO_HOST") != "127.0.0.1" ||
		os.Getenv("MONGO_PORT") == "" || os.Getenv("MONGO_PORT") == "27017" {
		t.Skip("requires tests/pipeline/smoke.py with a disposable MongoDB on loopback")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Disconnect(ctx) }()
	coll := articleCollection(client).Database().Collection("retention_smoke_" + primitive.NewObjectID().Hex())
	defer func() { _ = coll.Drop(ctx) }()
	const modelVersion = "$current-fixture" // A model version is a BSON literal, not a field path.
	base := RetentionReport{ModelVersion: modelVersion, PolicyVersion: ClassificationPolicyVersion}
	if got, err := buildRetentionReport(ctx, coll, modelVersion); err != nil || got != base {
		t.Fatalf("empty report: got %+v, err %v", got, err)
	}
	with := func(model, policy string, relevant, review any) bson.M {
		return bson.M{"relevance": bson.M{"model_version": model, "policy_version": policy,
			"relevant": relevant, "needs_review": review}}
	}
	p := ClassificationPolicyVersion
	if _, err := coll.InsertMany(ctx, []any{
		with(modelVersion, p, true, false),     // keep
		with(modelVersion, p, true, true),      // review
		with(modelVersion, p, false, true),     // review
		with(modelVersion, p, false, false),    // discard
		bson.M{"content": "fixture only"},      // missing verdict
		with("older-model", p, false, false),   // stale model
		with(modelVersion, "old", true, false), // stale policy
		with(modelVersion, p, false, nil),      // no explicit review decision
		with(modelVersion, p, "false", false),  // malformed relevance
		with(modelVersion, p, false, bson.A{}), // malformed review decision
	}); err != nil {
		t.Fatal(err)
	}
	base.Total, base.Keep, base.Review, base.Discard, base.Pending = 10, 1, 2, 1, 6
	if got, err := buildRetentionReport(ctx, coll, modelVersion); err != nil || got != base {
		t.Fatalf("retention report: got %+v, want %+v, err %v", got, base, err)
	}
}
