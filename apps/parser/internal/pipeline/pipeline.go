package pipeline

import (
	"context"
	"fmt"

	"github.com/S-rvo/BlueWhiteThreat/apps/parser/internal/entities"
	"github.com/S-rvo/BlueWhiteThreat/apps/parser/internal/extractor"
)

// ExtractWithFallback tente primary (typiquement Ollama, gratuit) puis, en cas
// d'échec ou de résultat vide après validation, escalade vers fallback
// (typiquement Claude). Ça borne le coût du fallback aux cas difficiles.
func ExtractWithFallback(ctx context.Context, primary, fallback extractor.Extractor, text string) (entities.Entities, error) {
	result, err := primary.Extract(ctx, text)
	if err == nil {
		cleaned := entities.ValidateAndClean(result)
		if !cleaned.IsEmpty() {
			return cleaned, nil
		}
	}

	fbResult, fbErr := fallback.Extract(ctx, text)
	if fbErr != nil {
		return entities.Entities{}, fmt.Errorf("primary and fallback both failed: primary_err=%v fallback_err=%w", err, fbErr)
	}
	return entities.ValidateAndClean(fbResult), nil
}
