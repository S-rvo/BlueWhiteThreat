package entities

import "testing"

func TestValidateIOCs_FiltersInvalidFormats(t *testing.T) {
	in := Entities{}
	in.IOCs.IPs = []string{"192.168.1.1", "not-an-ip", "999.999.999.999"}
	in.IOCs.Hashes = []string{"d41d8cd98f00b204e9800998ecf8427e", "tooshort"}
	in.IOCs.Domains = []string{"example.onion", "bad domain with space"}
	in.CVEs = []string{"CVE-2024-12345", "CVE-bad"}

	out := ValidateAndClean(in)

	if len(out.IOCs.IPs) != 1 || out.IOCs.IPs[0] != "192.168.1.1" {
		t.Fatalf("expected only valid IP kept, got %v", out.IOCs.IPs)
	}
	if len(out.IOCs.Hashes) != 1 {
		t.Fatalf("expected only valid hash kept, got %v", out.IOCs.Hashes)
	}
	if len(out.IOCs.Domains) != 1 {
		t.Fatalf("expected only valid domain kept, got %v", out.IOCs.Domains)
	}
	if len(out.CVEs) != 1 || out.CVEs[0] != "CVE-2024-12345" {
		t.Fatalf("expected only valid CVE kept, got %v", out.CVEs)
	}
}

func TestIsEmpty(t *testing.T) {
	if !(Entities{}).IsEmpty() {
		t.Fatal("zero-value Entities should be empty")
	}
	e := Entities{}
	e.CVEs = []string{"CVE-2024-1"}
	if e.IsEmpty() {
		t.Fatal("Entities with a CVE should not be empty")
	}
}
