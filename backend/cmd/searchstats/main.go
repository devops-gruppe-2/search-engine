package main

//==============================================================================================================================================================================
// SEARCHSTATS
//
// Læser søge-loggen (logs/searches.log) og viser, hvad brugerne søger efter.
// Listen over søgninger uden resultater fortæller, hvad crawleren skal hente næste gang.
//
// Lokalt fra backend-mappen:
//   go run ./cmd/searchstats -file logs/searches.log
// På serveren (programmet ligger i containeren):
//   docker exec whoknows-backend ./searchstats
//==============================================================================================================================================================================

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"time"
)

// entry er én linje i loggen (skrevet af logSearch i backend/searchlog.go)
type entry struct {
	Time     time.Time `json:"time"`
	Msg      string    `json:"msg"`
	Query    string    `json:"query"`
	Language string    `json:"language"`
	Results  int       `json:"results"`
}

// count er en søgning og hvor mange gange den er lavet
type count struct {
	Query string
	Times int
}

type stats struct {
	Total       int
	NoResults   int
	Days        int
	TopSearches []count
	TopMissing  []count
}

// summarize tæller søgningerne i loggen. Linjer der ikke er søgninger springes over.
func summarize(r io.Reader, top int) (stats, error) {
	searches := map[string]int{}
	missing := map[string]int{}
	days := map[string]bool{}
	var s stats

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		var e entry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil || e.Msg != "search" || e.Query == "" {
			continue
		}
		s.Total++
		searches[e.Query]++
		days[e.Time.Format(time.DateOnly)] = true
		if e.Results == 0 {
			s.NoResults++
			missing[e.Query]++
		}
	}
	if err := scanner.Err(); err != nil {
		return s, err
	}

	s.Days = len(days)
	s.TopSearches = mostCommon(searches, top)
	s.TopMissing = mostCommon(missing, top)
	return s, nil
}

// mostCommon giver de n hyppigste søgninger, de hyppigste først (ved lighed alfabetisk)
func mostCommon(counts map[string]int, n int) []count {
	list := make([]count, 0, len(counts))
	for query, times := range counts {
		list = append(list, count{query, times})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Times != list[j].Times {
			return list[i].Times > list[j].Times
		}
		return list[i].Query < list[j].Query
	})
	if len(list) > n {
		list = list[:n]
	}
	return list
}

func printList(w io.Writer, title string, list []count) {
	_, _ = fmt.Fprintf(w, "\n%s\n", title)
	if len(list) == 0 {
		_, _ = fmt.Fprintln(w, "  (ingen)")
	}
	for i, c := range list {
		_, _ = fmt.Fprintf(w, "  %2d. %-40s %d\n", i+1, c.Query, c.Times)
	}
}

func main() {
	file := flag.String("file", "logs/searches.log", "Stien til søge-loggen")
	top := flag.Int("top", 10, "Hvor mange søgninger der vises i hver liste")
	flag.Parse()

	f, err := os.Open(*file)
	if err != nil {
		log.Fatalf("Kan ikke åbne loggen: %v", err)
	}
	defer func() { _ = f.Close() }()

	s, err := summarize(f, *top)
	if err != nil {
		log.Fatalf("Kan ikke læse loggen: %v", err)
	}

	out := os.Stdout
	_, _ = fmt.Fprintf(out, "Søgninger i alt:       %d\n", s.Total)
	if s.Total > 0 {
		_, _ = fmt.Fprintf(out, "Uden resultater:       %d (%.0f %%)\n", s.NoResults, 100*float64(s.NoResults)/float64(s.Total))
		_, _ = fmt.Fprintf(out, "Søgninger pr. dag:     %.1f (over %d dage)\n", float64(s.Total)/float64(s.Days), s.Days)
	}
	printList(out, "Mest søgte:", s.TopSearches)
	printList(out, "Mest søgte uden resultater (det bør crawleren hente):", s.TopMissing)
}
