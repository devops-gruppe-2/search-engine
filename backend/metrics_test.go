package main

// Tests af målingerne til Prometheus (metrics.go)

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

// Tællerne er fælles for alle tests, så testene ser på, hvor meget de stiger
func counterDelta(t *testing.T, read func() float64, action func()) float64 {
	t.Helper()
	before := read()
	action()
	return read() - before
}

func TestMetricsEndpointShowsTheMeasurements(t *testing.T) {
	app := newTestApp(t)
	app.get("/api/search?q=go")

	res := app.get("/metrics")
	expectStatus(t, res, http.StatusOK)
	for _, name := range []string{
		"whoknows_http_responses_total",
		"whoknows_request_duration_milliseconds",
		"whoknows_searches_total",
		"whoknows_users_total",
		"whoknows_active_users_today",
	} {
		if !strings.Contains(res.body, name) {
			t.Errorf("/metrics is missing %s", name)
		}
	}
}

func TestSearchesAreCountedByResult(t *testing.T) {
	app := newTestApp(t)
	addPage(t, "Python", "en", "Python is a programming language.")

	found := func() float64 { return testutil.ToFloat64(searchesTotal.WithLabelValues("api", "found")) }
	none := func() float64 { return testutil.ToFloat64(searchesTotal.WithLabelValues("web", "none")) }

	if d := counterDelta(t, found, func() { app.get("/api/search?q=python") }); d != 1 {
		t.Errorf("found searches went up by %v, want 1", d)
	}
	if d := counterDelta(t, none, func() { app.do(http.MethodGet, "/?q=quantum", nil, true) }); d != 1 {
		t.Errorf("searches without results went up by %v, want 1", d)
	}
}

func TestLoginsAreCountedByResult(t *testing.T) {
	app := newTestApp(t)
	app.register("alice", "password-1")

	count := func(result string) func() float64 {
		return func() float64 { return testutil.ToFloat64(loginsTotal.WithLabelValues(result)) }
	}
	cases := []struct {
		result string
		form   url.Values
	}{
		{"success", url.Values{"username": {"alice"}, "password": {"password-1"}}},
		{"failed", url.Values{"username": {"alice"}, "password": {"wrong"}}},
		{"failed", url.Values{"username": {"nobody"}, "password": {"x"}}},
		{"missing", url.Values{"username": {"alice"}}},
	}
	for _, tc := range cases {
		if d := counterDelta(t, count(tc.result), func() { app.post("/api/login", tc.form) }); d != 1 {
			t.Errorf("login %v: %s went up by %v, want 1", tc.form, tc.result, d)
		}
	}
}

func TestRegistrationsAreCountedByResult(t *testing.T) {
	app := newTestApp(t)
	success := func() float64 { return testutil.ToFloat64(registrationsTotal.WithLabelValues("success")) }
	invalid := func() float64 { return testutil.ToFloat64(registrationsTotal.WithLabelValues("invalid")) }

	if d := counterDelta(t, success, func() { app.register("bob", "password-1") }); d != 1 {
		t.Errorf("successful registrations went up by %v, want 1", d)
	}
	if d := counterDelta(t, invalid, func() { app.post("/api/register", url.Values{"username": {"bob"}}) }); d != 1 {
		t.Errorf("invalid registrations went up by %v, want 1", d)
	}
}

func TestUsersTotalCountsTheDatabase(t *testing.T) {
	app := newTestApp(t)
	// Skemaet opretter selv en bruger, så vi ser på, hvor meget tallet stiger
	if d := counterDelta(t, countUsers, func() {
		app.register("alice", "password-1")
		app.register("bob", "password-1")
	}); d != 2 {
		t.Fatalf("users total went up by %v, want 2", d)
	}
}

func TestActiveUsersAreCountedOncePerDay(t *testing.T) {
	d := &dailyUsers{}
	monday := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)

	d.seen("alice", monday)
	d.seen("alice", monday.Add(time.Hour))
	d.seen("bob", monday)
	if got := d.count(monday); got != 2 {
		t.Fatalf("active users = %d, want 2", got)
	}
	if got := d.count(monday.Add(24 * time.Hour)); got != 0 {
		t.Fatalf("active users the next day = %d, want 0", got)
	}
}

func TestRequestsAreMeasuredByRoutePattern(t *testing.T) {
	app := newTestApp(t)
	ok := func() float64 {
		return testutil.ToFloat64(httpResponses.WithLabelValues("/api/search", "200"))
	}
	// To forskellige søgninger tæller på samme route, ikke en linje pr. søgning
	if d := counterDelta(t, ok, func() {
		app.get("/api/search?q=a")
		app.get("/api/search?q=b")
	}); d != 2 {
		t.Fatalf("responses for /api/search went up by %v, want 2", d)
	}
}
