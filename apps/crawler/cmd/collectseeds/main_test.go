package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestSourceURLs(t *testing.T) {
	files := []string{"forum.md", "markets.md"}
	sources := sourceURLs(files)
	want := len(files) + len(extraSources)
	if len(sources) != want {
		t.Fatalf("len(sources) = %d, want %d", len(sources), want)
	}
	if sources[0] != rawBaseURL+"forum.md" {
		t.Errorf("premier source = %q, attendu le fichier deepdarkCTI", sources[0])
	}
	// Les URLs Ahmia sont dynamiques (jeton), jamais dans la part statique.
	for _, s := range sources {
		if strings.HasPrefix(s, ahmiaSearchBase) {
			t.Errorf("URL Ahmia ne doit pas être dans sourceURLs: %q", s)
		}
	}
}

func TestAhmiaSearchURLsInjectsToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<form action="/search/" method="get">
			<input id="id_q" type="search" name="q">
			<input type="hidden" name="6c799e" value="38c084">
		</form>`)
	}))
	defer server.Close()
	// Rediriger ahmiaHomeURL vers le serveur de test n'est pas possible (const),
	// donc on teste via un client dont le Transport réécrit l'hôte.
	client := &http.Client{Transport: rewriteHost{target: server.URL}}
	urls := ahmiaSearchURLs(client)
	if len(urls) != len(ctiSearchKeywords) {
		t.Fatalf("len(urls) = %d, want %d", len(urls), len(ctiSearchKeywords))
	}
	for _, u := range urls {
		if !strings.Contains(u, "6c799e=38c084") {
			t.Errorf("jeton manquant dans %q", u)
		}
		if !strings.HasPrefix(u, ahmiaSearchBase) {
			t.Errorf("URL de recherche inattendue: %q", u)
		}
	}
}

// rewriteHost réécrit l'hôte de chaque requête vers le serveur de test, pour
// exercer ahmiaSearchURLs sans toucher au vrai ahmia.fi.
type rewriteHost struct{ target string }

func (rw rewriteHost) RoundTrip(req *http.Request) (*http.Response, error) {
	tgt, err := url.Parse(rw.target)
	if err != nil {
		return nil, err
	}
	req.URL.Scheme = tgt.Scheme
	req.URL.Host = tgt.Host
	return http.DefaultTransport.RoundTrip(req)
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
