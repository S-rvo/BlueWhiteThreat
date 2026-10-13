// Freeze a candidate, then optionally sample an unlabeled, independent pool.
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/confirmation"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
func run() error {
	dir := env("CONFIRMATION_DIR", "data/confirmation-campaign")
	if _, err := os.Stat(filepath.Join(dir, "manifest.json")); os.IsNotExist(err) {
		path := os.Getenv("CANDIDATE_MODEL_PATH")
		if path == "" {
			return fmt.Errorf("CANDIDATE_MODEL_PATH is required to freeze a new campaign")
		}
		c, err := confirmation.Freeze(dir, path, env("TRAINING_DATA", "training_data.csv"), ".")
		if err != nil {
			return err
		}
		log.Printf("Frozen candidate %s, threshold %.6f, %d reference documents, model SHA256 %s", c.Manifest.ModelVersion, c.Manifest.Threshold, c.Manifest.ReferenceDocuments, c.Manifest.ModelSHA256)
	} else {
		if _, err := confirmation.Load(dir, "."); err != nil {
			return err
		}
		log.Printf("Existing frozen campaign verified: %s", dir)
	}
	pool := os.Getenv("CONFIRMATION_POOL")
	if pool == "" {
		log.Println("No independent pool supplied. Candidate frozen; no annotation batch or result fabricated. Supply CONFIRMATION_POOL (id,text CSV) to prepare annotations.")
		return nil
	}
	n, err := strconv.Atoi(env("CONFIRMATION_SIZE", "300"))
	if err != nil {
		return err
	}
	seed, err := strconv.ParseInt(env("CONFIRMATION_SEED", "42"), 10, 64)
	if err != nil {
		return err
	}
	selection, err := confirmation.Prepare(dir, pool, ".", n, seed)
	if err != nil {
		return err
	}
	log.Printf("Prepared %d/%d requested documents; %d overlapping documents skipped while sampling. Annotate %s without consulting model scores.", len(selection.Documents), n, selection.Excluded, filepath.Join(dir, "annotations.csv"))
	return nil
}
func env(key, fallback string) string {
	if s := os.Getenv(key); s != "" {
		return s
	}
	return fallback
}
