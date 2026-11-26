package main

// Tests af søgningen med PostgreSQL's fuldtekstsøgning og af importen fra SQLite

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

func addPage(t testing.TB, title, language, content string) {
	t.Helper()
	_, err := db.ExecContext(t.Context(), "INSERT INTO pages (title, url, language, content) VALUES ($1, $2, $3, $4)",
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

// Småord som "the" og "is" fjernes af ordbogen og skal ikke stå på siden
func TestSearchIgnoresStopWords(t *testing.T) {
	app := newTestApp(t)
	addPage(t, "Rust", "en", "Rust is a systems programming language.")

	if got := searchTitles(t, app, "what is the rust"); len(got) != 1 {
		t.Fatalf("search for 'what is the rust' = %v, want [Rust]", got)
	}
}

func TestSearchInDanishUsesTheDanishDictionary(t *testing.T) {
	app := newTestApp(t)
	addPage(t, "Programmering", "da", "Programmeringssprog bruges til at skrive programmer.")

	res := app.get("/api/search?language=da&q=" + url.QueryEscape("programmet"))
	expectStatus(t, res, http.StatusOK)
	if !strings.Contains(res.body, "Programmering") {
		t.Fatalf("danish search did not find the page: %s", res.body)
	}
}

// Tegn der betyder noget i søgesyntaks må ikke give fejl 500
func TestSearchWithSpecialCharactersDoesNotFail(t *testing.T) {
	app := newTestApp(t)
	addPage(t, "C++", "en", "C++ is a programming language.")

	for _, query := range []string{`"`, `c++`, `-go`, `(java`, `AND`, `NOT OR`, `prog*`, `title:x`, `a"b`, `'; DROP TABLE pages; --`, `!`, `&|`} {
		t.Run(query, func(t *testing.T) {
			searchTitles(t, app, query)
		})
	}
	if got := searchTitles(t, app, "programming"); len(got) != 1 {
		t.Fatalf("search for programming = %v, want [C++]", got)
	}
}

// search_vector udregnes af PostgreSQL, så søgningen følger med, når pages ændres
func TestSearchIndexFollowsChangesToPages(t *testing.T) {
	app := newTestApp(t)
	addPage(t, "Go", "en", "Go was made at Google.")

	if _, err := db.ExecContext(t.Context(), "UPDATE pages SET content = 'Go is fast and simple.' WHERE title = 'Go'"); err != nil {
		t.Fatal(err)
	}
	if got := searchTitles(t, app, "google"); len(got) != 0 {
		t.Fatalf("old text is still searchable: %v", got)
	}
	if got := searchTitles(t, app, "simple"); len(got) != 1 {
		t.Fatalf("new text is not searchable: %v", got)
	}

	if _, err := db.ExecContext(t.Context(), "DELETE FROM pages WHERE title = 'Go'"); err != nil {
		t.Fatal(err)
	}
	if got := searchTitles(t, app, "simple"); len(got) != 0 {
		t.Fatalf("deleted page is still searchable: %v", got)
	}
}

// Opgaven "Indexing": søgningen skal kunne bruge GIN-indexet i stedet for at læse alle sider
func TestSearchCanUseTheGINIndex(t *testing.T) {
	newTestApp(t)
	addPage(t, "Go", "en", "Go is a programming language.")

	conn, err := db.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	// Med få sider vælger PostgreSQL selv at læse dem alle. Det slås fra her for at se,
	// at indexet kan bruges.
	if _, err := conn.ExecContext(t.Context(), "SET enable_seqscan = off"); err != nil {
		t.Fatal(err)
	}
	rows, err := conn.QueryContext(t.Context(), "EXPLAIN "+searchQuery, "en", "programming", maxSearchResults)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var plan strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(line + "\n")
	}
	if !strings.Contains(plan.String(), "pages_search_vector_idx") {
		t.Fatalf("the search does not use the GIN index:\n%s", plan.String())
	}
}

// Første opstart på serveren: brugere og sider kopieres fra den gamle SQLite-database
func TestImportFromSQLite(t *testing.T) {
	newTestApp(t)

	path := filepath.Join(t.TempDir(), "app.db")
	old, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		"CREATE TABLE users (id INTEGER PRIMARY KEY AUTOINCREMENT, username TEXT NOT NULL UNIQUE, email TEXT NOT NULL UNIQUE, password TEXT NOT NULL, must_change_password INTEGER NOT NULL DEFAULT 0)",
		"CREATE TABLE pages (title TEXT PRIMARY KEY UNIQUE, url TEXT NOT NULL UNIQUE, language TEXT NOT NULL DEFAULT 'en', last_updated TIMESTAMP, content TEXT NOT NULL)",
		"INSERT INTO users (username, email, password) VALUES ('admin', 'admin@example.com', '$2a$10$serveradmin')",
		"INSERT INTO users (username, email, password, must_change_password) VALUES ('alice', 'alice@example.com', '$2a$10$def', 1)",
		"INSERT INTO pages (title, url, language, last_updated, content) VALUES ('Rust', 'https://example.com/rust', 'en', '2026-10-26T19:26:43Z', 'Rust is a systems programming language.')",
		"INSERT INTO pages (title, url, language, content) VALUES ('Go', 'https://example.com/go', 'en', 'Go is simple.')",
	} {
		if _, err := old.ExecContext(t.Context(), stmt); err != nil {
			t.Fatal(err)
		}
	}
	_ = old.Close()

	if err := importFromSQLite(t.Context(), db, path); err != nil {
		t.Fatalf("import: %v", err)
	}

	// Admin fra SQLite erstatter standard-admin (admin123) fra migreringen
	var adminPassword string
	var adminMustChange bool
	if err := db.QueryRowContext(t.Context(), "SELECT password, must_change_password FROM users WHERE username = 'admin'").Scan(&adminPassword, &adminMustChange); err != nil {
		t.Fatal(err)
	}
	if adminPassword != "$2a$10$serveradmin" || adminMustChange {
		t.Fatalf("admin = %q (must change %v), want the password from SQLite", adminPassword, adminMustChange)
	}

	var mustChange bool
	if err := db.QueryRowContext(t.Context(), "SELECT must_change_password FROM users WHERE username = 'alice'").Scan(&mustChange); err != nil {
		t.Fatalf("alice was not imported: %v", err)
	}
	if !mustChange {
		t.Fatal("alice must still change her password after the import")
	}
	results, err := searchPages(t.Context(), "systems", "en")
	if err != nil || len(results) != 1 || results[0].LastUpdated != "2026-10-26T19:26:43Z" {
		t.Fatalf("results = %v, err = %v, want Rust with its date", results, err)
	}

	// Anden opstart: der er sider i PostgreSQL, så intet kopieres igen
	if _, err := db.ExecContext(t.Context(), "DELETE FROM users WHERE username = 'alice'"); err != nil {
		t.Fatal(err)
	}
	if err := importFromSQLite(t.Context(), db, path); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM users WHERE username = 'alice'").Scan(&count); err != nil || count != 0 {
		t.Fatalf("the import ran twice (count = %d, err = %v)", count, err)
	}
}

