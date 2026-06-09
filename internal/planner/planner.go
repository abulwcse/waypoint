// Package planner turns a route + departure time into "where will I be at
// time T" estimates, which the app uses as search centres for nearby places.
package planner

import (
	"time"

	gmaps "googlemaps.github.io/maps"
)

// Stop is a planned point along the route at a particular clock time.
type Stop struct {
	// At is the clock time the traveller is estimated to reach Location.
	At time.Time
	// Offset is how long after departure that is.
	Offset time.Duration
	// Location is the estimated position on the route at that time.
	Location gmaps.LatLng
}

// TotalDuration sums the travel time across all legs of the route.
func TotalDuration(route gmaps.Route) time.Duration {
	var total time.Duration
	for _, leg := range route.Legs {
		total += leg.Duration
	}
	return total
}

// LocationAt estimates the position on the route at the given offset from
// departure. It walks the route's steps accumulating each step's duration and
// linearly interpolates within the step the offset falls in. The bool is false
// if the offset is past the end of the route.
func LocationAt(route gmaps.Route, offset time.Duration) (gmaps.LatLng, bool) {
	if offset <= 0 {
		if loc, ok := firstLocation(route); ok {
			return loc, true
		}
		return gmaps.LatLng{}, false
	}

	var acc time.Duration
	for _, leg := range route.Legs {
		for _, step := range leg.Steps {
			if acc+step.Duration >= offset {
				frac := 0.0
				if step.Duration > 0 {
					frac = float64(offset-acc) / float64(step.Duration)
				}
				return interpolate(step.StartLocation, step.EndLocation, frac), true
			}
			acc += step.Duration
		}
	}
	return gmaps.LatLng{}, false
}

// Plan builds the ordered list of stops for the given target times, dropping
// any time that falls before departure or after arrival.
func Plan(route gmaps.Route, departure time.Time, targets []time.Time) []Stop {
	stops := make([]Stop, 0, len(targets))
	for _, t := range targets {
		offset := t.Sub(departure)
		loc, ok := LocationAt(route, offset)
		if !ok {
			continue
		}
		stops = append(stops, Stop{At: t, Offset: offset, Location: loc})
	}
	return stops
}

// IntervalTargets generates clock times every `every` from departure up to (and
// not exceeding) arrival = departure + total.
func IntervalTargets(departure time.Time, total, every time.Duration) []time.Time {
	if every <= 0 {
		return nil
	}
	var out []time.Time
	for d := every; d < total; d += every {
		out = append(out, departure.Add(d))
	}
	return out
}

func firstLocation(route gmaps.Route) (gmaps.LatLng, bool) {
	for _, leg := range route.Legs {
		for _, step := range leg.Steps {
			return step.StartLocation, true
		}
	}
	return gmaps.LatLng{}, false
}

func interpolate(a, b gmaps.LatLng, frac float64) gmaps.LatLng {
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	return gmaps.LatLng{
		Lat: a.Lat + (b.Lat-a.Lat)*frac,
		Lng: a.Lng + (b.Lng-a.Lng)*frac,
	}
}
