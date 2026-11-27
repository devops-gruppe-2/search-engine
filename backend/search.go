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

// Søgningen finder sider med mindst ét af ordene og viser de bedste først.
//
// Før krævede den, at alle ord stod på siden. Søge-loggen og Grafana viste, at mange søgninger
// som "best programming language for quantum computing" derfor gav 0 resultater, selv om
// vi havde sider om quantum computing.
//
//   - all_words: plainto_tsquery laver teksten om til ord, der alle skal findes ("a & b").
//     Stemming bruges ("languages" finder "language"), og småord som "the" fjernes.
//     Tegn som " - * ( ) og ord som AND/OR giver ikke fejl, fordi teksten ikke læses som syntaks.
//   - any_word: de samme ord, men hvor ét er nok ("a | b").
//
// Rækkefølgen: sider med alle ordene først, derefter efter ts_rank. Vægtene er {D, C, B, A}:
// ord i titlen (A) tæller 10 gange så meget som ord i teksten (B). Sider med flere af ordene
// får en højere score. Sproget vælger ordbogen, så danske sider søges med den danske.
const searchQuery = `
WITH cfg AS (
    SELECT CASE WHEN $1 = 'da' THEN 'danish'::regconfig ELSE 'english'::regconfig END AS c
), q AS (
    SELECT plainto_tsquery(cfg.c, $2) AS all_words,
           to_tsquery(cfg.c, replace(plainto_tsquery(cfg.c, $2)::text, ' & ', ' | ')) AS any_word
    FROM cfg
)
SELECT p.title, p.url, p.language, COALESCE(to_char(p.last_updated AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'), ''), p.content
FROM pages p, q
WHERE p.language = $1 AND p.search_vector @@ q.any_word
ORDER BY p.search_vector @@ q.all_words DESC,
         ts_rank('{0.1, 0.2, 0.1, 1.0}', p.search_vector, q.any_word) DESC,
         p.title
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
