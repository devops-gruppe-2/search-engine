package main

// Tests af logningen af søgninger

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureSearchLog sender søge-loggen til en buffer, så testen kan læse linjerne
func captureSearchLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	old := searchLog
	searchLog = slog.New(slog.NewJSONHandler(&buf, nil))
	t.Cleanup(func() { searchLog = old })
	return &buf
}

func logLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var lines []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("log line is not JSON: %q", line)
		}
		lines = append(lines, m)
	}
	return lines
}

func TestSearchesAreLogged(t *testing.T) {
	app := newTestApp(t)
	buf := captureSearchLog(t)
	addPage(t, "Python", "en", "Python is a programming language.")

	app.get("/api/search?q=" + "%20Python%20%20Language")
	app.do(http.MethodGet, "/?q=football", nil, true)

	lines := logLines(t, buf)
	if len(lines) != 2 {
		t.Fatalf("got %d log lines, want 2: %s", len(lines), buf.String())
	}

	api, web := lines[0], lines[1]
	expect := func(line map[string]any, key string, want any) {
		t.Helper()
		if line[key] != want {
			t.Errorf("%s = %v, want %v (line %v)", key, line[key], want, line)
		}
	}
	expect(api, "msg", "search")
	expect(api, "query", "python language")
	expect(api, "language", "en")
	expect(api, "results", float64(1))
	expect(api, "source", "api")
	expect(api, "logged_in", false)

	expect(web, "query", "football")
	expect(web, "results", float64(0))
	expect(web, "source", "web")
}

// Brugernavn og IP-adresse må ikke komme i loggen
func TestSearchLogHasNoPersonalData(t *testing.T) {
	app := newTestApp(t)
	buf := captureSearchLog(t)
	app.register("alice", "password-1")

	app.get("/api/search?q=go")

	lines := logLines(t, buf)
	if len(lines) != 1 || lines[0]["logged_in"] != true {
		t.Fatalf("lines = %v, want one search by a logged in user", lines)
	}
	for _, private := range []string{"alice", "127.0.0.1"} {
		if strings.Contains(buf.String(), private) {
			t.Fatalf("log contains %q: %s", private, buf.String())
		}
	}
}

// Tomme søgninger og forsiden uden søgning logges ikke
func TestEmptySearchesAreNotLogged(t *testing.T) {
	app := newTestApp(t)
	buf := captureSearchLog(t)

	app.get("/api/search?q=")
	app.do(http.MethodGet, "/", nil, true)

	if buf.Len() != 0 {
		t.Fatalf("expected no log lines, got: %s", buf.String())
	}
}

func TestNormalizeQuery(t *testing.T) {
	tests := map[string]string{
		"  Python   Tutorial ":   "python tutorial",
		"ÆBLE":                   "æble",
		strings.Repeat("ø", 300): strings.Repeat("ø", maxLoggedQueryLength),
	}
	for input, want := range tests {
		if got := normalizeQuery(input); got != want {
			t.Errorf("normalizeQuery(%q) = %q, want %q", input, got, want)
		}
	}
}

// Logfilen og mappen laves, og nye linjer kommer i bunden
func TestOpenSearchLogAppendsToFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "searches.log")
	for range 2 {
		logger, closer, err := openSearchLog(path)
		if err != nil {
			t.Fatal(err)
		}
		logger.Info("search", "query", "go")
		_ = closer.Close()
	}
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(data), `"query":"go"`); n != 2 {
		t.Fatalf("file has %d lines, want 2: %s", n, data)
	}
}
