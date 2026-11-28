package contract

// Base et collection du document article, partagées par tous les services.
const (
	Database          = "BlueWhiteThreat"
	ArticleCollection = "article"
)

// Champs du document article. Le commentaire de chaque groupe désigne le seul
// service autorisé à l'écrire ; les autres ne font que le lire.
const (
	// Écrits par le scraper (upsert par URL).
	FieldID         = "_id"
	FieldURL        = "url"
	FieldSource     = "source"
	FieldSourceType = "source_type"
	FieldScrapedAt  = "scraped_at"
	FieldContent    = "content"

	// Écrit par le classifier : sous-document de verdict.
	FieldRelevance = "relevance"

	// Écrits par le parser : extraction, suivi des échecs, projection Neo4j.
	FieldEntities                = "entities"
	FieldExtractionFailures      = "extraction_failures"
	FieldExtractionRetryAt       = "extraction_retry_at"
	FieldExtractionLastError     = "extraction_last_error"
	FieldGraphProjectionQueuedAt = "graph_projection_queued_at"
	FieldGraphProjectedAt        = "graph_projected_at"
)

// Clés du sous-document relevance (noms courts, tels qu'écrits en bson).
const (
	RelevanceRelevant        = "relevant"
	RelevanceScore           = "score"
	RelevanceCalibratedScore = "calibrated_score"
	RelevanceModelVersion    = "model_version"
	RelevancePolicyVersion   = "policy_version"
	RelevanceNeedsReview     = "needs_review"
	RelevanceReviewReason    = "review_reason"
)

// Chemins pointés vers relevance, utilisés dans les filtres et index.
const (
	PathRelevanceRelevant      = FieldRelevance + "." + RelevanceRelevant
	PathRelevanceNeedsReview   = FieldRelevance + "." + RelevanceNeedsReview
	PathRelevanceModelVersion  = FieldRelevance + "." + RelevanceModelVersion
	PathRelevancePolicyVersion = FieldRelevance + "." + RelevancePolicyVersion
)

// Valeurs de source_type écrites par le scraper.
const (
	SourceTypeDarkwebPage = "darkweb_page"
	SourceTypeUnknown     = "unknown"
)

// ContentDerivedFields liste les champs calculés à partir de content. Le
// scraper les efface quand le contenu d'une page change, pour que le classifier
// et le parser recalculent sur le nouveau texte. graph_projected_at n'en fait
// pas partie : il reste posé tant que l'article est dans le graphe, afin que le
// parser puisse l'en retirer s'il n'est plus éligible.
func ContentDerivedFields() []string {
	return []string{
		FieldRelevance,
		FieldEntities,
		FieldGraphProjectionQueuedAt,
		FieldExtractionFailures,
		FieldExtractionRetryAt,
		FieldExtractionLastError,
	}
}

// ArticleFields liste tous les champs de premier niveau du contrat.
func ArticleFields() []string {
	return []string{
		FieldID, FieldURL, FieldSource, FieldSourceType, FieldScrapedAt, FieldContent,
		FieldRelevance,
		FieldEntities, FieldExtractionFailures, FieldExtractionRetryAt, FieldExtractionLastError,
		FieldGraphProjectionQueuedAt, FieldGraphProjectedAt,
	}
}

// RelevanceFields liste les clés du sous-document relevance.
func RelevanceFields() []string {
	return []string{
		RelevanceRelevant, RelevanceScore, RelevanceCalibratedScore,
		RelevanceModelVersion, RelevancePolicyVersion, RelevanceNeedsReview, RelevanceReviewReason,
	}
}
