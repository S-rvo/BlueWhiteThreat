// apps/classifier/internal/db/mongo.go
package db

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func getEnvOrDefault(key, defaultValue string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return defaultValue
}

// Connect opens a MongoDB connection using the same MONGO_* environment
// variables as apps/scraper, so both services can share one .env in compose.
func Connect(ctx context.Context) (*mongo.Client, error) {
	user := getEnvOrDefault("MONGO_INITDB_ROOT_USERNAME", "root")
	pass := getEnvOrDefault("MONGO_INITDB_ROOT_PASSWORD", "root")
	host := getEnvOrDefault("MONGO_HOST", "localhost")
	port := getEnvOrDefault("MONGO_PORT", "27017")

	uri := "mongodb://" + user + ":" + pass + "@" + host + ":" + port + "/"
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, fmt.Errorf("db: connect: %w", err)
	}
	return client, nil
}

// ArticleContent is the minimal projection needed for scoring: the article's
// Mongo _id and its scraped text.
type ArticleContent struct {
	ID        interface{} `bson:"_id"`
	URL       string      `bson:"url"`
	Content   string      `bson:"content"`
	Relevance *Relevance  `bson:"relevance"`
}

func articleCollection(client *mongo.Client) *mongo.Collection {
	dbName := getEnvOrDefault("MONGO_DB", "BlueWhiteThreat")
	return client.Database(dbName).Collection("article")
}

func focusMatchCollection(client *mongo.Client) *mongo.Collection {
	dbName := getEnvOrDefault("MONGO_DB", "BlueWhiteThreat")
	return client.Database(dbName).Collection("focus_match")
}

func focusRunCollection(client *mongo.Client) *mongo.Collection {
	dbName := getEnvOrDefault("MONGO_DB", "BlueWhiteThreat")
	return client.Database(dbName).Collection("focus_profile_run")
}

// EnsureFocusMatchIndex guarantees that concurrent or repeated profile runs
// keep one current result per article/profile pair.
func EnsureFocusMatchIndex(ctx context.Context, client *mongo.Client) error {
	if err := dropIndexIfPresent(ctx, focusMatchCollection(client), "article_id_1_profile_id_1_profile_hash_1"); err != nil {
		return fmt.Errorf("db: replace legacy focus match index: %w", err)
	}
	_, err := focusMatchCollection(client).Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "article_id", Value: 1}, {Key: "profile_id", Value: 1}, {Key: "profile_hash", Value: 1}, {Key: "run_id", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return fmt.Errorf("db: ensure focus match index: %w", err)
	}
	if err := dropIndexIfPresent(ctx, focusRunCollection(client), "profile_id_1_profile_hash_1"); err != nil {
		return fmt.Errorf("db: replace legacy focus profile run index: %w", err)
	}
	_, err = focusRunCollection(client).Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "profile_id", Value: 1}, {Key: "profile_hash", Value: 1}, {Key: "run_id", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return fmt.Errorf("db: ensure focus profile run index: %w", err)
	}
	return nil
}

func dropIndexIfPresent(ctx context.Context, collection *mongo.Collection, name string) error {
	cursor, err := collection.Indexes().List(ctx)
	if err != nil {
		return err
	}
	defer cursor.Close(ctx)
	for cursor.Next(ctx) {
		var index bson.M
		if err := cursor.Decode(&index); err != nil {
			return err
		}
		if index["name"] == name {
			_, err := collection.Indexes().DropOne(ctx, name)
			return err
		}
	}
	return cursor.Err()
}

