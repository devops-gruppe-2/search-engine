package main

// Tests af søgningen med FTS5

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"testing"
)

func addPage(t testing.TB, title, language, content string) {
	t.Helper()
	_, err := db.ExecContext(t.Context(), "INSERT INTO pages (title, url, language, content) VALUES (?, ?, ?, ?)",
		title, "https://example.com/"+url.PathEscape(title), language, content)
	if err != nil {
		t.Fatal(err)
	}
}

// searchTitles søger via API'et og giver titlerne i den rækkefølge, de kom
func searchTitles(t *testing.T, app *testApp, query string) []string {
	t.Helper()
	res := app.get("/api/search?q=" + url.QueryEscape(query))
	expectStatus(t, res, http.StatusOK)

	var body APIUserSearchResponse
	if err := json.Unmarshal([]byte(res.body), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	titles := []string{}
	for _, r := range body.SearchResults {
		titles = append(titles, r.Title)
	}
	return titles
}

func TestSearchRanksTitleMatchesFirst(t *testing.T) {
	app := newTestApp(t)
	addPage(t, "History of computing", "en", "Many people used Python and other tools.")
	addPage(t, "Python", "en", "A programming language.")

	got := searchTitles(t, app, "python")
	if len(got) != 2 || got[0] != "Python" {
		t.Fatalf("search for python = %v, want the page with Python in the title first", got)
	}
}

func TestSearchFindsOtherFormsOfAWord(t *testing.T) {
	app := newTestApp(t)
	addPage(t, "Go", "en", "Go is a programming language.")

	if got := searchTitles(t, app, "languages"); len(got) != 1 {
		t.Fatalf("search for languages = %v, want the page that says language", got)
	}
}

func TestSearchNeedsAllWords(t *testing.T) {
	app := newTestApp(t)
	addPage(t, "Go", "en", "Go is a programming language from Google.")
	addPage(t, "Java", "en", "Java is a programming language from Sun.")

	if got := searchTitles(t, app, "programming google"); len(got) != 1 || got[0] != "Go" {
		t.Fatalf("search for programming google = %v, want [Go]", got)
	}
}

// Tegn der betyder noget i FTS5-syntaksen må ikke give fejl 500
func TestSearchWithSpecialCharactersDoesNotFail(t *testing.T) {
	app := newTestApp(t)
	addPage(t, "C++", "en", "C++ is a programming language.")

	for _, query := range []string{`"`, `c++`, `-go`, `(java`, `AND`, `NOT OR`, `prog*`, `title:x`, `a"b`, `'; DROP TABLE pages; --`} {
		t.Run(query, func(t *testing.T) {
			searchTitles(t, app, query)
		})
	}
	if got := searchTitles(t, app, "c++"); len(got) != 1 {
		t.Fatalf("search for c++ = %v, want [C++]", got)
	}
}

// Triggerne holder indekset opdateret, når pages ændres (fx af crawleren)
func TestSearchIndexFollowsChangesToPages(t *testing.T) {
	app := newTestApp(t)
	addPage(t, "Go", "en", "Go was made at Google.")

	if _, err := db.ExecContext(t.Context(), "UPDATE pages SET content = 'Go is fast and simple.' WHERE title = 'Go'"); err != nil {
		t.Fatal(err)
	}
	if got := searchTitles(t, app, "google"); len(got) != 0 {
		t.Fatalf("old text is still in the index: %v", got)
	}
	if got := searchTitles(t, app, "simple"); len(got) != 1 {
		t.Fatalf("new text is not in the index: %v", got)
	}

	if _, err := db.ExecContext(t.Context(), "DELETE FROM pages WHERE title = 'Go'"); err != nil {
		t.Fatal(err)
	}
	if got := searchTitles(t, app, "simple"); len(got) != 0 {
		t.Fatalf("deleted page is still in the index: %v", got)
	}
}

// En database fra før FTS5 (som den på serveren) får et indeks med de sider, der allerede findes
func TestExistingPagesAreIndexedAtStartup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	old, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		"CREATE TABLE pages (title TEXT PRIMARY KEY UNIQUE, url TEXT NOT NULL UNIQUE, language TEXT NOT NULL DEFAULT 'en', last_updated TIMESTAMP, content TEXT NOT NULL)",
		"INSERT INTO pages (title, url, language, content) VALUES ('Rust', 'https://example.com/rust', 'en', 'Rust is a systems programming language.')",
	} {
		if _, err := old.ExecContext(t.Context(), stmt); err != nil {
			t.Fatal(err)
		}
	}
	_ = old.Close()

	db, err = initDB(path)
	if err != nil {
		t.Fatalf("initDB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	results, err := searchPages(t.Context(), "systems", "en")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Title != "Rust" {
		t.Fatalf("results = %v, want [Rust]", results)
	}
}

func TestFTSQuery(t *testing.T) {
	tests := map[string]string{
		"python":            `"python"`,
		"  go   language  ": `"go" "language"`,
		`say "hi"`:          `"say" """hi"""`,
		"":                  "",
	}
	for input, want := range tests {
		if got := ftsQuery(input); got != want {
			t.Errorf("ftsQuery(%q) = %q, want %q", input, got, want)
		}
	}
}

// BenchmarkSearch sammenligner den gamle LIKE-søgning med FTS5 på 5.000 sider.
// Kør med: go test -bench Search -run '^$'
func BenchmarkSearch(b *testing.B) {
	var err error
	db, err = initDB(filepath.Join(b.TempDir(), "bench.db"))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = db.Close() })

	words := []string{"computer", "language", "network", "history", "science", "music", "football", "river", "city", "planet"}
	for i := range 5000 {
		content := ""
		for j := range 300 {
			content += words[(i*7+j*3)%len(words)] + fmt.Sprintf("%d ", j%50)
		}
		if i%100 == 0 {
			content += " kubernetes"
		}
		addPage(b, fmt.Sprintf("Page %d", i), "en", content)
	}

	b.Run("LIKE", func(b *testing.B) {
		for b.Loop() {
			rows, err := db.QueryContext(b.Context(), "SELECT title FROM pages WHERE language = 'en' AND content LIKE '%kubernetes%' LIMIT 50")
			if err != nil {
				b.Fatal(err)
			}
			for rows.Next() {
			}
			_ = rows.Close()
		}
	})

	b.Run("FTS5", func(b *testing.B) {
		for b.Loop() {
			if _, err := searchPages(b.Context(), "kubernetes", "en"); err != nil {
				b.Fatal(err)
			}
		}
	})
}
