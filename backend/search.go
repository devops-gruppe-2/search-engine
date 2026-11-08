package main

// Søgning med SQLite FTS5 (Full Text Search) i stedet for LIKE.
// pages_fts er et indeks over title og content i pages. Det gemmer ikke selv teksten
// (content='pages'), men slår op i pages via rowid. Triggers holder indekset opdateret,
// når pages ændres, også når crawleren skriver til databasen.

import (
	"context"
	"database/sql"
	"strings"
)

// Højst så mange resultater pr. søgning, de mest relevante først
const maxSearchResults = 50

// Søgningen i FTS-indekset. bm25 giver en relevans-score (lavere er bedre), og et ord
// i titlen tæller 10 gange så meget som et ord i teksten.
const searchQuery = `
SELECT p.title, p.url, p.language, COALESCE(p.last_updated, ''), p.content
FROM pages_fts
JOIN pages p ON p.rowid = pages_fts.rowid
WHERE pages_fts MATCH ? AND p.language = ?
ORDER BY bm25(pages_fts, 10.0, 1.0)
LIMIT ?`

// Statements der laver FTS-indekset. De ligger her og ikke i schema.sqlite, fordi
// triggerne indeholder semikolon, og schema.sqlite bliver delt op ved semikolon.
var searchIndexStatements = []string{
	// porter: "languages" finder også "language". unicode61: æ, ø, å og accenter virker.
	`CREATE VIRTUAL TABLE IF NOT EXISTS pages_fts USING fts5(
		title, content,
		content='pages', content_rowid='rowid',
		tokenize='porter unicode61'
	)`,
	`CREATE TRIGGER IF NOT EXISTS pages_fts_insert AFTER INSERT ON pages BEGIN
		INSERT INTO pages_fts(rowid, title, content) VALUES (new.rowid, new.title, new.content);
	END`,
	`CREATE TRIGGER IF NOT EXISTS pages_fts_delete AFTER DELETE ON pages BEGIN
		INSERT INTO pages_fts(pages_fts, rowid, title, content) VALUES ('delete', old.rowid, old.title, old.content);
	END`,
	`CREATE TRIGGER IF NOT EXISTS pages_fts_update AFTER UPDATE ON pages BEGIN
		INSERT INTO pages_fts(pages_fts, rowid, title, content) VALUES ('delete', old.rowid, old.title, old.content);
		INSERT INTO pages_fts(rowid, title, content) VALUES (new.rowid, new.title, new.content);
	END`,
	// Bygger indekset fra pages ved hver opstart. Det tager under et sekund med vores
	// antal sider, og det retter indekset, hvis rowid i pages har ændret sig (fx efter VACUUM).
	`INSERT INTO pages_fts(pages_fts) VALUES ('rebuild')`,
}

// migrateSearchIndex laver FTS-indekset og triggerne, hvis de mangler, og bygger indekset
func migrateSearchIndex(ctx context.Context, db *sql.DB) error {
	for _, stmt := range searchIndexStatements {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

// ftsQuery laver brugerens søgning om til en sikker FTS5-søgning. Hvert ord sættes i
// anførselstegn, så tegn som " - * ( ) og ord som AND/OR/NOT ikke giver en syntaksfejl.
// Ordene skal alle findes på siden, men ikke nødvendigvis ved siden af hinanden.
func ftsQuery(input string) string {
	var terms []string
	for _, word := range strings.Fields(input) {
		word = strings.ReplaceAll(word, `"`, `""`)
		terms = append(terms, `"`+word+`"`)
	}
	return strings.Join(terms, " ")
}

// searchPages finder de mest relevante sider på det valgte sprog
func searchPages(ctx context.Context, input, language string) ([]SearchResult, error) {
	results := []SearchResult{}

	match := ftsQuery(input)
	if match == "" {
		return results, nil
	}

	rows, err := db.QueryContext(ctx, searchQuery, match, language, maxSearchResults)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var result SearchResult
		if err := rows.Scan(&result.Title, &result.URL, &result.Language, &result.LastUpdated, &result.Content); err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, rows.Err()
}
