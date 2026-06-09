// Package poi defines the categories of stops the app can search for and maps
// them onto Google Places query parameters.
package poi

import (
	"math"
	"sort"
	"strings"

	gmaps "googlemaps.github.io/maps"
)

// Category describes how to search for a kind of stop.
type Category struct {
	// Label is the human-friendly name shown in output.
	Label string
	// Type is a Google Places type (https://developers.google.com/maps/documentation/places/web-service/supported_types).
	Type string
	// Keyword is a free-text term, used where no precise type exists.
	Keyword string
}

// categories maps user-facing aliases to a search Category.
var categories = map[string]Category{
	"masjid":     {Label: "Masjid", Type: "mosque"},
	"mosque":     {Label: "Masjid", Type: "mosque"},
	"restaurant": {Label: "Restaurant", Type: "restaurant"},
	"food":       {Label: "Restaurant", Type: "restaurant"},
	"toilet":     {Label: "Toilet", Keyword: "public toilet"},
	"toilets":    {Label: "Toilet", Keyword: "public toilet"},
	"wc":         {Label: "Toilet", Keyword: "public toilet"},
	"fuel":       {Label: "Petrol station", Type: "gas_station"},
	"petrol":     {Label: "Petrol station", Type: "gas_station"},
	"pharmacy":   {Label: "Pharmacy", Type: "pharmacy"},
	"chemist":    {Label: "Pharmacy", Type: "pharmacy"},
	"parking":    {Label: "Parking", Type: "parking"},
	"car_park":   {Label: "Parking", Type: "parking"},
	"cafe":       {Label: "Cafe", Type: "cafe"},
	"coffee":     {Label: "Cafe", Type: "cafe"},
	"atm":        {Label: "ATM", Type: "atm"},
	"hospital":   {Label: "Hospital", Type: "hospital"},
}

// Resolve looks up a category by alias (case-insensitive).
func Resolve(alias string) (Category, bool) {
	c, ok := categories[strings.ToLower(strings.TrimSpace(alias))]
	return c, ok
}

// Aliases returns the supported category aliases, for help text.
func Aliases() []string {
	out := make([]string, 0, len(categories))
	for k := range categories {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Option is a user-selectable category: a representative alias to send to the
// API plus its display label.
type Option struct {
	Alias string `json:"alias"`
	Label string `json:"label"`
}

// Options returns the distinct categories (deduped by label), each with a
// representative alias, sorted by label. Used to populate the frontend picker.
func Options() []Option {
	byLabel := map[string]string{} // label -> chosen alias (lexicographically smallest)
	for alias, cat := range categories {
		if cur, ok := byLabel[cat.Label]; !ok || alias < cur {
			byLabel[cat.Label] = alias
		}
	}
	out := make([]Option, 0, len(byLabel))
	for label, alias := range byLabel {
		out = append(out, Option{Alias: alias, Label: label})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Label < out[j].Label })
	return out
}

// DistanceMetres returns the great-circle distance between two points.
func DistanceMetres(a, b gmaps.LatLng) float64 {
	const earthRadius = 6371000.0 // metres
	lat1 := a.Lat * math.Pi / 180
	lat2 := b.Lat * math.Pi / 180
	dLat := (b.Lat - a.Lat) * math.Pi / 180
	dLng := (b.Lng - a.Lng) * math.Pi / 180

	h := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1)*math.Cos(lat2)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * earthRadius * math.Asin(math.Min(1, math.Sqrt(h)))
}
