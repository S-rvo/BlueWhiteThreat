package main

import (
	"testing"

	"github.com/S-rvo/BlueWhiteThreat/pkg/contract"
	"github.com/S-rvo/BlueWhiteThreat/pkg/contract/contracttest"
)

// TestSummaryMatchesContract casse si la vue exposée par l'API lit un champ
// article que le contrat partagé ne déclare pas.
func TestSummaryMatchesContract(t *testing.T) {
	contracttest.RequireBSONTagsEqual(t, summary{}, []string{contract.FieldID, contract.FieldSourceType, contract.FieldScrapedAt})
}
