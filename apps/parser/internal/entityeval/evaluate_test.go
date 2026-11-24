package entityeval

import (
	"strings"
	"testing"
)

func TestScoreByDocumentAndCategory(t *testing.T) {
	gold := `{"id":"one","entities":{"iocs":{"domains":["EXAMPLE.INVALID"],"ips":["192.0.2.42"]}}}
{"id":"two","entities":{"iocs":{"domains":["elsewhere.invalid"]}}}
{"id":"empty","entities":{}}`
	predictions := `{"id":"two","entities":{"iocs":{"domains":["example.invalid"]}}}
{"id":"one","entities":{"iocs":{"domains":[" example.invalid ","example.invalid"]}}}
{"id":"empty","entities":{}}`
	report, err := Evaluate(strings.NewReader(gold), strings.NewReader(predictions))
	if err != nil {
		t.Fatal(err)
	}
	if report.Documents != 3 || report.Micro.Counts != (Counts{TP: 1, FP: 1, FN: 2}) ||
		*report.Micro.Precision != .5 || *report.Micro.Recall != 1.0/3 || *report.Micro.F1 != .4 {
		t.Fatalf("incorrect aggregate: %+v", report)
	}
	if got := report.ByCategory["ips"]; got.FN != 1 || got.Precision != nil || got.Recall == nil || *got.Recall != 0 {
		t.Fatalf("incorrect IP metrics: %+v", got)
	}
	if got := report.ByCategory["urls"]; got.Precision != nil || got.Recall != nil || got.F1 != nil {
		t.Fatalf("undefined or empty category treated as a score: %+v", got)
	}
}

func TestRejectIncompleteOrAmbiguousInputs(t *testing.T) {
	gold := `{"id":"one","entities":{}}`
	for _, prediction := range []string{
		"", `{"id":"other","entities":{}}`, `{"id":"one","entities":null}`, `{"id":"one"}`,
		`{"id":"one","error":"timeout","entities":{}}`, `{"id":"one","entities":{"cves":[" "]}}`,
		gold + "\n" + gold, gold + " " + gold, `{"id":"one","entities":{"unknown":[]}}`,
	} {
		if _, err := Evaluate(strings.NewReader(gold), strings.NewReader(prediction)); err == nil {
			t.Errorf("accepted invalid prediction %q", prediction)
		}
	}
}