// FocusProfileRun records whether a versioned profile evaluation completed.
type FocusProfileRun struct {
	RunID       string    `bson:"run_id" json:"run_id"`
	ProfileID   string    `bson:"profile_id" json:"profile_id"`
	ProfileHash string    `bson:"profile_hash" json:"profile_hash"`
	ProfileJSON string    `bson:"profile_json" json:"profile_json"`
	Status      string    `bson:"status" json:"status"`
	Error       string    `bson:"error,omitempty" json:"error,omitempty"`
	StartedAt   time.Time `bson:"started_at" json:"started_at"`
	CompletedAt time.Time `bson:"completed_at,omitempty" json:"completed_at,omitempty"`
	Articles    int       `bson:"articles" json:"articles"`
	Matched     int       `bson:"matched" json:"matched"`
}

// FocusArticle joins a completed profile match to displayable article fields.
type FocusArticle struct {
	RunID       string      `bson:"run_id"`
	ArticleID   interface{} `bson:"article_id"`
	ProfileID   string      `bson:"profile_id"`
	ProfileHash string      `bson:"profile_hash"`
	Score       float64     `bson:"score"`
	Includes    []string    `bson:"matched_includes"`
	EvaluatedAt time.Time   `bson:"evaluated_at"`
	Title       string      `bson:"title"`
	URL         string      `bson:"url"`
	Source      string      `bson:"source"`
}

// GetFocusProfileRun returns the persisted state for one exact profile version.
func GetFocusProfileRun(ctx context.Context, client *mongo.Client, runID string) (FocusProfileRun, error) {
	var run FocusProfileRun
	filter := bson.M{"run_id": runID}
	if err := focusRunCollection(client).FindOne(ctx, filter).Decode(&run); err != nil {
		return FocusProfileRun{}, fmt.Errorf("db: find focus profile run %q: %w", runID, err)
	}
	return run, nil
}

// FetchFocusProfileRuns returns the most recent profile evaluation runs.
func FetchFocusProfileRuns(ctx context.Context, client *mongo.Client, limit int64) ([]FocusProfileRun, error) {
	if limit <= 0 {
		limit = 100
	}
	cursor, err := focusRunCollection(client).Find(ctx, bson.M{}, options.Find().
		SetSort(bson.D{{Key: "started_at", Value: -1}}).
		SetLimit(limit))
	if err != nil {
		return nil, fmt.Errorf("db: fetch focus profile runs: %w", err)
	}
	defer cursor.Close(ctx)
	var runs []FocusProfileRun
	if err := cursor.All(ctx, &runs); err != nil {
		return nil, fmt.Errorf("db: decode focus profile runs: %w", err)
	}
	return runs, nil
}

// ForEachMatchedFocusArticle streams positive matches for one completed
// profile version and joins the article's title, URL, and source.
func ForEachMatchedFocusArticle(ctx context.Context, client *mongo.Client, runID string, visit func(FocusArticle) error) error {
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"run_id": runID, "matched": true}}},
		{{Key: "$lookup", Value: bson.M{"from": "article", "localField": "article_id", "foreignField": "_id", "as": "article"}}},
		{{Key: "$unwind", Value: "$article"}},
		{{Key: "$project", Value: bson.M{
			"run_id": 1, "article_id": 1, "profile_id": 1, "profile_hash": 1, "score": 1,
			"matched_includes": 1, "evaluated_at": 1,
			"title": "$article.title", "url": "$article.url", "source": "$article.source",
		}}},
	}
	cursor, err := focusMatchCollection(client).Aggregate(ctx, pipeline)
	if err != nil {
		return fmt.Errorf("db: aggregate matched focus articles: %w", err)
	}
	defer cursor.Close(ctx)
	for cursor.Next(ctx) {
		var article FocusArticle
		if err := cursor.Decode(&article); err != nil {
			return fmt.Errorf("db: decode matched focus article: %w", err)
		}
		if err := visit(article); err != nil {
			return fmt.Errorf("db: export matched article %v: %w", article.ArticleID, err)
		}
	}
	if err := cursor.Err(); err != nil {
		return fmt.Errorf("db: iterate matched focus articles: %w", err)
	}
	return nil
}