// Ingen SQLite-fil: importen gør ingenting
func TestImportWithoutSQLiteFileDoesNothing(t *testing.T) {
	newTestApp(t)
	if err := importFromSQLite(t.Context(), db, filepath.Join(t.TempDir(), "missing.db")); err != nil {
		t.Fatal(err)
	}
}

// BenchmarkSearch sammenligner LIKE med fuldtekstsøgningen på 5.000 sider.
// Kør med: TEST_DATABASE_URL=... go test -bench Search -run '^$'
func BenchmarkSearch(b *testing.B) {
	openTestDB(b)

	words := []string{"computer", "language", "network", "history", "science", "music", "football", "river", "city", "planet"}
	for i := range 5000 {
		var content strings.Builder
		for j := range 300 {
			content.WriteString(words[(i*7+j*3)%len(words)] + fmt.Sprintf("%d ", j%50))
		}
		if i%100 == 0 {
			content.WriteString(" kubernetes")
		}
		addPage(b, fmt.Sprintf("Page %d", i), "en", content.String())
	}
	if _, err := db.ExecContext(b.Context(), "ANALYZE pages"); err != nil {
		b.Fatal(err)
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

	b.Run("tsvector+GIN", func(b *testing.B) {
		for b.Loop() {
			if _, err := searchPages(b.Context(), "kubernetes", "en"); err != nil {
				b.Fatal(err)
			}
		}
	})
}
