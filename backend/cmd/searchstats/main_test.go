package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestSummarize(t *testing.T) {
	log := strings.Join([]string{
		`{"time":"2026-11-16T10:00:00Z","level":"INFO","msg":"search","query":"python","language":"en","results":3}`,
		`{"time":"2026-11-16T11:00:00Z","level":"INFO","msg":"search","query":"python","language":"en","results":3}`,
		`{"time":"2026-11-16T12:00:00Z","level":"INFO","msg":"search","query":"football","language":"en","results":0}`,
		`{"time":"2026-11-17T09:00:00Z","level":"INFO","msg":"search","query":"football","language":"en","results":0}`,
		`{"time":"2026-11-17T09:30:00Z","level":"INFO","msg":"search","query":"pizza","language":"en","results":0}`,
		`not json`,
		`{"time":"2026-11-17T09:31:00Z","level":"INFO","msg":"something else"}`,
	}, "\n")

	got, err := summarize(strings.NewReader(log), 10)
	if err != nil {
		t.Fatal(err)
	}
	want := stats{
		Total:       5,
		NoResults:   3,
		Days:        2,
		TopSearches: []count{{"football", 2}, {"python", 2}, {"pizza", 1}},
		TopMissing:  []count{{"football", 2}, {"pizza", 1}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("summarize =\n%+v\nwant\n%+v", got, want)
	}
}

func TestSummarizeShowsOnlyTop(t *testing.T) {
	log := `{"msg":"search","query":"a","results":1}
{"msg":"search","query":"b","results":1}
{"msg":"search","query":"b","results":1}`

	got, err := summarize(strings.NewReader(log), 1)
	if err != nil {
		t.Fatal(err)
	}
	if want := []count{{"b", 2}}; !reflect.DeepEqual(got.TopSearches, want) {
		t.Fatalf("TopSearches = %v, want %v", got.TopSearches, want)
	}
}
