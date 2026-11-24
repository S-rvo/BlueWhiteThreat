package telegram

import (
	"net/url"
	"strings"
)

// DiscoveryBounds is shared by both queues for one cycle. The graph is rebuilt
// from existing provenance, so old registries need no schema migration.
type DiscoveryBounds struct {
	depths                                     map[string]int
	known                                      map[string]bool
	maxDepth, maxNew                           int
	NewCandidates, SkippedDepth, SkippedBudget int
}

func NewDiscoveryBounds(candidates []Candidate, maxDepth, maxNew int) *DiscoveryBounds {
	b := &DiscoveryBounds{depths: map[string]int{}, known: map[string]bool{}, maxDepth: maxDepth, maxNew: maxNew}
	children := map[string][]string{}
	queue := []string{}
	for _, c := range candidates {
		b.known[c.URL] = true
		if c.Kind != "public_candidate" {
			continue
		}
		root := len(c.Provenance) == 0 // Existing local seeds entered directly.
		for _, p := range c.Provenance {
			if p.Category == "manual" {
				root = true
			}
			if p.Category == "message_link" {
				if parent := sourceChannel(p.Source); parent != "" {
					children[parent] = append(children[parent], c.URL)
				}
			}
			for _, source := range Sources {
				if p.Source == source {
					root = true
				}
			}
		}
		if root {
			if _, seen := b.depths[c.URL]; !seen {
				b.depths[c.URL] = 0
				queue = append(queue, c.URL)
			}
		}
	}
	// Breadth-first from all roots finds shortest paths even with cycles.
	for i := 0; i < len(queue); i++ {
		parent := queue[i]
		if b.depths[parent] >= maxDepth {
			continue
		}
		for _, child := range children[parent] {
			if _, seen := b.depths[child]; !seen {
				b.depths[child] = b.depths[parent] + 1
				queue = append(queue, child)
			}
		}
	}
	return b
}

func sourceChannel(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	path := strings.Trim(strings.TrimPrefix(u.Path, "/s/"), "/")
	if strings.Count(path, "/") != 1 {
		return "" // Only references to actual messages establish a parent.
	}
	channel, kind, err := Normalize(raw)
	if err != nil || kind != "public_candidate" {
		return ""
	}
	return channel
}

func (b *DiscoveryBounds) Allows(channel string) bool {
	_, ok := b.depths[channel]
	return ok
}

// Links collects references without changing the article or the queue snapshot.
// The budget is for unique new URLs; existing candidates retain new provenance.
func (b *DiscoveryBounds) Links(a Article) []Candidate {
	depth, eligible := b.depths[a.Channel]
	var found []Candidate
	for _, address := range a.Links {
		normalized, kind, err := Normalize(address)
		if err != nil || normalized == a.Channel {
			continue
		}
		if !eligible || depth >= b.maxDepth {
			b.SkippedDepth++
			continue
		}
		if !b.known[normalized] {
			if b.NewCandidates >= b.maxNew {
				b.SkippedBudget++
				continue
			}
			b.NewCandidates++
			b.known[normalized] = true
		}
		state := "pending"
		if kind == "invite" {
			state = "invitation_required"
		}
		found = append(found, Candidate{URL: normalized, Kind: kind, State: state, Provenance: []Provenance{{Source: a.URL, Category: "message_link"}}})
	}
	return found
}
