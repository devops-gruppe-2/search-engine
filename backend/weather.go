package main

// Weather: vejrudsigten for København hentes i backenden fra Open-Meteo.
// Open-Meteo er gratis og kræver ingen API-nøgle. Svaret gemmes i en cache,
// så antallet af kald til Open-Meteo ikke vokser med antallet af brugere.

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	openMeteoURL = "https://api.open-meteo.com"

	weatherCity     = "Copenhagen"
	weatherLat      = "55.6761"
	weatherLon      = "12.5683"
	weatherTimezone = "Europe/Copenhagen"
	weatherDays     = "7"

	// Så længe bruges en gemt udsigt, før der hentes en ny
	weatherCacheTTL = 30 * time.Minute
)

// DailyForecast er udsigten for én dag
type DailyForecast struct {
	Date           string  `json:"date"`
	Description    string  `json:"description"`
	WeatherCode    int     `json:"weather_code"`
	TemperatureMax float64 `json:"temperature_max"`
	TemperatureMin float64 `json:"temperature_min"`
	Precipitation  float64 `json:"precipitation"`
}

// Forecast er det, både /weather og /api/weather viser
type Forecast struct {
	City      string          `json:"city"`
	FetchedAt time.Time       `json:"fetched_at"`
	Days      []DailyForecast `json:"forecast"`
}

// openMeteoResponse er de felter fra Open-Meteo, vi bruger. Hver liste har én værdi pr. dag.
type openMeteoResponse struct {
	Daily struct {
		Time             []string  `json:"time"`
		WeatherCode      []int     `json:"weather_code"`
		TemperatureMax   []float64 `json:"temperature_2m_max"`
		TemperatureMin   []float64 `json:"temperature_2m_min"`
		PrecipitationSum []float64 `json:"precipitation_sum"`
	} `json:"daily"`
}

// weatherService henter udsigten og husker den sidste i ttl
type weatherService struct {
	baseURL string
	client  *http.Client
	ttl     time.Duration

	// mu sørger for, at kun én request ad gangen læser eller opdaterer cachen.
	// Kommer mange brugere samtidig, når cachen er udløbet, henter kun den første.
	mu        sync.Mutex
	cached    *Forecast
	fetchedAt time.Time
}

func newWeatherService(baseURL string) *weatherService {
	return &weatherService{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 5 * time.Second},
		ttl:     weatherCacheTTL,
	}
}

// weather er den service, handlerne bruger. Testene skifter den ud med en, der peger på en falsk Open-Meteo.
var weather = newWeatherService(openMeteoURL)

// Forecast giver den gemte udsigt, hvis den er ny nok, og ellers en frisk fra Open-Meteo.
// Er Open-Meteo nede, bruges den sidst gemte udsigt hellere end at vise en fejl.
func (s *weatherService) Forecast(ctx context.Context) (*Forecast, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cached != nil && time.Since(s.fetchedAt) < s.ttl {
		return s.cached, nil
	}

	forecast, err := s.fetch(ctx)
	if err != nil {
		if s.cached != nil {
			log.Printf("Weather: using cached forecast, Open-Meteo failed: %v", err)
			return s.cached, nil
		}
		return nil, err
	}

	s.cached = forecast
	s.fetchedAt = time.Now()
	return forecast, nil
}

// fetch henter en frisk udsigt fra Open-Meteo
func (s *weatherService) fetch(ctx context.Context) (*Forecast, error) {
	params := url.Values{
		"latitude":      {weatherLat},
		"longitude":     {weatherLon},
		"daily":         {"weather_code,temperature_2m_max,temperature_2m_min,precipitation_sum"},
		"timezone":      {weatherTimezone},
		"forecast_days": {weatherDays},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+"/v1/forecast?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("create weather request: %w", err)
	}

	res, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("contact Open-Meteo: %w", err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("open-meteo responded with status %d", res.StatusCode)
	}

	var body openMeteoResponse
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("read Open-Meteo response: %w", err)
	}

	daily := body.Daily
	days := len(daily.Time)
	if days == 0 || len(daily.WeatherCode) != days || len(daily.TemperatureMax) != days ||
		len(daily.TemperatureMin) != days || len(daily.PrecipitationSum) != days {
		return nil, fmt.Errorf("open-meteo response has no complete forecast")
	}

	forecast := &Forecast{City: weatherCity, FetchedAt: time.Now()}
	for i := range days {
		forecast.Days = append(forecast.Days, DailyForecast{
			Date:           daily.Time[i],
			Description:    weatherDescription(daily.WeatherCode[i]),
			WeatherCode:    daily.WeatherCode[i],
			TemperatureMax: daily.TemperatureMax[i],
			TemperatureMin: daily.TemperatureMin[i],
			Precipitation:  daily.PrecipitationSum[i],
		})
	}
	return forecast, nil
}

// weatherDescription oversætter Open-Meteos vejrkode (WMO) til tekst
func weatherDescription(code int) string {
	switch {
	case code == 0:
		return "Clear sky"
	case code <= 2:
		return "Partly cloudy"
	case code == 3:
		return "Overcast"
	case code == 45 || code == 48:
		return "Fog"
	case code >= 51 && code <= 57:
		return "Drizzle"
	case code >= 61 && code <= 67, code >= 80 && code <= 82:
		return "Rain"
	case code >= 71 && code <= 77, code == 85 || code == 86:
		return "Snow"
	case code >= 95:
		return "Thunderstorm"
	default:
		return "Unknown"
	}
}

//==============================================================================================================================================================================
// WEATHER ENDPOINTS
//==============================================================================================================================================================================

// WeatherAPI svarer med {"data": {...}}, som StandardResponse i OpenAPI-spec'en
func WeatherAPI(c *gin.Context) {
	forecast, err := weather.Forecast(c.Request.Context())
	if err != nil {
		log.Printf("Weather: %v", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"data": gin.H{"message": "Weather forecast is unavailable"}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": forecast})
}

// WeatherPage viser udsigten som en side
func WeatherPage(c *gin.Context) {
	forecast, err := weather.Forecast(c.Request.Context())
	if err != nil {
		log.Printf("Weather: %v", err)
		c.HTML(http.StatusOK, "weather.html", pageData(c, gin.H{"Error": "Weather forecast is unavailable right now"}))
		return
	}
	c.HTML(http.StatusOK, "weather.html", pageData(c, gin.H{"Forecast": forecast}))
}
