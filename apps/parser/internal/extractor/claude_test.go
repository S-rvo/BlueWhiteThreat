package extractor

import "testing"

func TestParseToolInput_ValidJSON(t *testing.T) {
	raw := []byte(`{"iocs":{"ips":["8.8.8.8"],"hashes":[],"domains":[],"urls":[]},
        "cves":[],"malware":["Cobalt Strike"],"threat_actors":[],"infrastructure":[]}`)
	out, err := parseToolInput(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Malware) != 1 || out.Malware[0] != "Cobalt Strike" {
		t.Fatalf("expected malware parsed, got %v", out.Malware)
	}
}

func TestParseToolInput_InvalidJSON(t *testing.T) {
	_, err := parseToolInput([]byte("not json"))
	if err == nil {
		t.Fatal("expected error on invalid JSON")
	}
}
