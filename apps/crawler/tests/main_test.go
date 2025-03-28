package crawler_test

import (
	"testing"

	crawler "github.com/S-rvo/BlueWhiteThreat/internal"
)

func TestStatus200Ok(t *testing.T) {
	status, err := crawler.TorClient()
	if err != nil {
		t.Fatalf("TorClient() returned error: %v", err)
	}
	want := "200 OK"
	if status != want {
		t.Errorf("TorClient() = %q; want %q", status, want)
	}
}
