package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSourceURLs(t *testing.T) {
	files := []string{"forum.md", "markets.md"}
	sources := sourceURLs(files)
	want := len(files) + len(extraSources) + len(ctiSearchKeywords)
	if len(sources) != want {
		t.Fatalf("len(sources) = %d, want %d", len(sources), want)
	}
	if sources[0] != rawBaseURL+"forum.md" {
		t.Errorf("premier source = %q, attendu le fichier deepdarkCTI", sources[0])
	}
	var ahmia int
	for _, s := range sources {
		if strings.HasPrefix(s, ahmiaSearchBase) {
			ahmia++
		}
	}
	if ahmia != len(ctiSearchKeywords) {
		t.Errorf("requêtes Ahmia = %d, want %d", ahmia, len(ctiSearchKeywords))
	}
}

func TestFetchOnionAddressesBodyAndHeader(t *testing.T) {
	bodyOnion := strings.Repeat("a", 56) + ".onion"
	headerOnion := strings.Repeat("b", 56) + ".onion"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Onion-Location", "http://"+headerOnion+"/")
		w.Write([]byte("directory listing: visit " + bodyOnion + " for more"))
	}))
	defer server.Close()

	found := map[string]bool{}
	n, err := fetchOnionAddresses(server.Client(), server.URL, found)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("n = %d, want 2", n)
	}
	if !found[bodyOnion] {
		t.Errorf("adresse du corps non extraite: %v", found)
	}
	if !found[headerOnion] {
		t.Errorf("adresse Onion-Location non extraite: %v", found)
	}
}
