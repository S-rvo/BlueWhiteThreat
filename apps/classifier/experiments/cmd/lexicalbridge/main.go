// lexicalbridge is an offline parity probe, not the production classifier.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/S-rvo/BlueWhiteThreat/apps/classifier/experiments/internal/experimentallexical"
	"os"
	"time"
)

func main() {
	python := flag.String("python", "", "Python environment interpreter")
	script := flag.String("script", "", "stdio scorer script")
	candidate := flag.String("candidate", "", "trusted candidate directory")
	input := flag.String("input", "", "JSONL input")
	output := flag.String("output", "", "new JSONL output")
	flag.Parse()
	if err := run(*python, *script, *candidate, *input, *output); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(python, script, candidate, input, output string) error {
	f, e := os.Open(input)
	if e != nil {
		return e
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	var docs []experimentallexical.Document
	for dec.More() {
		var d experimentallexical.Document
		if e = dec.Decode(&d); e != nil {
			return e
		}
		docs = append(docs, d)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	results, e := (experimentallexical.Bridge{Python: python, Script: script, Candidate: candidate}).Score(ctx, docs)
	if e != nil {
		return e
	}
	out, e := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	defer out.Close()
	enc := json.NewEncoder(out)
	for _, r := range results {
		if e = enc.Encode(r); e != nil {
			return e
		}
	}
	return nil
}