// StartFocusProfileRun marks a profile version as in progress. Results are
// retained by version, so interrupted runs cannot overwrite prior snapshots.
func StartFocusProfileRun(ctx context.Context, client *mongo.Client, run FocusProfileRun) error {
	run.Status = "running"
	filter := bson.M{"run_id": run.RunID}
	_, err := focusRunCollection(client).UpdateOne(ctx, filter, bson.M{
		"$set":   bson.M{"status": run.Status, "started_at": run.StartedAt, "profile_id": run.ProfileID, "profile_hash": run.ProfileHash, "profile_json": run.ProfileJSON, "articles": 0, "matched": 0},
		"$unset": bson.M{"completed_at": "", "error": ""},
	}, options.Update().SetUpsert(true))
	if err != nil {
		return fmt.Errorf("db: start focus profile run %q: %w", run.ProfileID, err)
	}
	return nil
}

// CompleteFocusProfileRun publishes a version as complete after every batch
// has been written successfully.
func CompleteFocusProfileRun(ctx context.Context, client *mongo.Client, run FocusProfileRun) error {
	filter := bson.M{"run_id": run.RunID}
	update := bson.M{"$set": bson.M{
		"status": "completed", "completed_at": run.CompletedAt,
		"articles": run.Articles, "matched": run.Matched,
	}, "$unset": bson.M{"error": ""}}
	if _, err := focusRunCollection(client).UpdateOne(ctx, filter, update); err != nil {
		return fmt.Errorf("db: complete focus profile run %q: %w", run.ProfileID, err)
	}
	return nil
}

// FailFocusProfileRun records why an execution did not produce an exportable snapshot.
func FailFocusProfileRun(ctx context.Context, client *mongo.Client, run FocusProfileRun, cause error) error {
	filter := bson.M{"run_id": run.RunID}
	update := bson.M{"$set": bson.M{
		"status": "failed", "completed_at": time.Now().UTC(), "error": cause.Error(),
	}}
	if _, err := focusRunCollection(client).UpdateOne(ctx, filter, update); err != nil {
		return fmt.Errorf("db: fail focus profile run %q: %w", run.RunID, err)
	}
	return nil
}

// FocusMatch stores a profile-specific result separately from global relevance.
type FocusMatch struct {
	RunID       string      `bson:"run_id"`
	ArticleID   interface{} `bson:"article_id"`
	ProfileID   string      `bson:"profile_id"`
	ProfileHash string      `bson:"profile_hash"`
	Matched     bool        `bson:"matched"`
	Score       float64     `bson:"score"`
	Includes    []string    `bson:"matched_includes,omitempty"`
	Excludes    []string    `bson:"matched_excludes,omitempty"`
	EvaluatedAt time.Time   `bson:"evaluated_at"`
}

// UpdateFocusMatch upserts one article/profile result in its own collection.
// This deliberately leaves the article's global relevance field untouched.
func UpdateFocusMatch(ctx context.Context, client *mongo.Client, result FocusMatch) error {
	return UpdateFocusMatches(ctx, client, []FocusMatch{result})
}

// UpdateFocusMatches writes profile results in one Mongo bulk operation.
func UpdateFocusMatches(ctx context.Context, client *mongo.Client, results []FocusMatch) error {
	if len(results) == 0 {
		return nil
	}
	models := make([]mongo.WriteModel, 0, len(results))
	for _, result := range results {
		filter := bson.M{"article_id": result.ArticleID, "profile_id": result.ProfileID, "profile_hash": result.ProfileHash, "run_id": result.RunID}
		models = append(models, mongo.NewUpdateOneModel().
			SetFilter(filter).
			SetUpdate(bson.M{"$set": result}).
			SetUpsert(true))
	}
	_, err := focusMatchCollection(client).BulkWrite(ctx, models, options.BulkWrite().SetOrdered(false))
	if err != nil {
		return fmt.Errorf("db: bulk update %d focus matches: %w", len(results), err)
	}
	return nil
}

