// Package trip orchestrates the end-to-end plan: route a journey, estimate
// positions at target times, and find nearby places. Both the CLI and the HTTP
// server build on this so the logic lives in one place.
package trip

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/abulhassan/waypoint/internal/maps"
	"github.com/abulhassan/waypoint/internal/planner"
	"github.com/abulhassan/waypoint/internal/poi"
	gmaps "googlemaps.github.io/maps"
)

// Request describes a journey to plan. Times are already resolved to absolute
// values by the caller (CLI flags or HTTP JSON).
type Request struct {
	From       string
	To         string
	Depart     time.Time
	Targets    []time.Time     // explicit clock times to stop at
	Every      time.Duration   // if > 0, generate targets on this interval
	Categories []poi.Category  // place types to search for
	Radius     uint            // metres
	Top        int             // max results per category per stop
}

// Result is the full plan, in a form that's easy to print or marshal to JSON.
type Result struct {
	Summary     string    `json:"summary"`
	DistanceKm  float64   `json:"distanceKm"`
	DurationMin int       `json:"durationMin"`
	Depart      time.Time `json:"depart"`
	Arrive      time.Time `json:"arrive"`
	Stops       []Stop    `json:"stops"`
	Suggestions []string  `json:"suggestions"`
}

// Stop is one planned stop along the route, with the places found near it.
type Stop struct {
	At         time.Time  `json:"at"`
	OffsetMin  int        `json:"offsetMin"`
	Lat        float64    `json:"lat"`
	Lng        float64    `json:"lng"`
	Categories []CatBlock `json:"categories"`
}

// CatBlock groups the places found for one category at a stop.
type CatBlock struct {
	Label  string  `json:"label"`
	Places []Place `json:"places"`
}

// Place is a single nearby result.
type Place struct {
	Name       string  `json:"name"`
	Vicinity   string  `json:"vicinity"`
	Rating     float32 `json:"rating"`
	OpenNow    *bool   `json:"openNow"`
	DistanceKm float64 `json:"distanceKm"`
	MapsURL    string  `json:"mapsUrl"`
}

// Planner runs trips against a maps provider (OSM or Google — see package maps).
type Planner struct {
	client maps.Provider
}

// New builds a Planner backed by the given maps provider.
func New(client maps.Provider) *Planner {
	return &Planner{client: client}
}

// Suggest returns place-name completions for a partial query (type-ahead).
func (p *Planner) Suggest(ctx context.Context, query string) ([]string, error) {
	return p.client.Suggest(ctx, query)
}

// Plan executes the full journey plan.
func (p *Planner) Plan(ctx context.Context, req Request) (*Result, error) {
	if req.From == "" || req.To == "" {
		return nil, fmt.Errorf("from and to are required")
	}
	if len(req.Categories) == 0 {
		return nil, fmt.Errorf("at least one place type is required")
	}
	if req.Radius == 0 {
		req.Radius = 5000
	}
	if req.Top <= 0 {
		req.Top = 3
	}

	route, err := p.client.Route(ctx, req.From, req.To, req.Depart)
	if err != nil {
		return nil, err
	}
	total := planner.TotalDuration(route)

	targets := req.Targets
	switch {
	case req.Every > 0:
		targets = planner.IntervalTargets(req.Depart, total, req.Every)
	case len(targets) == 0:
		targets = []time.Time{req.Depart.Add(total / 2)} // midpoint default
	}

	stops := planner.Plan(route, req.Depart, targets)

	res := &Result{
		Summary:     route.Summary,
		DistanceKm:  routeDistanceKm(route),
		DurationMin: int(total.Round(time.Minute) / time.Minute),
		Depart:      req.Depart,
		Arrive:      req.Depart.Add(total),
		Stops:       make([]Stop, 0, len(stops)),
	}

	for _, s := range stops {
		stop := Stop{
			At:        s.At,
			OffsetMin: int(s.Offset.Round(time.Minute) / time.Minute),
			Lat:       s.Location.Lat,
			Lng:       s.Location.Lng,
		}
		for _, cat := range req.Categories {
			results, err := p.client.NearbyPlaces(ctx, s.Location, cat.Type, cat.Keyword, req.Radius)
			block := CatBlock{Label: cat.Label}
			if err == nil {
				block.Places = topPlaces(s.Location, results, req.Top)
			}
			stop.Categories = append(stop.Categories, block)
		}
		res.Stops = append(res.Stops, stop)
	}

	res.Suggestions = suggestions(total, len(res.Stops))
	return res, nil
}

