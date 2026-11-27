package main

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestMissingQueriesFromTheSearchLog(t *testing.T) {
	log := strings.Join([]string{
		`{"msg":"search","query":"quantum computing","language":"en","results":0}`,
		`{"msg":"search","query":"quantum computing","language":"en","results":0}`,
		`{"msg":"search","query":"cryptography","language":"en","results":0}`,
		`{"msg":"search","query":"python","language":"en","results":12}`,
		`{"msg":"search","query":"programmering","language":"da","results":0}`,
		`not json`,
	}, "\n")

	got, err := missingQueries(strings.NewReader(log), "en", 10)
	if err != nil {
		t.Fatal(err)
	}
	want := []missingQuery{{"quantum computing", 2}, {"cryptography", 1}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("missingQueries = %v, want %v", got, want)
	}

	if got, _ := missingQueries(strings.NewReader(log), "en", 1); len(got) != 1 || got[0].Query != "quantum computing" {
		t.Fatalf("top 1 = %v, want only quantum computing", got)
	}
}

// En falsk Wikipedia, så testen ikke kalder den rigtige
func TestSearchWikipedia(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("list") != "search" || r.URL.Query().Get("srsearch") != "quantum computing" {
			t.Errorf("unexpected request: %s", r.URL)
		}
		if r.Header.Get("User-Agent") != userAgent {
			t.Errorf("missing User-Agent")
		}
		_, _ = w.Write([]byte(`{"query":{"search":[{"title":"Quantum computing"},{"title":"Qubit"}]}}`))
	}))
	defer server.Close()

	got, err := searchWikipedia(t.Context(), server.Client(), server.URL, "quantum computing", 2)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"Quantum computing", "Qubit"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("searchWikipedia = %v, want %v", got, want)
	}
}
