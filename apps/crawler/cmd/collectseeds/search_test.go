package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSearchPaginationBudgetAndURL(t *testing.T) {
	calls := 0
	host := strings.Repeat("b", 56) + ".onion"
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("X-API-Key") != "secret" {
			t.Error("missing credential")
		}
		if r.URL.Query().Get("page") != fmt.Sprint(calls) {
			t.Error("wrong page")
		}
		fmt.Fprintf(w, `{"status":"success","pagination":{"total_pages":100},"results":[{"url":"https://%s/Forum?page=%d"}]}`, host, calls)
	}))
	defer s.Close()
	found := map[string]bool{}
	err := collectSearch(context.Background(), &http.Client{Transport: rewriteHost{s.URL}}, "secret", []string{"forum"}, 2, 0, found)
	if err != nil || calls != 2 || len(found) != 2 {
		t.Fatalf("calls=%d results=%v error=%v", calls, found, err)
	}
	if !found["https://"+host+"/Forum?page=2"] {
		t.Fatal(found)
	}
}
func TestSearchStopsOnErrorAndRedirect(t *testing.T) {
	for _, code := range []int{302, 401, 429, 500} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			calls := 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Location", "/redirect")
				w.WriteHeader(code)
			}))
			defer s.Close()
			err := collectSearch(context.Background(), &http.Client{Transport: rewriteHost{s.URL}}, "secret", []string{"forum", "leak"}, 24, 0, map[string]bool{})
			if err == nil || calls != 1 {
				t.Fatalf("calls=%d error=%v", calls, err)
			}
		})
	}
}
func TestSearchWithoutKeyDoesNotConnect(t *testing.T) {
	if err := collectSearch(context.Background(), nil, "", nil, 24, 0, nil); err != nil {
		t.Fatal(err)
	}
}
func TestHiddenSearchFields(t *testing.T) {
	got := hiddenSearchFields([]byte(`<input value='a&amp;b' name='token' type='hidden'><input name='q' type='hidden' value='wrong'>`))
	if got != "token=a%26b" {
		t.Fatal(got)
	}
}

func TestSearchQueryFairness(t *testing.T) {
	var queries []string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.Query().Get("q"))
		fmt.Fprint(w, `{"status":"success","pagination":{"total_pages":100},"results":[{"url":"ignored"}]}`)
	}))
	defer s.Close()
	err := collectSearch(context.Background(), &http.Client{Transport: rewriteHost{s.URL}}, "key", []string{"one", "two"}, 3, 0, map[string]bool{})
	if err != nil || strings.Join(queries, ",") != "one,two,one" {
		t.Fatalf("%v %v", queries, err)
	}
}
