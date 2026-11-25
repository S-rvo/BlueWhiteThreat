package db

import "testing"

func TestCleanPageURL(t *testing.T) {
	for input, want := range map[string]string{
		" HTTP://EXAMPLE.onion/Forum/123?x=Y#section ": "http://example.onion/Forum/123?x=Y",
		"http://example.onion/":                        "http://example.onion",
	} {
		if got := cleanOnionURL(input); got != want {
			t.Errorf("got %q want %q", got, want)
		}
	}
}
