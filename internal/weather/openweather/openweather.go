// Package openweather implements weather.Provider using OpenWeatherMap's One
// Call 4.0 hourly timeline API — the richer "pro" forecast backend (humidity,
// UV index, visibility, precipitation probability) offered to signed-in pro
// users. Requires an OPENWEATHERMAP_API_KEY (see .env.example) subscribed to
// the One Call by Call plan (free tier: 1,000 calls/day).
package openweather

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/abulhassan/waypoint/internal/weather"
)

// Provider talks to the OpenWeatherMap One Call 4.0 API.
type Provider struct {
	http    *http.Client
	apiKey  string
	baseURL string
}

// New builds an OpenWeatherMap-backed weather provider.
func New(apiKey string) *Provider {
	return &Provider{
		http:    &http.Client{Timeout: 15 * time.Second},
		apiKey:  apiKey,
		baseURL: envOr("OPENWEATHERMAP_URL", "https://api.openweathermap.org/data/4.0/onecall/timeline/1h"),
	}
}

// Forecast fetches conditions for each point concurrently. Unlike Open-Meteo,
// the One Call timeline endpoint takes one coordinate per request, so there's
// no batching endpoint to use instead — a per-point failure just leaves that
// entry nil.
func (p *Provider) Forecast(ctx context.Context, points []weather.Point) ([]*weather.Conditions, error) {
	out := make([]*weather.Conditions, len(points))
	var wg sync.WaitGroup
	for i, pt := range points {
		wg.Add(1)
		go func(i int, pt weather.Point) {
			defer wg.Done()
			if c, err := p.forecastOne(ctx, pt); err == nil {
				out[i] = c
			}
		}(i, pt)
	}
	wg.Wait()
	return out, nil
}

type owWeatherDesc struct {
	ID          int    `json:"id"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
}

type owEntry struct {
	Dt         int64           `json:"dt"`
	Temp       float64         `json:"temp"`
	FeelsLike  float64         `json:"feels_like"`
	Humidity   int             `json:"humidity"`
	UVI        float64         `json:"uvi"`
	Visibility float64         `json:"visibility"` // metres
	WindSpeed  float64         `json:"wind_speed"` // metres/second
	Pop        float64         `json:"pop"`        // 0..1 probability of precipitation
	Weather    []owWeatherDesc `json:"weather"`
}

func (p *Provider) forecastOne(ctx context.Context, pt weather.Point) (*weather.Conditions, error) {
	q := url.Values{
		"lat":   {strconv.FormatFloat(pt.Lat, 'f', 5, 64)},
		"lon":   {strconv.FormatFloat(pt.Lng, 'f', 5, 64)},
		"appid": {p.apiKey},
		"units": {"metric"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openweathermap: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openweathermap: status %d", resp.StatusCode)
	}

	var body struct {
		Data []owEntry `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("openweathermap: decode: %w", err)
	}

	// The timeline/1h endpoint returns one page (20 hours) starting at the
	// current hour, so a target far outside that window (a very long trip)
	// legitimately has no match.
	target := pt.At.Unix()
	best := -1
	var bestDiff int64
	for i, h := range body.Data {
		diff := target - h.Dt
		if diff < 0 {
			diff = -diff
		}
		if best == -1 || diff < bestDiff {
			best, bestDiff = i, diff
		}
	}
	if best == -1 || bestDiff > 90*60 {
		return nil, fmt.Errorf("no forecast hour close enough to %s", pt.At)
	}

	h := body.Data[best]
	desc, icon, code := "Unknown", "❓", 0
	if len(h.Weather) > 0 {
		desc = capitalize(h.Weather[0].Description)
		icon = weatherIcon(h.Weather[0].Icon)
		code = h.Weather[0].ID
	}
	humidity := h.Humidity
	uvi := h.UVI
	visKm := h.Visibility / 1000
	return &weather.Conditions{
		Source:        "OpenWeatherMap",
		TempC:         h.Temp,
		FeelsLikeC:    h.FeelsLike,
		Code:          code,
		Description:   desc,
		Icon:          icon,
		PrecipPercent: int(h.Pop*100 + 0.5),
		WindKph:       h.WindSpeed * 3.6,
		HumidityPct:   &humidity,
		UVIndex:       &uvi,
		VisibilityKm:  &visKm,
	}, nil
}

// weatherIcon maps an OpenWeatherMap icon code (e.g. "10d") to the emoji style
// used throughout this app (see internal/weather's describe, and
// web/src/maps/markers.js), ignoring the day/night suffix except for clear sky.
func weatherIcon(code string) string {
	if len(code) < 2 {
		return "❓"
	}
	switch code[:2] {
	case "01":
		if strings.HasSuffix(code, "n") {
			return "🌙"
		}
		return "☀️"
	case "02":
		return "🌤️"
	case "03":
		return "⛅"
	case "04":
		return "☁️"
	case "09":
		return "🌦️"
	case "10":
		return "🌧️"
	case "11":
		return "⛈️"
	case "13":
		return "🌨️"
	case "50":
		return "🌫️"
	default:
		return "❓"
	}
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
