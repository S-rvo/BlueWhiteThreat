package extractor

import (
	"context"

	"github.com/S-rvo/BlueWhiteThreat/apps/parser/internal/entities"
)

// Extractor transforme un texte brut en entités CTI structurées.
type Extractor interface {
	Extract(ctx context.Context, text string) (entities.Entities, error)
}
