package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"io"
	"log"
	"os"
	"strings"

	"github.com/S-rvo/BlueWhiteThreat/apps/parser/internal/entityeval"
)

func main() {
	gold := flag.String("gold", "", "private reference JSONL")
	predictions := flag.String("predictions", "", "private prediction JSONL")
	version := flag.String("version", "", "frozen extractor/model/configuration identifier")
	flag.Parse()
	if *gold == "" || *predictions == "" || strings.TrimSpace(*version) == "" || flag.NArg() != 0 {
		flag.Usage()
		os.Exit(2)
	}
	if err := evaluate(*gold, *predictions, *version); err != nil {
		log.Fatal(err)
	}
}

func evaluate(gold, predictions, version string) error {
	g, err := os.Open(gold)
	if err != nil {
		return err
	}
	defer func() { _ = g.Close() }()
	p, err := os.Open(predictions)
	if err != nil {
		return err
	}
	defer func() { _ = p.Close() }()

	gh, ph := sha256.New(), sha256.New()
	result, err := entityeval.Evaluate(io.TeeReader(g, gh), io.TeeReader(p, ph))
	if err != nil {
		return err
	}
	report := struct {
		Version           string `json:"extractor_version"`
		GoldSHA256        string `json:"gold_sha256"`
		PredictionsSHA256 string `json:"predictions_sha256"`
		entityeval.Report
	}{version, hex.EncodeToString(gh.Sum(nil)), hex.EncodeToString(ph.Sum(nil)), result}
	output := json.NewEncoder(os.Stdout)
	output.SetIndent("", "  ")
	return output.Encode(report)
}
