package crawler_test

import (
	"testing"

	crawler "github.com/S-rvo/BlueWhiteThreat/internal"
)

func TestHelloString(t *testing.T) {
	got := crawler.Hello()
	want := "Hello world"
	if got != want {
		t.Errorf("Hello() = %q; want %q", got, want)
	}
}
