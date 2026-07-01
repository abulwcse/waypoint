// Package weather fetches forecast conditions for points along a route, so
// the planner can show what to expect at each stop's estimated arrival time.
//
// The only implementation talks to Open-Meteo (https://open-meteo.com), a
// free, keyless forecast API — in keeping with this project's free-by-default
// stack (see internal/maps/osm).
package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Point is a place and time to fetch a forecast for.
type Point struct {
	Lat float64
	Lng float64
	At  time.Time
}

// Conditions is the forecast nearest to one Point's time. Source identifies
// which backend produced it ("Open-Meteo" or "OpenWeatherMap"), so the UI can
// show pro users what they're getting for their subscription. The pointer
// fields are richer data only the pro (OpenWeatherMap) provider fills in.
type Conditions struct {
	Source        string   `json:"source"`
	TempC         float64  `json:"tempC"`
	FeelsLikeC    float64  `json:"feelsLikeC"`
	Code          int      `json:"code"`
	Description   string   `json:"description"`
	Icon          string   `json:"icon"`
	PrecipPercent int      `json:"precipPercent"`
	WindKph       float64  `json:"windKph"`
	HumidityPct   *int     `json:"humidityPct,omitempty"`
	UVIndex       *float64 `json:"uvIndex,omitempty"`
	VisibilityKm  *float64 `json:"visibilityKm,omitempty"`
}

// Provider fetches forecasts for a batch of points in one round trip. The
// returned slice is the same length as points; an entry is nil if no forecast
// was available for that point (e.g. it falls outside the provider's window).
type Provider interface {
	Forecast(ctx context.Context, points []Point) ([]*Conditions, error)
}

// openMeteo talks to the Open-Meteo hourly forecast API.
type openMeteo struct {
	http    *http.Client
	baseURL string
}

// New builds the Open-Meteo-backed weather provider. No API key needed.
func New() Provider {
	return &openMeteo{
		http:    &http.Client{Timeout: 15 * time.Second},
		baseURL: envOr("OPEN_METEO_URL", "https://api.open-meteo.com/v1/forecast"),
	}
}

func (p *openMeteo) Forecast(ctx context.Context, points []Point) ([]*Conditions, error) {
	if len(points) == 0 {
		return nil, nil
	}

	lats := make([]string, len(points))
	lngs := make([]string, len(points))
	for i, pt := range points {
		lats[i] = strconv.FormatFloat(pt.Lat, 'f', 5, 64)
		lngs[i] = strconv.FormatFloat(pt.Lng, 'f', 5, 64)
	}

	q := url.Values{
		"latitude":  {strings.Join(lats, ",")},
		"longitude": {strings.Join(lngs, ",")},
		"hourly":    {"temperature_2m,apparent_temperature,weathercode,precipitation_probability,windspeed_10m"},
		"timezone":  {"UTC"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("open-meteo: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("open-meteo: status %d", resp.StatusCode)
	}

	// Open-Meteo returns a single forecast object for one location, but an
	// array of them (one per coordinate, in request order) for several.
	var forecasts []hourlyForecast
	if len(points) == 1 {
		var single hourlyForecast
		if err := json.NewDecoder(resp.Body).Decode(&single); err != nil {
			return nil, fmt.Errorf("open-meteo: decode: %w", err)
		}
		forecasts = []hourlyForecast{single}
	} else if err := json.NewDecoder(resp.Body).Decode(&forecasts); err != nil {
		return nil, fmt.Errorf("open-meteo: decode: %w", err)
	}

	out := make([]*Conditions, len(points))
	for i, pt := range points {
		if i < len(forecasts) {
			out[i] = nearestHour(forecasts[i], pt.At)
		}
	}
	return out, nil
}

type hourlyForecast struct {
	Hourly struct {
		Time                     []string  `json:"time"`
		Temperature2m            []float64 `json:"temperature_2m"`
		ApparentTemperature      []float64 `json:"apparent_temperature"`
		WeatherCode              []int     `json:"weathercode"`
		PrecipitationProbability []int     `json:"precipitation_probability"`
		WindSpeed10m             []float64 `json:"windspeed_10m"`
	} `json:"hourly"`
}

// nearestHour finds the hourly sample closest to at (compared in UTC, since
// the request asked for timezone=UTC) and converts it to Conditions. It
// returns nil if the forecast has no data or at falls too far outside the
// returned window to be a meaningful match.
func nearestHour(f hourlyForecast, at time.Time) *Conditions {
	target := at.UTC()
	best := -1
	var bestDiff time.Duration
	for i, ts := range f.Hourly.Time {
		t, err := time.Parse("2006-01-02T15:04", ts)
		if err != nil {
			continue
		}
		diff := target.Sub(t)
		if diff < 0 {
			diff = -diff
		}
		if best == -1 || diff < bestDiff {
			best, bestDiff = i, diff
		}
	}
	if best == -1 || bestDiff > 90*time.Minute {
		return nil
	}

	code := valueAt(f.Hourly.WeatherCode, best)
	desc, icon := describe(code)
	return &Conditions{
		Source:        "Open-Meteo",
		TempC:         valueAt(f.Hourly.Temperature2m, best),
		FeelsLikeC:    valueAt(f.Hourly.ApparentTemperature, best),
		Code:          code,
		Description:   desc,
		Icon:          icon,
		PrecipPercent: valueAt(f.Hourly.PrecipitationProbability, best),
		WindKph:       valueAt(f.Hourly.WindSpeed10m, best),
	}
}

func valueAt[T any](s []T, i int) T {
	var zero T
	if i < 0 || i >= len(s) {
		return zero
	}
	return s[i]
}

// describe maps a WMO weather code (the vocabulary open-meteo's "weathercode"
// field uses) to a short human description and an emoji icon, matching the
// emoji-pin style already used for place categories (see web/src/maps/markers.js).
// https://open-meteo.com/en/docs (see "WMO Weather interpretation codes")
func describe(code int) (description, icon string) {
	switch code {
	case 0:
		return "Clear sky", "☀️"
	case 1:
		return "Mainly clear", "🌤️"
	case 2:
		return "Partly cloudy", "⛅"
	case 3:
		return "Overcast", "☁️"
	case 45, 48:
		return "Fog", "🌫️"
	case 51, 53, 55:
		return "Drizzle", "🌦️"
	case 56, 57:
		return "Freezing drizzle", "🌧️"
	case 61, 63:
		return "Rain", "🌧️"
	case 65:
		return "Heavy rain", "🌧️"
	case 66, 67:
		return "Freezing rain", "🌧️"
	case 71, 73:
		return "Snow", "🌨️"
	case 75, 86:
		return "Heavy snow", "❄️"
	case 77:
		return "Snow grains", "🌨️"
	case 80, 81:
		return "Rain showers", "🌦️"
	case 82:
		return "Violent rain showers", "⛈️"
	case 85:
		return "Snow showers", "🌨️"
	case 95, 96, 99:
		return "Thunderstorm", "⛈️"
	default:
		return "Unknown", "❓"
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
