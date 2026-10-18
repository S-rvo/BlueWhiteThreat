package extractor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/S-rvo/BlueWhiteThreat/apps/parser/internal/entities"
)

type OllamaExtractor struct {
	baseURL string
	model   string
	client  *http.Client
}

func NewOllamaExtractor(baseURL, model string) *OllamaExtractor {
	return &OllamaExtractor{
		baseURL: baseURL,
		model:   model,
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

const systemPrompt = `Tu es un extracteur d'entités CTI. Analyse le texte fourni et retourne
UNIQUEMENT un objet JSON respectant strictement ce schéma, sans aucun texte autour :
{"iocs":{"ips":[],"hashes":[],"domains":[],"urls":[]},"cves":[],"malware":[],
"threat_actors":[],"infrastructure":[]}
Si aucune entité d'une catégorie n'est trouvée, renvoie un tableau vide pour cette catégorie.`

var jsonSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
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
}

func (o *OllamaExtractor) Extract(ctx context.Context, text string) (entities.Entities, error) {
	reqBody := map[string]any{
		"model": o.model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": text},
		},
		"format": jsonSchema,
		"stream": false,
	}
	buf, err := json.Marshal(reqBody)
	if err != nil {
		return entities.Entities{}, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/api/chat", bytes.NewReader(buf))
	if err != nil {
		return entities.Entities{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return entities.Entities{}, fmt.Errorf("ollama request failed: %w", err)
	}
	defer resp.Body.Close()

	var parsed struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return entities.Entities{}, fmt.Errorf("decode ollama response: %w", err)
	}

	var out entities.Entities
	if err := json.Unmarshal([]byte(parsed.Message.Content), &out); err != nil {
		return entities.Entities{}, fmt.Errorf("model output is not valid JSON: %w", err)
	}
	return out, nil
}
