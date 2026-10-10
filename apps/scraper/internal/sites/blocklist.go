package sites

import "strings"

// blockedContentTerms flags pages that index or advertise child sexual abuse
// material. Any match causes the page to be dropped instead of persisted —
// this project only stores CTI-relevant content, never illegal material.
var blockedContentTerms = []string{
	"child porn",
	"childporn",
	"child porno",
	"childporno",
	"pthc",
	"lolita",
	"loli porn",
	"loliporn",
	"jailbait",
	"csam",
	"cp links",
	"hard cp",
	"pedo",
	"pedomom",
	"kidsparty",
	"preteen",
	"pre-teen",
	"underage",
}

// ContainsBlockedContent reports whether content matches a known
// CSAM-indexing term and must not be stored.
func ContainsBlockedContent(content string) bool {
	low := strings.ToLower(content)
	for _, term := range blockedContentTerms {
		if strings.Contains(low, term) {
			return true
		}
	}
	return false
}
