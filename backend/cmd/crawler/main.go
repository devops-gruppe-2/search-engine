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
	"os"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

//==============================================================================================================================================================================
// CRAWLER
//
// Henter Wikipedia-artikler fra en eller flere kategorier og gemmer dem i pages-tabellen,
// så søgningen har noget indhold at finde.
//
// Kør fra backend-mappen:
//   go run ./cmd/crawler -limit 200
//
// Den skriver til PostgreSQL (DATABASE_URL). Tabellen pages laves af appens migreringer,
// så appen skal have kørt én gang først.
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
	dbURL := flag.String("db", os.Getenv("DATABASE_URL"), "PostgreSQL-forbindelse (standard: DATABASE_URL)")
	lang := flag.String("lang", "en", "Wikipedia-sprog (en eller da)")
	categories := flag.String("categories", "Category:Programming languages,Category:Computer programming", "kategorier adskilt af komma")
	limit := flag.Int("limit", 200, "maks antal artikler der hentes")
	delay := flag.Duration("delay", 200*time.Millisecond, "pause mellem kald, så vi ikke belaster Wikipedia")
	flag.Parse()

	if *dbURL == "" {
		log.Fatal("Angiv databasen med -db eller DATABASE_URL")
	}
	if *lang != "en" && *lang != "da" {
		log.Fatalf("lang skal være en eller da, fik %q", *lang)
	}

	db, err := sql.Open("pgx", *dbURL)
	if err != nil {
		log.Fatalf("Kunne ikke åbne databasen: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("Kunne ikke lukke databasen: %v", err)
		}
	}()

	ctx := context.Background()

	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("Kunne ikke forbinde til databasen: %v", err)
	}

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
		if err := savePage(ctx, db, p); err != nil {
			log.Printf("[%d/%d] Kunne ikke gemme %q: %v", i+1, len(titles), title, err)
			continue
		}
		saved++
		log.Printf("[%d/%d] Gemt %q", i+1, len(titles), title)
		time.Sleep(*delay)
	}

	log.Printf("Færdig: %d af %d artikler gemt i databasen", saved, len(titles))
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
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("wikipedia svarede %s", res.Status)
	}

	return json.NewDecoder(res.Body).Decode(target)
}

//==============================================================================================================================================================================
// DATABASE
//==============================================================================================================================================================================

// savePage indsætter artiklen, eller opdaterer den hvis den allerede er crawlet før
func savePage(ctx context.Context, db *sql.DB, p page) error {
	_, err := db.ExecContext(ctx, `INSERT INTO pages (title, url, language, last_updated, content)
		VALUES ($1, $2, $3, NULLIF($4, '')::timestamptz, $5)
		ON CONFLICT (title) DO UPDATE SET
			url = excluded.url,
			language = excluded.language,
			last_updated = excluded.last_updated,
			content = excluded.content`,
		p.Title, p.URL, p.Language, p.LastUpdated, p.Content)
	return err
}