func topPlaces(center gmaps.LatLng, results []gmaps.PlacesSearchResult, top int) []Place {
	sort.Slice(results, func(i, j int) bool {
		return poi.DistanceMetres(center, results[i].Geometry.Location) <
			poi.DistanceMetres(center, results[j].Geometry.Location)
	})
	out := make([]Place, 0, top)
	for i, r := range results {
		if i >= top {
			break
		}
		var openNow *bool
		if r.OpeningHours != nil && r.OpeningHours.OpenNow != nil {
			openNow = r.OpeningHours.OpenNow
		}
		out = append(out, Place{
			Name:       r.Name,
			Vicinity:   r.Vicinity,
			Rating:     r.Rating,
			OpenNow:    openNow,
			DistanceKm: poi.DistanceMetres(center, r.Geometry.Location) / 1000,
			MapsURL:    mapsLink(r),
		})
	}
	return out
}

func routeDistanceKm(route gmaps.Route) float64 {
	var m int
	for _, leg := range route.Legs {
		m += leg.Distance.Meters
	}
	return float64(m) / 1000
}

func suggestions(total time.Duration, stopCount int) []string {
	out := []string{
		"Positions are estimates from typical driving times (no live traffic), so real arrival drifts — leave a buffer around prayer times.",
		"Widen the radius if a category shows nothing, or narrow it to keep detours short.",
	}
	if total > 4*time.Hour && stopCount < 2 {
		out = append([]string{"That's a long drive — add more stop times or use an interval for regular breaks."}, out...)
	}
	return out
}

func mapsLink(r gmaps.PlacesSearchResult) string {
	if r.PlaceID != "" {
		return fmt.Sprintf("https://www.google.com/maps/search/?api=1&query=%.6f,%.6f&query_place_id=%s",
			r.Geometry.Location.Lat, r.Geometry.Location.Lng, r.PlaceID)
	}
	return fmt.Sprintf("https://www.google.com/maps/search/?api=1&query=%.6f,%.6f",
		r.Geometry.Location.Lat, r.Geometry.Location.Lng)
}

// --- input parsing shared by CLI and HTTP ---

// ParseDeparture accepts "now", "HH:MM" (today), or RFC3339.
func ParseDeparture(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	now := time.Now()
	if s == "" || strings.EqualFold(s, "now") {
		return now, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.ParseInLocation("15:04", s, now.Location()); err == nil {
		return time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, now.Location()), nil
	}
	return time.Time{}, fmt.Errorf("could not parse departure %q (use \"now\", \"HH:MM\", or RFC3339)", s)
}

// ParseClockTimes parses "HH:MM" values into times on ref's date.
func ParseClockTimes(values []string, ref time.Time) ([]time.Time, error) {
	var out []time.Time
	for _, part := range values {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		t, err := time.ParseInLocation("15:04", part, ref.Location())
		if err != nil {
			return nil, fmt.Errorf("could not parse time %q (use HH:MM)", part)
		}
		out = append(out, time.Date(ref.Year(), ref.Month(), ref.Day(), t.Hour(), t.Minute(), 0, 0, ref.Location()))
	}
	return out, nil
}

// ResolveTypes maps category aliases to Categories.
func ResolveTypes(aliases []string) ([]poi.Category, error) {
	var out []poi.Category
	for _, a := range aliases {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		c, ok := poi.Resolve(a)
		if !ok {
			return nil, fmt.Errorf("unknown type %q (supported: %s)", a, strings.Join(poi.Aliases(), ", "))
		}
		out = append(out, c)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no valid types given")
	}
	return out, nil
}
