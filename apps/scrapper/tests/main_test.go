package scrapper_test

import (
	"testing"

	scrapper "github.com/S-rvo/BlueWhiteThreat/internal"
)

func TestHelloString(t *testing.T) {
	got := scrapper.Hello()
	want := "Hello world"
	if got != want {
		t.Errorf("Hello() = %q; want %q", got, want)
	}
}
