// apps/classifier/internal/text/tokenize_test.go
package text

import (
	"reflect"
	"testing"
)

func TestTokenize(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{
			name: "lowercases and strips punctuation",
			in:   "LockBit 3.0 a attaqué l'entreprise!",
			want: []string{"lockbit", "attaqué", "entreprise"},
		},
		{
			name: "drops single-character tokens",
			in:   "a b cd",
			want: []string{"cd"},
		},
		{
			name: "empty string yields no tokens",
			in:   "",
			want: []string{},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Tokenize(c.in)
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("Tokenize(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}
