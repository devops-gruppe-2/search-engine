package main

// Søgning med PostgreSQL's fuldtekstsøgning. Kolonnen search_vector og GIN-indexet laves
// i migrations/00002_search_index.sql, og PostgreSQL holder dem selv opdateret, når
// pages ændres, også når crawleren skriver til databasen.

import (
	"context"
	"strings"
)

// Højst så mange resultater pr. søgning, de mest relevante først
const maxSearchResults = 50

// plainto_tsquery laver brugerens tekst om til en søgning: alle ord skal findes på siden,
// stemming bruges ("languages" finder "language"), og småord som "the" fjernes.
// Tegn som " - * ( ) og ord som AND/OR giver ikke fejl, fordi teksten ikke læses som syntaks.
//
// ts_rank giver en relevans-score. Vægtene er {D, C, B, A}: ord i titlen (A) tæller
// 10 gange så meget som ord i teksten (B), som med bm25(10, 1) i FTS5 før.
//
// Sproget vælger ordbogen, så danske sider søges med den danske.
const searchQuery = `
WITH q AS (
    SELECT plainto_tsquery(CASE WHEN $1 = 'da' THEN 'danish'::regconfig ELSE 'english'::regconfig END, $2) AS query
)
SELECT p.title, p.url, p.language, COALESCE(to_char(p.last_updated AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'), ''), p.content
FROM pages p, q
WHERE p.language = $1 AND p.search_vector @@ q.query
ORDER BY ts_rank('{0.1, 0.2, 0.1, 1.0}', p.search_vector, q.query) DESC, p.title
LIMIT $3`

// searchPages finder de mest relevante sider på det valgte sprog
func searchPages(ctx context.Context, input, language string) ([]SearchResult, error) {
	results := []SearchResult{}

	input = strings.TrimSpace(input)
	if input == "" {
		return results, nil
	}

	rows, err := db.QueryContext(ctx, searchQuery, language, input, maxSearchResults)
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
