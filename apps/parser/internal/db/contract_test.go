package db

import (
	"testing"

	"github.com/S-rvo/BlueWhiteThreat/pkg/contract"
	"github.com/S-rvo/BlueWhiteThreat/pkg/contract/contracttest"
)

// TestArticleViewsMatchContract casse si les vues du parser sur la collection
// article lisent un champ que le contrat partagé ne déclare pas.
func TestArticleViewsMatchContract(t *testing.T) {
	contracttest.RequireBSONTagsEqual(t, UnparsedArticle{}, []string{contract.FieldID, contract.FieldContent})
	contracttest.RequireBSONTagsEqual(t, PendingGraphArticle{}, []string{contract.FieldID, contract.FieldEntities})
}
