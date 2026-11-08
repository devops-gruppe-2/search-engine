package main

// Integrationstests af weather: appen taler med en falsk Open-Meteo, så testene
// ikke afhænger af internettet og kan styre, hvad Open-Meteo svarer.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

const fakeOpenMeteoBody = `{
	"daily": {
		"time": ["2026-11-09", "2026-11-10"],
		"weather_code": [3, 61],
		"temperature_2m_max": [9.4, 7.1],
		"temperature_2m_min": [4.2, 2.8],
		"precipitation_sum": [0.0, 5.6]
	}
}`

// fakeOpenMeteo er en falsk Open-Meteo, der tæller hvor mange gange den bliver kaldt
type fakeOpenMeteo struct {
	calls   atomic.Int32
	failing atomic.Bool
}

// useFakeOpenMeteo starter den falske Open-Meteo og får appen til at bruge den
func useFakeOpenMeteo(t *testing.T) *fakeOpenMeteo {
	t.Helper()
	fake := &fakeOpenMeteo{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fake.calls.Add(1)
		if fake.failing.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if r.URL.Path != "/v1/forecast" || r.URL.Query().Get("latitude") == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fakeOpenMeteoBody))
	}))
	t.Cleanup(server.Close)

	previous := weather
	weather = newWeatherService(server.URL)
	t.Cleanup(func() { weather = previous })

	return fake
}

func TestWeatherAPIReturnsTheForecast(t *testing.T) {
	app := newTestApp(t)
	useFakeOpenMeteo(t)

	res := app.get("/api/weather")
	expectStatus(t, res, http.StatusOK)

	var body struct {
		Data Forecast `json:"data"`
	}
	if err := json.Unmarshal([]byte(res.body), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body.Data.City != "Copenhagen" || len(body.Data.Days) != 2 {
		t.Fatalf("forecast = %+v, want 2 days for Copenhagen", body.Data)
	}
	rainy := body.Data.Days[1]
	if rainy.Date != "2026-11-10" || rainy.Description != "Rain" || rainy.TemperatureMax != 7.1 || rainy.Precipitation != 5.6 {
		t.Fatalf("second day = %+v", rainy)
	}
}

func TestWeatherIsCachedSoOpenMeteoIsNotCalledForEveryUser(t *testing.T) {
	app := newTestApp(t)
	fake := useFakeOpenMeteo(t)

	for range 5 {
		expectStatus(t, app.get("/api/weather"), http.StatusOK)
	}
	expectStatus(t, app.do(http.MethodGet, "/weather", nil, true), http.StatusOK)

	if got := fake.calls.Load(); got != 1 {
		t.Fatalf("Open-Meteo was called %d times, want 1", got)
	}
}

func TestWeatherUsesTheLastForecastWhenOpenMeteoFails(t *testing.T) {
	app := newTestApp(t)
	fake := useFakeOpenMeteo(t)
	expectStatus(t, app.get("/api/weather"), http.StatusOK)

	// Cachen er udløbet, og Open-Meteo er nede
	weather.ttl = 0
	fake.failing.Store(true)

	res := app.get("/api/weather")
	expectStatus(t, res, http.StatusOK)
	if !strings.Contains(res.body, "2026-11-10") {
		t.Fatalf("expected the cached forecast, got %s", res.body)
	}
	if got := fake.calls.Load(); got != 2 {
		t.Fatalf("Open-Meteo was called %d times, want 2", got)
	}
}

func TestWeatherWithoutForecastShowsAnError(t *testing.T) {
	app := newTestApp(t)
	fake := useFakeOpenMeteo(t)
	fake.failing.Store(true)

	expectStatus(t, app.get("/api/weather"), http.StatusServiceUnavailable)

	page := app.do(http.MethodGet, "/weather", nil, true)
	expectStatus(t, page, http.StatusOK)
	if !strings.Contains(page.body, "Weather forecast is unavailable") {
		t.Fatal("weather page should explain that the forecast is unavailable")
	}
}

func TestWeatherPageShowsTheForecast(t *testing.T) {
	app := newTestApp(t)
	useFakeOpenMeteo(t)

	page := app.do(http.MethodGet, "/weather", nil, true)
	expectStatus(t, page, http.StatusOK)
	for _, want := range []string{`id="weather-forecast"`, "2026-11-09", "Overcast", "9.4 °C", "5.6 mm", `id="nav-weather"`} {
		if !strings.Contains(page.body, want) {
			t.Errorf("weather page is missing %q", want)
		}
	}
}

func TestWeatherDescription(t *testing.T) {
	tests := map[int]string{0: "Clear sky", 2: "Partly cloudy", 3: "Overcast", 45: "Fog", 53: "Drizzle", 63: "Rain", 81: "Rain", 73: "Snow", 95: "Thunderstorm", 42: "Unknown"}
	for code, want := range tests {
		if got := weatherDescription(code); got != want {
			t.Errorf("weatherDescription(%d) = %q, want %q", code, got, want)
		}
	}
}
