package scrapper_test

import (
	"testing"

	scraper "github.com/S-rvo/BlueWhiteThreat/internal/scraper"
)

func TestHelloString(t *testing.T) {
	got := scraper.Hello()
	want := "Hello world"
	if got != want {
		t.Errorf("Hello() = %q; want %q", got, want)
	}
}
