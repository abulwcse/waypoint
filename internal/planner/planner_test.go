package planner

import (
	"testing"
	"time"

	gmaps "googlemaps.github.io/maps"
)

// sampleRoute is a two-step route: 10 min from (0,0)->(0,1), then 10 min
// (0,1)->(0,2). Total 20 min, travelling east along the equator.
func sampleRoute() gmaps.Route {
	return gmaps.Route{
		Legs: []*gmaps.Leg{{
			Duration: 20 * time.Minute,
			Distance: gmaps.Distance{Meters: 2000},
			Steps: []*gmaps.Step{
				{Duration: 10 * time.Minute, StartLocation: gmaps.LatLng{Lat: 0, Lng: 0}, EndLocation: gmaps.LatLng{Lat: 0, Lng: 1}},
				{Duration: 10 * time.Minute, StartLocation: gmaps.LatLng{Lat: 0, Lng: 1}, EndLocation: gmaps.LatLng{Lat: 0, Lng: 2}},
			},
		}},
	}
}

func TestTotalDuration(t *testing.T) {
	if got := TotalDuration(sampleRoute()); got != 20*time.Minute {
		t.Fatalf("TotalDuration = %v, want 20m", got)
	}
}

func TestLocationAt(t *testing.T) {
	r := sampleRoute()
	cases := []struct {
		name    string
		offset  time.Duration
		wantLng float64
		wantOK  bool
	}{
		{"start", 0, 0, true},
		{"quarter into first step", 5 * time.Minute, 0.5, true},
		{"step boundary", 10 * time.Minute, 1, true},
		{"into second step", 15 * time.Minute, 1.5, true},
		{"end", 20 * time.Minute, 2, true},
		{"past end", 25 * time.Minute, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			loc, ok := LocationAt(r, c.offset)
			if ok != c.wantOK {
				t.Fatalf("ok = %v, want %v", ok, c.wantOK)
			}
			if ok && abs(loc.Lng-c.wantLng) > 1e-9 {
				t.Fatalf("lng = %v, want %v", loc.Lng, c.wantLng)
			}
		})
	}
}

func TestIntervalTargets(t *testing.T) {
	dep := time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC)
	got := IntervalTargets(dep, 20*time.Minute, 7*time.Minute)
	// Expect 7m and 14m offsets (21m would equal/exceed total).
	want := []time.Time{dep.Add(7 * time.Minute), dep.Add(14 * time.Minute)}
	if len(got) != len(want) {
		t.Fatalf("got %d targets, want %d", len(got), len(want))
	}
	for i := range want {
		if !got[i].Equal(want[i]) {
			t.Fatalf("target[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
