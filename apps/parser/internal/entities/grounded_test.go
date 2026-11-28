package entities

import (
	"reflect"
	"testing"
)

func TestKeepGrounded_DropsIndicatorsAbsentFromText(t *testing.T) {
	text := "Visit HTTP://abc.onion/ or contact admin@shop.pro. Exploit CVE-2024-1234 from 10.0.0.1, hash d41d8cd98f00b204e9800998ecf8427e, see market.onion."
	in := Entities{}
	in.IOCs.IPs = []string{"10.0.0.1", "10.0.0.2"}
	in.IOCs.Hashes = []string{"D41D8CD98F00B204E9800998ECF8427E", "e3b0c44298fc1c149afbf4c8996fb924"}
	in.IOCs.Domains = []string{"market.onion", "invented.com"}
	in.IOCs.URLs = []string{"http://abc.onion", "https://apple-tor-store.com", "admin@shop.pro", "),", "hashes"}
	in.CVEs = []string{"CVE-2024-1234", "CVE-2023-0001"}
	in.Malware = []string{"NotInText"}
	in.ThreatActors = []string{"Some Group"}

	out := KeepGrounded(in, text)

	check := func(name string, got, want []string) {
		t.Helper()
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got %v, want %v", name, got, want)
		}
	}
	check("ips", out.IOCs.IPs, []string{"10.0.0.1"})
	check("hashes", out.IOCs.Hashes, []string{"D41D8CD98F00B204E9800998ECF8427E"})
	check("domains", out.IOCs.Domains, []string{"market.onion"})
	check("urls", out.IOCs.URLs, []string{"http://abc.onion"})
	check("cves", out.CVEs, []string{"CVE-2024-1234"})
	check("malware", out.Malware, []string{})
	check("threat_actors", out.ThreatActors, []string{})
}

func TestKeepGrounded_DropsNamesAbsentFromText(t *testing.T) {
	text := "Le groupe APT 28 (Fancy-Bear) diffuse LockBit 3.0 via un serveur Cobalt Strike hébergé chez Bulletproof Host."
	in := Entities{}
	in.Malware = []string{"lockbit-3.0", "Emotet", "  ", "!!"}
	in.ThreatActors = []string{"APT28", "Fancy Bear", "Lazarus Group"}
	in.Infrastructure = []string{"Cobalt Strike", "bulletproof host", "AWS"}

	out := KeepGrounded(in, text)

	for name, pair := range map[string][2][]string{
		"malware":        {out.Malware, {"lockbit-3.0"}},
		"threat_actors":  {out.ThreatActors, {"APT28", "Fancy Bear"}},
		"infrastructure": {out.Infrastructure, {"Cobalt Strike", "bulletproof host"}},
	} {
		if !reflect.DeepEqual(pair[0], pair[1]) {
			t.Errorf("%s: got %v, want %v", name, pair[0], pair[1])
		}
	}
}

func TestLooksLikeURL(t *testing.T) {
	for v, want := range map[string]bool{
		"http://abc.onion/path":       true,
		"abc.onion":                   true,
		"http://user@abc.onion/":      true,
		"admin@shop.pro":              false,
		"),":                          false,
		"domains':['X@Y.COM'],":       false,
		"http://":                     false,
		"https://www.example.com/a b": false,
	} {
		if got := looksLikeURL(v); got != want {
			t.Errorf("looksLikeURL(%q) = %v, want %v", v, got, want)
		}
	}
}
