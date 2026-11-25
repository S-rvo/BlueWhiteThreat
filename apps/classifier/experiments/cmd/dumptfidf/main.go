// apps/classifier/experiments/cmd/dumptfidf/main.go
//
// One-off tool for docs/pistes-a-creuser-2026-11-05.md point 1: dumps the
// frozen production TF-IDF representation (vocabulary + IDF from model.json,
// no refitting) for every row of a CSV, as raw little-endian float64 rows,
// so a Python script can pair it against a frozen encoder embedding under
// the same cross-validation folds. Not part of the production pipeline.
package main

import (
	"encoding/binary"
	"encoding/csv"
	"flag"
	"log"
	"math"
	"os"

	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/model"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/internal/tfidf"
)

func main() {
	modelPath := flag.String("model", "model.json", "frozen production model")
	inputPath := flag.String("input", "", "CSV with id,text,label")
	outPrefix := flag.String("out", "", "output prefix: writes <prefix>.bin, <prefix>.ids.txt, <prefix>.labels.txt")
	flag.Parse()

	if *inputPath == "" || *outPrefix == "" {
		log.Fatal("-input and -out are required")
	}

	m, err := model.Load(*modelPath)
	if err != nil {
		log.Fatalf("load model: %v", err)
	}
	vectorizer := &tfidf.Vectorizer{Vocabulary: m.Vocabulary, IDF: m.IDF, LegacyCharNgrams: m.CharNgrams, LegacySublinearTF: m.SublinearTF}
	dims := len(m.IDF)

	file, err := os.Open(*inputPath)
	if err != nil {
		log.Fatalf("open input: %v", err)
	}
	defer file.Close()
	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	rows, err := reader.ReadAll()
	if err != nil {
		log.Fatalf("read csv: %v", err)
	}
	if len(rows) == 0 || rows[0][0] != "id" {
		log.Fatal("expected header id,text,label")
	}
	rows = rows[1:]

	binFile, err := os.Create(*outPrefix + ".bin")
	if err != nil {
		log.Fatalf("create bin: %v", err)
	}
	defer binFile.Close()
	idsFile, err := os.Create(*outPrefix + ".ids.txt")
	if err != nil {
		log.Fatalf("create ids: %v", err)
	}
	defer idsFile.Close()
	labelsFile, err := os.Create(*outPrefix + ".labels.txt")
	if err != nil {
		log.Fatalf("create labels: %v", err)
	}
	defer labelsFile.Close()

	buf := make([]byte, 8)
	written := 0
	for _, row := range rows {
		if len(row) != 3 {
			continue
		}
		id, text, label := row[0], row[1], row[2]
		vec := vectorizer.Transform(text)
		if len(vec) != dims {
			log.Fatalf("row %s: vector dims %d != model dims %d", id, len(vec), dims)
		}
		for _, v := range vec {
			binary.LittleEndian.PutUint64(buf, math.Float64bits(v))
			if _, err := binFile.Write(buf); err != nil {
				log.Fatalf("write bin: %v", err)
			}
		}
		if _, err := idsFile.WriteString(id + "\n"); err != nil {
			log.Fatalf("write ids: %v", err)
		}
		if _, err := labelsFile.WriteString(label + "\n"); err != nil {
			log.Fatalf("write labels: %v", err)
		}
		written++
	}
	log.Printf("wrote %d rows, %d dims, to %s.bin", written, dims, *outPrefix)
}