// FetchAllContent returns every article's id and content, regardless of
// relevance status — used by cmd/bootstrap to build a labeling candidate set.
func FetchAllContent(ctx context.Context, client *mongo.Client) ([]ArticleContent, error) {
	var results []ArticleContent
	if err := ForEachArticleContent(ctx, client, func(article ArticleContent) error {
		results = append(results, article)
		return nil
	}); err != nil {
		return nil, err
	}
	return results, nil
}

// ForEachArticleContent visits article ids and content through a Mongo cursor,
// keeping memory use bounded for corpus-wide processing.
func ForEachArticleContent(ctx context.Context, client *mongo.Client, visit func(ArticleContent) error) error {
	cursor, err := articleCollection(client).Find(ctx, bson.M{}, options.Find().SetProjection(bson.M{"_id": 1, "url": 1, "content": 1, "relevance": 1}))
	if err != nil {
		return fmt.Errorf("db: fetch all content: %w", err)
	}
	defer cursor.Close(ctx)
	for cursor.Next(ctx) {
		var article ArticleContent
		if err := cursor.Decode(&article); err != nil {
			return fmt.Errorf("db: decode article content: %w", err)
		}
		if err := visit(article); err != nil {
			return fmt.Errorf("db: process article %v: %w", article.ID, err)
		}
	}
	if err := cursor.Err(); err != nil {
		return fmt.Errorf("db: iterate article content: %w", err)
	}
	return nil
}

// ClassificationPolicyVersion identifies inference and review rules outside
// the model artifact. Bump it when these rules change so stored results refresh.
const ClassificationPolicyVersion = "review-reasons-v2"

// classificationFilter selects articles with no relevance result or a result
// produced by another model or policy version. MongoDB's $ne also matches documents
// where the nested field is absent, which includes legacy relevance records.
func classificationFilter(modelVersion string) bson.M {
	return bson.M{"$or": bson.A{
		bson.M{"relevance": bson.M{"$exists": false}},
		bson.M{"relevance.model_version": bson.M{"$ne": modelVersion}},
		bson.M{"relevance.policy_version": bson.M{"$ne": ClassificationPolicyVersion}},
	}}
}

// FetchNeedingClassification returns articles that have not been scored by
// modelVersion and the current policy. Existing results matching both versions
// are left untouched; missing and stale results are recalculated.
func FetchNeedingClassification(ctx context.Context, client *mongo.Client, modelVersion string) ([]ArticleContent, error) {
	if modelVersion == "" {
		return nil, fmt.Errorf("db: model version is required to fetch articles for classification")
	}
	filter := classificationFilter(modelVersion)
	cursor, err := articleCollection(client).Find(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("db: fetch articles for model %q: %w", modelVersion, err)
	}
	defer cursor.Close(ctx)

	var results []ArticleContent
	if err := cursor.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("db: decode articles for model %q: %w", modelVersion, err)
	}
	return results, nil
}

// Relevance mirrors the "relevance" subdocument written onto each article.
type Relevance struct {
	Relevant        bool    `bson:"relevant"`
	Score           float64 `bson:"score"`
	CalibratedScore float64 `bson:"calibrated_score"`
	ModelVersion    string  `bson:"model_version"`
	PolicyVersion   string  `bson:"policy_version"`
	NeedsReview     bool    `bson:"needs_review"`
	ReviewReason    string  `bson:"review_reason,omitempty"`
}

// UpdateRelevance sets the "relevance" field on the article identified by id.
func UpdateRelevance(ctx context.Context, client *mongo.Client, id interface{}, r Relevance) error {
	_, err := articleCollection(client).UpdateOne(
		ctx,
		bson.M{"_id": id},
		bson.M{"$set": bson.M{"relevance": r}},
	)
	if err != nil {
		return fmt.Errorf("db: update relevance for %v: %w", id, err)
	}
	return nil
}
