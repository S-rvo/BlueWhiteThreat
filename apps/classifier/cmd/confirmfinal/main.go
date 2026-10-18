// Confirm the frozen candidate exactly once on its independently annotated lot.
// Run from apps/classifier, after cmd/prepareconfirmation. Never trains or tunes.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/confirmation"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
func run() error {
	dir := env("CONFIRMATION_DIR", "data/confirmation-campaign")
	labels := env("CONFIRMATION_LABELS_PATH", filepath.Join(dir, "annotations.csv"))
	if _, err := os.Stat(labels); os.IsNotExist(err) {
		log.Println("No new annotated confirmation batch yet; nothing scored.")
		return nil
	} else if err != nil {
		return err
	}
	result, err := confirmation.Confirm(dir, labels, ".")
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	if !result.TargetSupported {
		log.Println("The 99% precision target is NOT supported by this confirmation's 95% Wilson lower bound. No model promotion performed.")
	}
	return nil
}
func env(key, fallback string) string {
	if s := os.Getenv(key); s != "" {
		return s
	}
	return fallback
}
