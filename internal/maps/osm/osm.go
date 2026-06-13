// Package osm implements the maps.Provider interface using free, keyless
// OpenStreetMap services:
//
//   - Nominatim  geocodes addresses to coordinates   (no API key)
//   - OSRM       computes the driving route + steps   (no API key)
//   - Overpass   finds places by tag around a point   (no API key)
//
// No billing account or API key is required. The endpoints default to the
// public community servers; override them via the NOMINATIM_URL, OSRM_URL, and
// OVERPASS_URL environment variables (e.g. to point at your own instances).
//
// Results are returned using the googlemaps.github.io/maps structs purely as a
// neutral data carrier, so the rest of the app is provider-agnostic.
package osm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	gmaps "googlemaps.github.io/maps"
)

// userAgent identifies this app to the public OSM services. Nominatim's usage
// policy requires a valid identifying User-Agent.
const userAgent = "waypoint/1.0 (learning project; https://github.com/abulhassan/waypoint)"

// Provider talks to the OpenStreetMap service stack over HTTP.
type Provider struct {
	http      *http.Client
	nominatim string
	osrm      string
	overpass  string
	photon    string
}

// New builds an OSM provider. It takes no API key — every backing service is
// free and keyless.
func New() *Provider {
	return &Provider{
		http:      &http.Client{Timeout: 25 * time.Second},
		nominatim: envOr("NOMINATIM_URL", "https://nominatim.openstreetmap.org"),
		osrm:      envOr("OSRM_URL", "https://router.project-osrm.org"),
		overpass:  envOr("OVERPASS_URL", "https://overpass-api.de/api/interpreter"),
		photon:    envOr("PHOTON_URL", "https://photon.komoot.io/api"),
	}
}

// Route returns the best driving route between origin and destination. Each may
// be an address (geocoded via Nominatim) or a "lat,lng" pair. departure is
// accepted for interface compatibility but unused — OSRM has no live-traffic
// model.
func (p *Provider) Route(ctx context.Context, origin, destination string, _ time.Time) (gmaps.Route, error) {
	from, err := p.resolve(ctx, origin)
	if err != nil {
		return gmaps.Route{}, fmt.Errorf("origin %q: %w", origin, err)
	}
	to, err := p.resolve(ctx, destination)
	if err != nil {
		return gmaps.Route{}, fmt.Errorf("destination %q: %w", destination, err)
	}

	// OSRM wants lon,lat order. steps=true + geojson geometry gives us per-step
	// start/end coordinates, which the planner interpolates between.
	u := fmt.Sprintf("%s/route/v1/driving/%f,%f;%f,%f?overview=false&steps=true&geometries=geojson",
		strings.TrimRight(p.osrm, "/"), from.Lng, from.Lat, to.Lng, to.Lat)

	var resp osrmResponse
	if err := p.getJSON(ctx, u, &resp); err != nil {
		return gmaps.Route{}, fmt.Errorf("routing: %w", err)
	}
	if resp.Code != "Ok" || len(resp.Routes) == 0 {
		return gmaps.Route{}, fmt.Errorf("no route found between %q and %q", origin, destination)
	}

	r := resp.Routes[0]
	route := gmaps.Route{Legs: make([]*gmaps.Leg, 0, len(r.Legs))}
	var summaries []string
	for _, l := range r.Legs {
		if l.Summary != "" {
			summaries = append(summaries, l.Summary)
		}
		leg := &gmaps.Leg{
			Distance: gmaps.Distance{Meters: int(math.Round(l.Distance)), HumanReadable: fmt.Sprintf("%.1f km", l.Distance/1000)},
			Duration: seconds(l.Duration),
			Steps:    make([]*gmaps.Step, 0, len(l.Steps)),
		}
		for _, s := range l.Steps {
			coords := s.Geometry.Coordinates
			if len(coords) == 0 {
				continue
			}
			leg.Steps = append(leg.Steps, &gmaps.Step{
				StartLocation: lonLat(coords[0]),
				EndLocation:   lonLat(coords[len(coords)-1]),
				Duration:      seconds(s.Duration),
			})
		}
		route.Legs = append(route.Legs, leg)
	}
	route.Summary = strings.Join(dedupe(summaries), ", ")
	if route.Summary == "" {
		route.Summary = "via OpenStreetMap"
	}
	return route, nil
}

