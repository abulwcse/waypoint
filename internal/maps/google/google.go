// Package google implements the maps.Provider interface using the Google Maps
// Platform APIs (Directions + Places Nearby Search).
//
// This adapter requires a Google Maps API key with the Directions API and
// Places API enabled, and billing turned on — it is a paid backend. Select it
// with MAPS_PROVIDER=google; otherwise the app defaults to the free OSM stack.
package google

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	gmaps "googlemaps.github.io/maps"
)

// Provider is a thin wrapper over the Google Maps Go client.
type Provider struct {
	c *gmaps.Client
}

// New builds a Google provider from an API key.
func New(apiKey string) (*Provider, error) {
	c, err := gmaps.NewClient(gmaps.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("create google maps client: %w", err)
	}
	return &Provider{c: c}, nil
}

// Route returns the best route between origin and destination, leaving at the
// given departure time. Origin and destination may be addresses or "lat,lng".
func (p *Provider) Route(ctx context.Context, origin, destination string, departure time.Time) (gmaps.Route, error) {
	req := &gmaps.DirectionsRequest{
		Origin:        origin,
		Destination:   destination,
		DepartureTime: strconv.FormatInt(departure.Unix(), 10),
	}
	routes, _, err := p.c.Directions(ctx, req)
	if err != nil {
		return gmaps.Route{}, fmt.Errorf("directions: %w", err)
	}
	if len(routes) == 0 {
		return gmaps.Route{}, fmt.Errorf("no route found between %q and %q", origin, destination)
	}
	return routes[0], nil
}

// NearbyPlaces searches for places around loc. Pass a Google place type (e.g.
// "mosque", "restaurant") and/or a free-text keyword. radius is in metres.
func (p *Provider) NearbyPlaces(ctx context.Context, loc gmaps.LatLng, placeType, keyword string, radius uint) ([]gmaps.PlacesSearchResult, error) {
	req := &gmaps.NearbySearchRequest{
		Location: &loc,
		Radius:   radius,
		Keyword:  keyword,
	}
	if placeType != "" {
		req.Type = gmaps.PlaceType(placeType)
	}
	resp, err := p.c.NearbySearch(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("nearby search: %w", err)
	}
	return resp.Results, nil
}

// Photo fetches a Places photo by reference (as returned in a
// PlacesSearchResult's Photos, see NearbyPlaces), scaled to at most maxWidth
// pixels wide. The server proxies this rather than handing the frontend a key
// (see cmd/server's handlePhoto) since the Places Photo endpoint needs a key
// scoped for the Places API, which the browser-restricted Maps key isn't.
func (p *Provider) Photo(ctx context.Context, photoRef string, maxWidth uint) (contentType string, data io.ReadCloser, err error) {
	resp, err := p.c.PlacePhoto(ctx, &gmaps.PlacePhotoRequest{PhotoReference: photoRef, MaxWidth: maxWidth})
	if err != nil {
		return "", nil, fmt.Errorf("place photo: %w", err)
	}
	return resp.ContentType, resp.Data, nil
}

// Suggest returns city-name completions via the Places Autocomplete API.
func (p *Provider) Suggest(ctx context.Context, query string) ([]string, error) {
	query = strings.TrimSpace(query)
	if len(query) < 2 {
		return nil, nil
	}
	resp, err := p.c.PlaceAutocomplete(ctx, &gmaps.PlaceAutocompleteRequest{
		Input: query,
		Types: gmaps.AutocompletePlaceTypeCities,
	})
	if err != nil {
		return nil, fmt.Errorf("autocomplete: %w", err)
	}
	out := make([]string, 0, len(resp.Predictions))
	for _, pr := range resp.Predictions {
		if pr.Description != "" {
			out = append(out, pr.Description)
		}
	}
	return out, nil
}
