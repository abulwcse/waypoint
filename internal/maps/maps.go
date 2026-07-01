// Package maps defines the map-data Provider interface the planner depends on,
// and a factory that selects a concrete adapter at runtime.
//
// Two adapters implement Provider:
//
//   - osm    — free OpenStreetMap services (Nominatim + OSRM + Overpass), no key
//   - google — Google Maps (Directions + Places), requires an API key + billing
//
// Choose with the MAPS_PROVIDER environment variable. The default is "osm" so
// the app runs for free out of the box. Both adapters return the same
// googlemaps.github.io/maps structs, so nothing downstream is provider-aware.
package maps

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	gmaps "googlemaps.github.io/maps"

	"github.com/abulhassan/waypoint/internal/maps/google"
	"github.com/abulhassan/waypoint/internal/maps/osm"
)

// Provider is a pluggable map-data backend: routing plus nearby-place search.
// Implementations live in the osm and google sub-packages.
type Provider interface {
	// Route returns the best driving route between origin and destination. Each
	// may be an address or a "lat,lng" pair.
	Route(ctx context.Context, origin, destination string, departure time.Time) (gmaps.Route, error)
	// NearbyPlaces finds places of the given type/keyword within radius metres
	// of loc, as search results to rank by distance.
	NearbyPlaces(ctx context.Context, loc gmaps.LatLng, placeType, keyword string, radius uint) ([]gmaps.PlacesSearchResult, error)
	// Suggest returns up to a handful of place-name completions for a partial
	// query, for type-ahead in the UI.
	Suggest(ctx context.Context, query string) ([]string, error)
}

// New builds the provider selected by the MAPS_PROVIDER environment variable:
//
//   - "" or "osm" (default) → free OpenStreetMap stack, no API key needed
//   - "google"              → Google Maps; requires GOOGLE_MAPS_API_KEY
func New() (Provider, error) {
	switch p := ResolvedName(); p {
	case "osm":
		return osm.New(), nil
	case "google":
		key := os.Getenv("GOOGLE_MAPS_API_KEY")
		if key == "" {
			return nil, fmt.Errorf("MAPS_PROVIDER=google requires GOOGLE_MAPS_API_KEY (see .env.example)")
		}
		return google.New(key)
	default:
		return nil, fmt.Errorf("unknown MAPS_PROVIDER %q (use \"osm\" or \"google\")", p)
	}
}

// ResolvedName returns the normalized provider name ("osm" or "google") that
// New would build, for callers (e.g. the HTTP API) that need to tell the
// frontend which map renderer to use without constructing a client.
func ResolvedName() string {
	switch p := strings.ToLower(strings.TrimSpace(os.Getenv("MAPS_PROVIDER"))); p {
	case "", "osm", "openstreetmap":
		return "osm"
	case "google", "googlemaps":
		return "google"
	default:
		return p
	}
}
