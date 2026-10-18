package extractor

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/S-rvo/BlueWhiteThreat/apps/parser/internal/entities"
)

type ClaudeExtractor struct {
	client anthropic.Client
	model  anthropic.Model
}

func NewClaudeExtractor() *ClaudeExtractor {
	return &ClaudeExtractor{
		client: anthropic.NewClient(),
		model:  anthropic.ModelClaudeHaiku4_5_20251001,
	}
}

const claudeSystemPrompt = `Tu es un extracteur d'entités CTI. Appelle TOUJOURS l'outil
record_entities avec les entités trouvées dans le texte fourni (IOCs, CVEs, malware,
acteurs/groupes, infrastructure). Utilise des tableaux vides pour les catégories sans résultat.`

var recordEntitiesTool = anthropic.ToolParam{
	Name:        "record_entities",
	Description: anthropic.String("Enregistre les entités CTI extraites du texte"),
	InputSchema: anthropic.ToolInputSchemaParam{
		Properties: map[string]any{
			"iocs": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"ips":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"hashes":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"domains": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"urls":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				},
			},
			"cves":           map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"malware":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"threat_actors":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"infrastructure": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		},
	},
}

func parseToolInput(raw []byte) (entities.Entities, error) {
	var out entities.Entities
	if err := json.Unmarshal(raw, &out); err != nil {
		return entities.Entities{}, fmt.Errorf("invalid tool input JSON: %w", err)
	}
	return out, nil
}

func (c *ClaudeExtractor) Extract(ctx context.Context, text string) (entities.Entities, error) {
	resp, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     c.model,
		MaxTokens: 4096,
		System: []anthropic.TextBlockParam{
			{Text: claudeSystemPrompt},
		},
		Tools: []anthropic.ToolUnionParam{{OfTool: &recordEntitiesTool}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(text)),
		},
	})
	if err != nil {
		return entities.Entities{}, fmt.Errorf("claude request failed: %w", err)
	}

	for _, block := range resp.Content {
		if variant, ok := block.AsAny().(anthropic.ToolUseBlock); ok && variant.Name == "record_entities" {
			return parseToolInput([]byte(variant.JSON.Input.Raw()))
		}
	}
	return entities.Entities{}, fmt.Errorf("claude did not call record_entities")
}
