package openweather

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/abulhassan/waypoint/internal/weather"
)

func TestForecastPicksNearestHourAndMapsFields(t *testing.T) {
	now := time.Now().Truncate(time.Hour)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[
			{"dt":` + strconv.FormatInt(now.Unix(), 10) + `,"temp":10,"feels_like":9,"humidity":50,"uvi":1,"visibility":10000,"wind_speed":2,"pop":0.1,"weather":[{"id":800,"description":"clear sky","icon":"01d"}]},
			{"dt":` + strconv.FormatInt(now.Add(time.Hour).Unix(), 10) + `,"temp":22.5,"feels_like":23.1,"humidity":80,"uvi":5.5,"visibility":8000,"wind_speed":5,"pop":0.6,"weather":[{"id":501,"description":"moderate rain","icon":"10d"}]}
		]}`))
	}))
	defer srv.Close()

	p := New("test-key")
	p.baseURL = srv.URL

	out, err := p.Forecast(t.Context(), []weather.Point{{Lat: 51.5, Lng: -0.1, At: now.Add(time.Hour)}})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0] == nil {
		t.Fatalf("expected one non-nil result, got %+v", out)
	}
	c := out[0]
	if c.Source != "OpenWeatherMap" {
		t.Errorf("source = %q", c.Source)
	}
	if c.TempC != 22.5 {
		t.Errorf("tempC = %v, want 22.5 (should pick the second, closer hour)", c.TempC)
	}
	if c.Description != "Moderate rain" {
		t.Errorf("description = %q", c.Description)
	}
	if c.Icon != "🌧️" {
		t.Errorf("icon = %q", c.Icon)
	}
	if c.PrecipPercent != 60 {
		t.Errorf("precipPercent = %d, want 60", c.PrecipPercent)
	}
	if got := c.WindKph; got < 17.9 || got > 18.1 {
		t.Errorf("windKph = %v, want ~18 (5 m/s * 3.6)", got)
	}
	if c.HumidityPct == nil || *c.HumidityPct != 80 {
		t.Errorf("humidityPct = %v, want 80", c.HumidityPct)
	}
	if c.UVIndex == nil || *c.UVIndex != 5.5 {
		t.Errorf("uvIndex = %v, want 5.5", c.UVIndex)
	}
	if c.VisibilityKm == nil || *c.VisibilityKm != 8 {
		t.Errorf("visibilityKm = %v, want 8", c.VisibilityKm)
	}
}

func TestForecastOutOfWindowReturnsNil(t *testing.T) {
	now := time.Now().Truncate(time.Hour)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"dt":` + strconv.FormatInt(now.Unix(), 10) + `,"temp":10,"weather":[{"id":800,"description":"clear sky","icon":"01d"}]}]}`))
	}))
	defer srv.Close()

	p := New("test-key")
	p.baseURL = srv.URL

	out, err := p.Forecast(t.Context(), []weather.Point{{Lat: 51.5, Lng: -0.1, At: now.Add(72 * time.Hour)}})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0] != nil {
		t.Fatalf("expected a nil result for a target far outside the forecast window, got %+v", out)
	}
}
