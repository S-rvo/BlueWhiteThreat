package db

import (
	"log"
	"regexp"
)

// onionHashRegex captures the real Tor v3 address (56 base32 chars) embedded
// anywhere in a host, regardless of any subdomain-style prefix in front of it.
var onionHashRegex = regexp.MustCompile(`([a-z2-7]{56})\.onion`)

// farmPrefixThreshold is how many distinct prefixes a single backend hash can
// accumulate before it is treated as a wildcard-subdomain spam farm. A
// legitimate .onion service has no reason to be reachable under more than a
// couple of different subdomain-style prefixes.
const farmPrefixThreshold = 2

// extractOnionHashAndPrefix splits a (lowercased) .onion host into its real
// backend hash and the arbitrary text in front of it. ok is false if no
// v3 onion hash is found.
func extractOnionHashAndPrefix(rawURL string) (hash, prefix string, ok bool) {
	loc := onionHashRegex.FindStringSubmatchIndex(rawURL)
	if loc == nil {
		return "", "", false
	}
	hash = rawURL[loc[2]:loc[3]]
	prefix = rawURL[:loc[2]]
	return hash, prefix, true
}

// IsFarmBackend records the (hash, prefix) pair seen for this URL and reports
// whether its backend hash has now been identified as a wildcard-subdomain
// spam farm (same backend reused under enough distinct auto-generated
// prefixes to defeat keyword filtering). Once a hash crosses the threshold it
// stays blocked for the rest of the crawl via the "dynamic_blocked_hashes"
// Redis set, so it does not need to be hardcoded like blockedBackendHashes.
func IsFarmBackend(rawURL string) bool {
	hash, prefix, ok := extractOnionHashAndPrefix(rawURL)
	if !ok {
		return false
	}

	isBlocked, err := RedisClient.SIsMember(ctx, "dynamic_blocked_hashes", hash).Result()
	if err == nil && isBlocked {
		return true
	}

	key := "farm:prefixes:" + hash
	if err := RedisClient.SAdd(ctx, key, prefix).Err(); err != nil {
		return false
	}
	count, err := RedisClient.SCard(ctx, key).Result()
	if err != nil {
		return false
	}

	if count >= farmPrefixThreshold {
		added, err := RedisClient.SAdd(ctx, "dynamic_blocked_hashes", hash).Result()
		if err == nil && added > 0 {
			log.Printf("Farm backend detected and blocked: %s.onion (%d distinct prefixes)", hash, count)
		}
		return true
	}

	return false
}
