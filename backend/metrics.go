package main

// Monitoring med Prometheus, bygget som Anders' eksempel i whoknows_variations (branch monitoring):
// appen tæller ting og viser tallene på /metrics, Prometheus henter dem, og Grafana tegner dem.
//
// Målingerne handler om brugerne, ikke om serveren:
//   - Finder brugerne det, de søger efter?               whoknows_searches_total{source, results}
//   - Opretter folk sig, og lykkes det?                  whoknows_registrations_total{result}
//   - Kan brugerne logge ind, eller fejler det?          whoknows_logins_total{result}
//   - Hvor mange forskellige brugere er aktive i dag?    whoknows_active_users_today
//   - Hvor mange brugere har vi i alt?                   whoknows_users_total
//   - Hvor længe venter brugeren på hver side?           whoknows_request_duration_milliseconds{route}
//   - Hvor mange svar sender vi, og med hvilken status?  whoknows_http_responses_total{route, status}
//
// /metrics er kun til Prometheus. Nginx svarer 404 på den udefra.

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// Samme navn som i Anders' eksempel, men delt op på route og status
	httpResponses = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "whoknows_http_responses_total",
		Help: "The count of HTTP responses sent, by route and status code.",
	}, []string{"route", "status"})

	// Samme navn som i Anders' eksempel, men delt op på route
	requestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "whoknows_request_duration_milliseconds",
		Help:    "Request duration distribution in milliseconds, by route.",
		Buckets: []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500},
	}, []string{"route"})

	// Søgninger, delt op efter om de gav resultater. "none" er brugere, der ikke fandt noget.
	searchesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "whoknows_searches_total",
		Help: "Searches made by users, by source (web/api) and whether they found anything (found/none).",
	}, []string{"source", "results"})

	// Registreringer: success, invalid (fejl i brugerens input, 422) eller error (fejl hos os)
	registrationsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "whoknows_registrations_total",
		Help: "Registration attempts by result (success/invalid/error).",
	}, []string{"result"})

	// Logins: success, failed (forkert brugernavn eller password), missing (tomme felter),
	// must_change_password (ramt af sikkerhedsbruddet) eller error (fejl hos os)
	loginsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "whoknows_logins_total",
		Help: "Login attempts by result (success/failed/missing/must_change_password/error).",
	}, []string{"result"})

	// Antal brugere i databasen. Tælles hver gang Prometheus henter tallene.
	_ = promauto.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "whoknows_users_total",
		Help: "Total number of registered users.",
	}, countUsers)

	// Antal forskellige brugere, der har brugt siden logget ind i dag (UTC)
	_ = promauto.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "whoknows_active_users_today",
		Help: "Number of different logged in users who have made a request today (UTC).",
	}, func() float64 { return float64(activeUsers.count(time.Now())) })
)

// countUsers tæller brugerne i databasen. Fejler det, vises -1, så det kan ses på grafen.
func countUsers() float64 {
	if db == nil {
		return -1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		return -1
	}
	return float64(count)
}

// dailyUsers husker, hvilke brugere der har været aktive i dag. Listen ligger kun i
// hukommelsen og starter forfra ved midnat (UTC). Kun antallet bliver vist på /metrics,
// aldrig brugernavnene.
type dailyUsers struct {
	mu    sync.Mutex
	day   string
	users map[string]struct{}
}

var activeUsers = &dailyUsers{}

func (d *dailyUsers) resetIfNewDay(now time.Time) {
	if today := now.UTC().Format(time.DateOnly); d.day != today {
		d.day = today
		d.users = map[string]struct{}{}
	}
}

func (d *dailyUsers) seen(username string, now time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.resetIfNewDay(now)
	d.users[username] = struct{}{}
}

func (d *dailyUsers) count(now time.Time) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.resetIfNewDay(now)
	return len(d.users)
}

// metricsMiddleware måler hvert request, ligesom before_request og after_request i Anders'
// eksempel. Route er mønsteret fra Gin (fx "/api/search"), så "?q=..." ikke giver en ny
// linje i Prometheus for hver søgning.
func metricsMiddleware(c *gin.Context) {
	started := time.Now()
	c.Next()

	route := c.FullPath()
	if route == "" {
		route = "not_found"
	}
	httpResponses.WithLabelValues(route, strconv.Itoa(c.Writer.Status())).Inc()
	requestDuration.WithLabelValues(route).Observe(float64(time.Since(started).Microseconds()) / 1000)

	if username, ok := sessions.Default(c).Get("username").(string); ok {
		activeUsers.seen(username, time.Now())
	}
}

// metricsHandler er /metrics, som Prometheus henter tallene fra
var metricsHandler = gin.WrapH(promhttp.Handler())

// countSearch tæller en søgning i whoknows_searches_total
func countSearch(source string, results int) {
	label := "found"
	if results == 0 {
		label = "none"
	}
	searchesTotal.WithLabelValues(source, label).Inc()
}
