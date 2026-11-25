package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

func combineVectors(lexical, semantic []float64) []float64 {
	result := append(append(make([]float64, 0, len(lexical)+len(semantic)), lexical...), semantic...)
	sum := 0.0
	for _, value := range result {
		sum += value * value
	}
	if sum > 0 {
		norm := math.Sqrt(sum)
		for i := range result {
			result[i] /= norm
		}
	}
	return result
}

// The encoder is frozen and has no corpus-fitted vocabulary or IDF. Only the
// supervised classifier is fitted separately within each existing fold.
func loadEmbeddings(path string, groups []group) (map[string][]float64, string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	protocol, err := os.ReadFile(filepath.Join(filepath.Dir(path), "protocol.json"))
	if err != nil {
		return nil, "", err
	}
	var cache struct {
		ProtocolSHA string `json:"protocol_sha256"`
		Rows        []struct {
			ID          string    `json:"id"`
			TextSHA     string    `json:"text_sha256"`
			ProtocolSHA string    `json:"protocol_sha256"`
			Characters  int       `json:"characters"`
			Chunks      int       `json:"chunks"`
			Vector      []float64 `json:"vector"`
		} `json:"rows"`
	}
	if err = json.Unmarshal(raw, &cache); err != nil {
		return nil, "", err
	}
	expectedSHA := fmt.Sprintf("%x", sha256.Sum256(protocol))
	if cache.ProtocolSHA != expectedSHA || len(cache.Rows) != len(groups) {
		return nil, "", fmt.Errorf("embedding protocol/count mismatch")
	}
	var manifest struct {
		Dimensions      int      `json:"dimensions"`
		Truncate        bool     `json:"truncate"`
		IDs             []string `json:"representative_ids"`
		BlockCharacters int      `json:"block_characters"`
	}
	if err = json.Unmarshal(protocol, &manifest); err != nil {
		return nil, "", err
	}
	if manifest.Dimensions != 768 || manifest.Truncate || manifest.BlockCharacters != 1000 || len(manifest.IDs) != len(groups) {
		return nil, "", fmt.Errorf("unexpected embedding protocol")
	}
	result := make(map[string][]float64, len(groups))
	for i, row := range cache.Rows {
		g := groups[i]
		characters := len([]rune(g.text))
		if row.ID != g.id || manifest.IDs[i] != g.id || row.TextSHA != fmt.Sprintf("%x", sha256.Sum256([]byte(g.text))) || row.ProtocolSHA != expectedSHA || row.Characters != characters || row.Chunks != (characters+999)/1000 || len(row.Vector) != 768 {
			return nil, "", fmt.Errorf("embedding text/coverage mismatch at row %d", i)
		}
		sum := 0.0
		for _, value := range row.Vector {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return nil, "", fmt.Errorf("nonfinite embedding")
			}
			sum += value * value
		}
		if math.Abs(math.Sqrt(sum)-1) > 1e-6 {
			return nil, "", fmt.Errorf("unnormalized embedding")
		}
		result[g.id] = row.Vector
	}
	return result, fmt.Sprintf("%x", sha256.Sum256(raw)), nil
}
