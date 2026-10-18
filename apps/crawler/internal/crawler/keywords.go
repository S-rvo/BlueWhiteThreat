package crawler

import "strings"

// relevantKeywords are cyber-threat/darkweb terms used to keep the crawl
// focused on content useful to the classifier (hacking, malware, leaks,
// fraud...) instead of expanding into unrelated link farms. A link is only
// queued for crawling if its URL or anchor text contains at least one of
// these terms.
var relevantKeywords = []string{
	"hack", "hacker", "hacking",
	"exploit", "vulnerab", "cve", "0day", "zero-day", "zeroday",
	"malware", "ransom", "trojan", "rootkit", "keylogger", "spyware", "worm", "virus",
	"breach", "leak", "dump", "database", "credential", "creds",
	"phish", "botnet", "ddos", "c2", "backdoor",
	"carding", "cvv", "fraud", "stealer", "rat",
}

// isRelevantLink reports whether the given URL or its anchor text contains
// at least one cyber-threat-intel keyword. Matching is case-insensitive and
// checks both strings combined so hash-only onion domains can still match on
// a descriptive anchor text, and vice versa.
func isRelevantLink(rawURL, anchorText string) bool {
	combined := strings.ToLower(rawURL + " " + anchorText)
	for _, keyword := range relevantKeywords {
		if strings.Contains(combined, keyword) {
			return true
		}
	}
	return false
}
