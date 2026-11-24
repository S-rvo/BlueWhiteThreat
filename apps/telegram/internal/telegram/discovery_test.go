package telegram

import "testing"

func TestDiscoveryUsesShortestRootedPath(t *testing.T) {
	const origin = "https://t.me/"
	c := func(name string, p ...Provenance) Candidate {
		return Candidate{URL: origin + name, Kind: "public_candidate", Provenance: p}
	}
	from := func(name string) Provenance {
		return Provenance{Source: origin + name + "/3", Category: "message_link"}
	}
	all := []Candidate{
		c("grandchild", from("branch_one"), from("root_two")),
		c("branch_one", from("root_one"), from("grandchild")),
		c("root_one", Provenance{Source: Sources[0]}),
		c("root_two", Provenance{Category: "manual"}, from("orphan_one")),
		c("orphan_one", from("orphan_two")),
		c("orphan_two", from("orphan_one")),
		c("not_a_post", Provenance{Source: origin + "root_one", Category: "message_link"}),
	}
	b := NewDiscoveryBounds(all, 1, 1)
	for _, name := range []string{"root_one", "root_two", "grandchild", "branch_one"} {
		if !b.Allows(origin + name) {
			t.Errorf("%s was excluded", name)
		}
	}
	for _, name := range []string{"orphan_one", "orphan_two", "not_a_post"} {
		if b.Allows(origin + name) {
			t.Errorf("%s was included", name)
		}
	}

	first := b.Links(Article{Channel: origin + "root_one", URL: origin + "root_one/7", Links: []string{origin + "fresh_channel", origin + "overflow_channel"}})
	second := b.Links(Article{Channel: origin + "root_two", URL: origin + "root_two/9", Links: []string{origin + "fresh_channel", origin + "branch_one"}})
	if len(first) != 1 || len(second) != 2 || b.NewCandidates != 1 || b.SkippedBudget != 1 || b.Allows(origin+"fresh_channel") {
		t.Fatalf("first=%+v second=%+v count=%d skipped=%d", first, second, b.NewCandidates, b.SkippedBudget)
	}
	if got := b.Links(Article{Channel: origin + "branch_one", Links: []string{origin + "too_far"}}); len(got) != 0 || b.SkippedDepth != 1 {
		t.Fatalf("allowed deeper expansion: %+v", got)
	}
}
