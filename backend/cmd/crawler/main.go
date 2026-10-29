package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//==============================================================================================================================================================================
// CRAWLER
//
// Henter Wikipedia-artikler fra en eller flere kategorier og gemmer dem i pages-tabellen,
// så søgningen har noget indhold at finde.
//
// Kør fra backend-mappen:
//   go run ./cmd/crawler -db ./app.db -limit 200
//==============================================================================================================================================================================

// Wikipedia beder om en User-Agent, der fortæller hvem der crawler
const userAgent = "WhoKnowsCrawler/1.0 (https://github.com/devops-gruppe-2/search-engine)"

type categoryResponse struct {
	Continue struct {
		CmContinue string `json:"cmcontinue"`
	} `json:"continue"`
	Query struct {
		CategoryMembers []struct {
			Title string `json:"title"`
		} `json:"categorymembers"`
	} `json:"query"`
}

type extractResponse struct {
	Query struct {
		Pages []struct {
			Title    string `json:"title"`
			Extract  string `json:"extract"`
			FullURL  string `json:"fullurl"`
			Missing  bool   `json:"missing"`
			Touched  string `json:"touched"`
			Language string `json:"pagelanguage"`
		} `json:"pages"`
	} `json:"query"`
}

type page struct {
	Title       string
	URL         string
	Language    string
	LastUpdated string
	Content     string
}

func main() {
	dbPath := flag.String("db", "./app.db", "sti til SQLite-databasen")
	lang := flag.String("lang", "en", "Wikipedia-sprog (en eller da)")
	categories := flag.String("categories", "Category:Programming languages,Category:Computer programming", "kategorier adskilt af komma")
	limit := flag.Int("limit", 200, "maks antal artikler der hentes")
	delay := flag.Duration("delay", 200*time.Millisecond, "pause mellem kald, så vi ikke belaster Wikipedia")
	flag.Parse()

	if *lang != "en" && *lang != "da" {
		log.Fatalf("lang skal være en eller da, fik %q", *lang)
	}

	db, err := sql.Open("sqlite", *dbPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		log.Fatalf("Kunne ikke åbne databasen: %v", err)
	}
	defer db.Close()

	if err := ensurePagesTable(db); err != nil {
		log.Fatalf("Kunne ikke oprette pages-tabellen: %v", err)
	}

	ctx := context.Background()
	client := &http.Client{Timeout: 30 * time.Second}
	apiURL := fmt.Sprintf("https://%s.wikipedia.org/w/api.php", *lang)

	// Find artikeltitler i kategorierne
	titles := []string{}
	seen := map[string]bool{}
	for _, category := range strings.Split(*categories, ",") {
		category = strings.TrimSpace(category)
		if category == "" {
			continue
		}
		found, err := categoryMembers(ctx, client, apiURL, category, *limit-len(titles), *delay)
		if err != nil {
			log.Printf("Kunne ikke hente kategorien %q: %v", category, err)
			continue
		}
		for _, title := range found {
			if !seen[title] {
				seen[title] = true
				titles = append(titles, title)
			}
		}
		if len(titles) >= *limit {
			break
		}
	}
	log.Printf("Fandt %d artikler", len(titles))

	// Hent indholdet af hver artikel og gem det
	saved := 0
	for i, title := range titles {
		p, err := fetchPage(ctx, client, apiURL, title, *lang)
		if err != nil {
			log.Printf("[%d/%d] Sprang over %q: %v", i+1, len(titles), title, err)
			continue
		}
		if err := savePage(db, p); err != nil {
			log.Printf("[%d/%d] Kunne ikke gemme %q: %v", i+1, len(titles), title, err)
			continue
		}
		saved++
		log.Printf("[%d/%d] Gemt %q", i+1, len(titles), title)
		time.Sleep(*delay)
	}

	log.Printf("Færdig: %d af %d artikler gemt i %s", saved, len(titles), *dbPath)
}

//==============================================================================================================================================================================
// WIKIPEDIA
//==============================================================================================================================================================================

// categoryMembers returnerer titlerne på artiklerne (ikke underkategorier) i en kategori
func categoryMembers(ctx context.Context, client *http.Client, apiURL, category string, limit int, delay time.Duration) ([]string, error) {
	titles := []string{}
	cmContinue := ""

	for len(titles) < limit {
		params := url.Values{
			"action":      {"query"},
			"format":      {"json"},
			"list":        {"categorymembers"},
			"cmtitle":     {category},
			"cmnamespace": {"0"},
			"cmlimit":     {"100"},
		}
		if cmContinue != "" {
			params.Set("cmcontinue", cmContinue)
		}

		var resp categoryResponse
		if err := getJSON(ctx, client, apiURL+"?"+params.Encode(), &resp); err != nil {
			return titles, err
		}

		for _, member := range resp.Query.CategoryMembers {
			if len(titles) >= limit {
				break
			}
			titles = append(titles, member.Title)
		}

		if resp.Continue.CmContinue == "" {
			break
		}
		cmContinue = resp.Continue.CmContinue
		time.Sleep(delay)
	}

	return titles, nil
}

// fetchPage henter en artikel som ren tekst sammen med dens URL og hvornår den sidst blev ændret
func fetchPage(ctx context.Context, client *http.Client, apiURL, title, lang string) (page, error) {
	params := url.Values{
		"action":        {"query"},
		"format":        {"json"},
		"formatversion": {"2"},
		"prop":          {"extracts|info"},
		"inprop":        {"url"},
		"explaintext":   {"1"},
		"redirects":     {"1"},
		"titles":        {title},
	}

	var resp extractResponse
	if err := getJSON(ctx, client, apiURL+"?"+params.Encode(), &resp); err != nil {
		return page{}, err
	}
	if len(resp.Query.Pages) == 0 || resp.Query.Pages[0].Missing {
		return page{}, fmt.Errorf("artiklen findes ikke")
	}

	p := resp.Query.Pages[0]
	if strings.TrimSpace(p.Extract) == "" {
		return page{}, fmt.Errorf("artiklen har intet indhold")
	}

	return page{
		Title:       p.Title,
		URL:         p.FullURL,
		Language:    lang,
		LastUpdated: p.Touched,
		Content:     p.Extract,
	}, nil
}

func getJSON(ctx context.Context, client *http.Client, requestURL string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)

	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("wikipedia svarede %s", res.Status)
	}

	return json.NewDecoder(res.Body).Decode(target)
}

//==============================================================================================================================================================================
// DATABASE
//==============================================================================================================================================================================

// ensurePagesTable opretter tabellen med samme skema som schema.sqlite, hvis den mangler
func ensurePagesTable(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS pages (
		title TEXT PRIMARY KEY UNIQUE,
		url TEXT NOT NULL UNIQUE,
		language TEXT NOT NULL CHECK(language IN ('en', 'da')) DEFAULT 'en',
		last_updated TIMESTAMP,
		content TEXT NOT NULL
	)`)
	return err
}

// savePage indsætter artiklen, eller opdaterer den hvis den allerede er crawlet før
func savePage(db *sql.DB, p page) error {
	_, err := db.Exec(`INSERT INTO pages (title, url, language, last_updated, content)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(title) DO UPDATE SET
			url = excluded.url,
			language = excluded.language,
			last_updated = excluded.last_updated,
			content = excluded.content`,
		p.Title, p.URL, p.Language, p.LastUpdated, p.Content)
	return err
}
