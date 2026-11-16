package main

// Logning af søgninger (opgaven "Let them search").
// Hver søgning skrives som én linje JSON i en fil med Go's indbyggede log/slog. Så kan vi se,
// hvad brugerne søger efter, og især hvad de ikke finder, og bruge det til at vælge, hvad
// crawleren skal hente. Brugernavn og IP-adresse logges ikke.
//
// Se en opsummering med: go run ./cmd/searchstats -file logs/searches.log

import (
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// På serveren er mappen logs/ et volume, så loggen bliver bevaret, når containeren skiftes ud ved deploy
const searchLogPath = "logs/searches.log"

// Meget lange søgninger forkortes, så én bruger ikke kan fylde loggen op
const maxLoggedQueryLength = 200

// searchLog skriver ingen steder, indtil main åbner filen. Tests kan skifte den ud.
var searchLog = slog.New(slog.NewJSONHandler(io.Discard, nil))

// openSearchLog åbner (eller laver) logfilen og tilføjer nye linjer i bunden
func openSearchLog(path string) (*slog.Logger, io.Closer, error) {
	path = filepath.Clean(path)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, nil, err
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, nil, err
	}
	return slog.New(slog.NewJSONHandler(file, nil)), file, nil
}

// startSearchLog bruges af main. Kan filen ikke åbnes, kører appen videre uden søge-log.
func startSearchLog() io.Closer {
	logger, closer, err := openSearchLog(searchLogPath)
	if err != nil {
		log.Printf("Search log is disabled: %v", err)
		return io.NopCloser(nil)
	}
	searchLog = logger
	log.Printf("Logging searches to %s", searchLogPath)
	return closer
}

// normalizeQuery gør "  Python  Tutorial" og "python tutorial" til den samme søgning i loggen
func normalizeQuery(query string) string {
	normalized := strings.ToLower(strings.Join(strings.Fields(query), " "))
	if runes := []rune(normalized); len(runes) > maxLoggedQueryLength {
		normalized = string(runes[:maxLoggedQueryLength])
	}
	return normalized
}

// logSearch skriver én søgning til loggen. source er "web" (søgesiden) eller "api".
func logSearch(c *gin.Context, source, query, language string, results int, took time.Duration) {
	searchLog.LogAttrs(c.Request.Context(), slog.LevelInfo, "search",
		slog.String("query", normalizeQuery(query)),
		slog.String("language", language),
		slog.Int("results", results),
		slog.String("source", source),
		slog.Bool("logged_in", isLoggedIn(c)),
		slog.Int64("duration_ms", took.Milliseconds()),
	)
}
