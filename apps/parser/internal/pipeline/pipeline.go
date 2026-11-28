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
		cleaned := clean(result, text)
		if !cleaned.IsEmpty() || fallback == nil {
			return cleaned, nil
		}
	}

	if fallback == nil {
		return entities.Entities{}, err
	}

	fbResult, fbErr := fallback.Extract(ctx, text)
	if fbErr != nil {
		return entities.Entities{}, fmt.Errorf("primary and fallback both failed: primary_err=%v fallback_err=%w", err, fbErr)
	}
	return clean(fbResult, text), nil
}

// clean applique la validation de format puis retire les indicateurs absents
// du texte source.
func clean(e entities.Entities, text string) entities.Entities {
	return entities.KeepGrounded(entities.ValidateAndClean(e), text)
}
