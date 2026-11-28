package db

import (
	"testing"

	"github.com/S-rvo/BlueWhiteThreat/pkg/contract"
	"github.com/S-rvo/BlueWhiteThreat/pkg/contract/contracttest"
)

// TestArticleViewsMatchContract casse si le classifier lit ou écrit sur la
// collection article un champ que le contrat partagé ne déclare pas.
func TestArticleViewsMatchContract(t *testing.T) {
	contracttest.RequireBSONTagsEqual(t, ArticleContent{}, []string{
		contract.FieldID, contract.FieldURL, contract.FieldSource, contract.FieldSourceType,
		contract.FieldScrapedAt, contract.FieldContent, contract.FieldRelevance,
	})
	contracttest.RequireBSONTagsIn(t, ArticleContent{}, contract.ArticleFields())
	contracttest.RequireBSONTagsEqual(t, Relevance{}, contract.RelevanceFields())
}