// NearbyPlaces searches OpenStreetMap (via Overpass) for places near loc. It
// maps the app's place type / keyword onto OSM tags. radius is in metres.
func (p *Provider) NearbyPlaces(ctx context.Context, loc gmaps.LatLng, placeType, keyword string, radius uint) ([]gmaps.PlacesSearchResult, error) {
	filters := overpassFilters(placeType, keyword)
	if len(filters) == 0 {
		return nil, nil
	}

	var q strings.Builder
	q.WriteString("[out:json][timeout:25];(")
	for _, f := range filters {
		fmt.Fprintf(&q, "nwr%s(around:%d,%f,%f);", f, radius, loc.Lat, loc.Lng)
	}
	q.WriteString(");out tags center 40;")

	form := url.Values{"data": {q.String()}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.overpass, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", userAgent)

	var resp overpassResponse
	if err := p.do(req, &resp); err != nil {
		return nil, fmt.Errorf("nearby search: %w", err)
	}

	out := make([]gmaps.PlacesSearchResult, 0, len(resp.Elements))
	for _, e := range resp.Elements {
		lat, lng := e.Lat, e.Lon
		if e.Center != nil {
			lat, lng = e.Center.Lat, e.Center.Lon
		}
		if lat == 0 && lng == 0 {
			continue // no usable location
		}
		out = append(out, gmaps.PlacesSearchResult{
			Name:     placeName(e.Tags),
			Vicinity: vicinity(e.Tags),
			Geometry: gmaps.AddressGeometry{Location: gmaps.LatLng{Lat: lat, Lng: lng}},
			PlaceID:  fmt.Sprintf("%s/%d", e.Type, e.ID),
		})
	}
	return out, nil
}

// Suggest returns up to 5 city-name completions via Photon, komoot's OSM-based
// autocomplete service (free, keyless, built for prefix/type-ahead search —
// Nominatim is a geocoder, not an autocomplete engine).
func (p *Provider) Suggest(ctx context.Context, query string) ([]string, error) {
	query = strings.TrimSpace(query)
	if len(query) < 2 {
		return nil, nil
	}
	u := fmt.Sprintf("%s/?q=%s&limit=5&layer=city",
		strings.TrimRight(p.photon, "/"), url.QueryEscape(query))

	var resp struct {
		Features []struct {
			Properties struct {
				Name    string `json:"name"`
				City    string `json:"city"`
				State   string `json:"state"`
				County  string `json:"county"`
				Country string `json:"country"`
			} `json:"properties"`
		} `json:"features"`
	}
	if err := p.getJSON(ctx, u, &resp); err != nil {
		return nil, err
	}

	out := make([]string, 0, len(resp.Features))
	seen := map[string]bool{}
	for _, f := range resp.Features {
		label := photonLabel(f.Properties.Name, f.Properties.City, f.Properties.State, f.Properties.County, f.Properties.Country)
		if label != "" && !seen[label] {
			seen[label] = true
			out = append(out, label)
		}
	}
	return out, nil
}

// photonLabel assembles a readable "City, Region, Country" label, skipping
// empty and duplicate parts.
func photonLabel(name, city, state, county, country string) string {
	region := state
	if region == "" {
		region = county
	}
	parts := make([]string, 0, 3)
	seen := map[string]bool{}
	for _, c := range []string{firstNonEmpty(name, city), region, country} {
		if c != "" && !seen[c] {
			seen[c] = true
			parts = append(parts, c)
		}
	}
	return strings.Join(parts, ", ")
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// resolve turns an address or "lat,lng" string into coordinates.
func (p *Provider) resolve(ctx context.Context, s string) (gmaps.LatLng, error) {
	if loc, ok := parseLatLng(s); ok {
		return loc, nil
	}
	return p.geocode(ctx, s)
}

// geocode looks up a free-text location via Nominatim.
func (p *Provider) geocode(ctx context.Context, query string) (gmaps.LatLng, error) {
	u := fmt.Sprintf("%s/search?format=jsonv2&limit=1&q=%s",
		strings.TrimRight(p.nominatim, "/"), url.QueryEscape(query))

	var hits []struct {
		Lat string `json:"lat"`
		Lon string `json:"lon"`
	}
	if err := p.getJSON(ctx, u, &hits); err != nil {
		return gmaps.LatLng{}, err
	}
	if len(hits) == 0 {
		return gmaps.LatLng{}, fmt.Errorf("no match found")
	}
	lat, err1 := strconv.ParseFloat(hits[0].Lat, 64)
	lng, err2 := strconv.ParseFloat(hits[0].Lon, 64)
	if err1 != nil || err2 != nil {
		return gmaps.LatLng{}, fmt.Errorf("bad coordinates in geocode response")
	}
	return gmaps.LatLng{Lat: lat, Lng: lng}, nil
}

// getJSON issues a GET with the required headers and decodes the JSON body.
func (p *Provider) getJSON(ctx context.Context, u string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	return p.do(req, out)
}

func (p *Provider) do(req *http.Request, out any) error {
	res, err := p.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return fmt.Errorf("%s: %s", res.Status, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(res.Body).Decode(out)
}

// --- OSM tag mapping ---

// overpassFilters maps the app's place type / keyword onto Overpass tag
// filters. Each returned string is a complete tag selector; results matching
// any of them are unioned.
func overpassFilters(placeType, keyword string) []string {
	switch placeType {
	case "mosque":
		return []string{`["amenity"="place_of_worship"]["religion"="muslim"]`}
	case "restaurant":
		return []string{`["amenity"="restaurant"]`}
	case "gas_station":
		return []string{`["amenity"="fuel"]`}
	case "pharmacy":
		return []string{`["amenity"="pharmacy"]`}
	case "parking":
		return []string{`["amenity"="parking"]`}
	case "cafe":
		return []string{`["amenity"="cafe"]`}
	case "atm":
		return []string{`["amenity"="atm"]`}
	case "hospital":
		return []string{`["amenity"="hospital"]`}
	}

	kw := strings.ToLower(strings.TrimSpace(keyword))
	switch {
	case kw == "":
		return nil
	case strings.Contains(kw, "toilet") || kw == "wc":
		return []string{`["amenity"="toilets"]`}
	default:
		// Fall back to a case-insensitive name match.
		return []string{fmt.Sprintf(`["name"~%q,i]`, keyword)}
	}
}

// placeName returns a human-readable name for an OSM element, falling back to a
// humanised amenity tag for unnamed features (common for toilets, parking).
func placeName(tags map[string]string) string {
	if n := tags["name"]; n != "" {
		return n
	}
	if a := tags["amenity"]; a != "" {
		return titleCase(strings.ReplaceAll(a, "_", " "))
	}
	return "Unnamed place"
}

// vicinity builds a short address line from OSM addr:* tags.
func vicinity(tags map[string]string) string {
	var parts []string
	if street := strings.TrimSpace(strings.Join([]string{tags["addr:housenumber"], tags["addr:street"]}, " ")); street != "" {
		parts = append(parts, street)
	}
	for _, k := range []string{"addr:suburb", "addr:city", "addr:town", "addr:postcode"} {
		if v := tags[k]; v != "" {
			parts = append(parts, v)
		}
	}
	return strings.Join(parts, ", ")
}

// --- small helpers ---

// parseLatLng parses a "lat,lng" string, returning ok=false if it isn't one.
func parseLatLng(s string) (gmaps.LatLng, bool) {
	a, b, ok := strings.Cut(s, ",")
	if !ok {
		return gmaps.LatLng{}, false
	}
	lat, err1 := strconv.ParseFloat(strings.TrimSpace(a), 64)
	lng, err2 := strconv.ParseFloat(strings.TrimSpace(b), 64)
	if err1 != nil || err2 != nil || lat < -90 || lat > 90 || lng < -180 || lng > 180 {
		return gmaps.LatLng{}, false
	}
	return gmaps.LatLng{Lat: lat, Lng: lng}, true
}

// lonLat converts a GeoJSON [lon, lat] pair to a LatLng.
func lonLat(c []float64) gmaps.LatLng {
	if len(c) < 2 {
		return gmaps.LatLng{}
	}
	return gmaps.LatLng{Lng: c[0], Lat: c[1]}
}

func seconds(s float64) time.Duration {
	return time.Duration(s * float64(time.Second))
}

// titleCase upper-cases the first letter of each space-separated word.
func titleCase(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := in[:0]
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// --- response shapes ---

type osrmResponse struct {
	Code   string `json:"code"`
	Routes []struct {
		Legs []struct {
			Summary  string  `json:"summary"`
			Distance float64 `json:"distance"`
			Duration float64 `json:"duration"`
			Steps    []struct {
				Distance float64 `json:"distance"`
				Duration float64 `json:"duration"`
				Geometry struct {
					Coordinates [][]float64 `json:"coordinates"`
				} `json:"geometry"`
			} `json:"steps"`
		} `json:"legs"`
	} `json:"routes"`
}

type overpassResponse struct {
	Elements []struct {
		Type   string  `json:"type"`
		ID     int64   `json:"id"`
		Lat    float64 `json:"lat"`
		Lon    float64 `json:"lon"`
		Center *struct {
			Lat float64 `json:"lat"`
			Lon float64 `json:"lon"`
		} `json:"center"`
		Tags map[string]string `json:"tags"`
	} `json:"elements"`
}
