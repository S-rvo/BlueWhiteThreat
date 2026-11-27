package db

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/S-rvo/BlueWhiteThreat/apps/parser/internal/entities"
)

func TestArticleKeyIsPlainHex(t *testing.T) {
	id, err := primitive.ObjectIDFromHex("65f0c0ffee0000000000abcd")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		in   any
		want string
	}{
		{id, "65f0c0ffee0000000000abcd"},
		{"legacy-id", "legacy-id"},
		{int32(7), "7"},
	}
	for _, c := range cases {
		if got := ArticleKey(c.in); got != c.want {
			t.Errorf("ArticleKey(%#v) = %q; want %q", c.in, got, c.want)
		}
	}
}

func TestExtractionBackoffDoublesAndCaps(t *testing.T) {
	cases := map[int]time.Duration{
		0:   10 * time.Minute,
		1:   10 * time.Minute,
		2:   20 * time.Minute,
		4:   80 * time.Minute,
		8:   21*time.Hour + 20*time.Minute,
		9:   24 * time.Hour,
		100: 24 * time.Hour,
	}
	for failures, want := range cases {
		if got := ExtractionBackoff(failures); got != want {
			t.Errorf("ExtractionBackoff(%d) = %s; want %s", failures, got, want)
		}
	}
}

func TestEntityLabelsAvoidCrawlerURL(t *testing.T) {
	for _, label := range entityLabels {
		if label == "URL" {
			t.Fatal("le label URL appartient au crawler")
		}
	}
	if len(entityGroups(emptyEntities())) != len(entityLabels) {
		t.Fatal("entityGroups et entityLabels divergent")
	}
	for label := range entityGroups(emptyEntities()) {
		found := false
		for _, l := range entityLabels {
			found = found || l == label
		}
		if !found {
			t.Errorf("label %q absent de entityLabels (pas de contrainte d'unicité)", label)
		}
	}
}

func emptyEntities() entities.Entities { return entities.Entities{} }
