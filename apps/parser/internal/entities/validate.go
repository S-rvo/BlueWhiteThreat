package entities

import "regexp"

var (
	reIPv4   = regexp.MustCompile(`^(\d{1,3}\.){3}\d{1,3}$`)
	reHash   = regexp.MustCompile(`^[a-fA-F0-9]{32}$|^[a-fA-F0-9]{40}$|^[a-fA-F0-9]{64}$`)
	reDomain = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)+$`)
	reCVE    = regexp.MustCompile(`^CVE-\d{4}-\d{4,7}$`)
	reDot    = regexp.MustCompile(`\.`)
)

func isValidIPv4(s string) bool {
	if !reIPv4.MatchString(s) {
		return false
	}
	parts := reDot.Split(s, -1)
	for _, p := range parts {
		if len(p) > 1 && p[0] == '0' {
			return false
		}
		n := 0
		for _, c := range p {
			n = n*10 + int(c-'0')
		}
		if n > 255 {
			return false
		}
	}
	return true
}

func filter(items []string, valid func(string) bool) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		if valid(it) {
			out = append(out, it)
		}
	}
	return out
}

// ValidateAndClean garde uniquement les entités dont le format est correct.
// Élimine les hallucinations de format produites par le LLM.
func ValidateAndClean(in Entities) Entities {
	out := in
	out.IOCs.IPs = filter(in.IOCs.IPs, isValidIPv4)
	out.IOCs.Hashes = filter(in.IOCs.Hashes, reHash.MatchString)
	out.IOCs.Domains = filter(in.IOCs.Domains, reDomain.MatchString)
	out.CVEs = filter(in.CVEs, reCVE.MatchString)
	// Malware / ThreatActors / Infrastructure : pas de format vérifiable,
	// on les garde tels quels (validation sémantique hors scope regex).
	return out
}
