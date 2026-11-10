package db

import "testing"

func TestPriorityScoreRequiresContextForAmbiguousTerms(t *testing.T) {
	if got := priorityScore("http://example.onion/database"); got != 0 {
		t.Fatalf("generic database priority = %d, want 0", got)
	}
	if got := priorityScore("http://example.onion/corporate-database-leak"); got != 1 {
		t.Fatalf("corroborated leak priority = %d, want 1", got)
	}
	if got := priorityScore("http://example.onion/initial-access"); got != 2 {
		t.Fatalf("strong expression priority = %d, want 2", got)
	}
}

func TestPrioritizeURLsKeepsCollectionOpenButBoundsCycle(t *testing.T) {
	input := []string{
		"http://hash-only-a.onion",
		"http://example.onion/corporate-database-leak",
		"http://example.onion/ransomware",
		"http://hash-only-b.onion",
	}
	got := prioritizeURLs(input, nil, 3)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if got[0] != input[2] || got[1] != input[1] {
		t.Fatalf("priority order = %v", got)
	}
	all := prioritizeURLs(input, nil, len(input))
	seen := map[string]bool{}
	for _, url := range all {
		seen[url] = true
	}
	for _, url := range input {
		if !seen[url] {
			t.Fatalf("unprioritized URL was dropped: %s", url)
		}
	}
}

// La fraîcheur départage à pertinence CTI égale : au sein d'un même tier, l'URL
// crawlée le plus récemment passe en premier (le service .onion a le plus de
// chances d'être encore en vie). C'est le cœur de la priorisation par fraîcheur.
func TestPrioritizeURLsFreshestFirstWithinSameRelevanceTier(t *testing.T) {
	older := "http://example.onion/ransomware-old"
	newer := "http://example.onion/ransomware-new"
	freshness := map[string]float64{older: 1_700_000_000, newer: 1_700_009_999}
	got := prioritizeURLs([]string{older, newer}, freshness, 2)
	if got[0] != newer {
		t.Fatalf("freshest-first within tier: got %v, want %s first", got, newer)
	}
}

// La pertinence CTI reste la clé primaire : une URL forte mais ancienne passe
// devant une URL fraîche sans signal CTI. La fraîcheur ne doit jamais inverser
// les tiers.
func TestPrioritizeURLsRelevanceDominatesFreshness(t *testing.T) {
	staleStrong := "http://example.onion/initial-access" // priorité 2, ancien
	freshWeak := "http://hash-only.onion"                 // priorité 0, frais
	freshness := map[string]float64{staleStrong: 1, freshWeak: 1_700_009_999}
	got := prioritizeURLs([]string{freshWeak, staleStrong}, freshness, 2)
	if got[0] != staleStrong {
		t.Fatalf("CTI relevance must dominate freshness: got %v", got)
	}
}

// Une URL sans horodatage de fraîcheur (absente de "urls_crawled_at") est
// traitée comme la plus ancienne mais jamais écartée : elle reste candidate,
// simplement après ses pairs horodatés du même tier.
func TestPrioritizeURLsMissingTimestampTreatedAsOldestNotDropped(t *testing.T) {
	withTS := "http://example.onion/ransomware-a"
	noTS := "http://example.onion/ransomware-b" // absent de la map -> 0
	freshness := map[string]float64{withTS: 1_700_000_000}
	got := prioritizeURLs([]string{noTS, withTS}, freshness, 2)
	if len(got) != 2 {
		t.Fatalf("missing-timestamp URL dropped: %v", got)
	}
	if got[0] != withTS {
		t.Fatalf("timestamped URL should precede missing one: %v", got)
	}
}
