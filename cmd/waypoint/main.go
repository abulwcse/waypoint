// Command waypoint plans stops along a driving route: given a start and end, a
// departure time, and the times you want to stop, it estimates where you'll be
// at each time and finds nearby places you choose — masjids, toilets,
// restaurants, pharmacies, parking, petrol and more.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/abulhassan/waypoint/internal/config"
	"github.com/abulhassan/waypoint/internal/maps"
	"github.com/abulhassan/waypoint/internal/poi"
	"github.com/abulhassan/waypoint/internal/trip"
	"github.com/abulhassan/waypoint/internal/weather"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		from    = flag.String("from", "", "start location (address or \"lat,lng\")")
		to      = flag.String("to", "", "end location (address or \"lat,lng\")")
		depart  = flag.String("depart", "now", "departure time: \"now\", \"HH:MM\" (today), or RFC3339")
		at      = flag.String("at", "", "comma-separated clock times to stop, e.g. \"13:15,15:30\"")
		every   = flag.Duration("every", 0, "stop on a fixed interval instead of --at, e.g. 2h")
		typesCS = flag.String("types", "masjid,toilet,restaurant", "comma-separated stop types: "+strings.Join(poi.Aliases(), ", "))
		radius  = flag.Uint("radius", 5000, "search radius around each stop, in metres")
		topN    = flag.Int("top", 3, "max results to show per type per stop")
	)
	flag.Parse()

	if *from == "" || *to == "" {
		flag.Usage()
		return fmt.Errorf("--from and --to are required")
	}

	departure, err := trip.ParseDeparture(*depart)
	if err != nil {
		return err
	}
	targets, err := trip.ParseClockTimes(splitCSV(*at), departure)
	if err != nil {
		return err
	}
	cats, err := trip.ResolveTypes(splitCSV(*typesCS))
	if err != nil {
		return err
	}

	if _, err := config.Load(); err != nil { // loads optional .env (endpoint overrides)
		return err
	}
	client, err := maps.New()
	if err != nil {
		return err
	}

	ctx := context.Background()
	free := trip.Tier{Maps: client, Weather: weather.New()}
	result, err := trip.New(free, nil).Plan(ctx, trip.Request{
		From:       *from,
		To:         *to,
		Depart:     departure,
		Targets:    targets,
		Every:      *every,
		Categories: cats,
		Radius:     *radius,
		Top:        *topN,
	})
	if err != nil {
		return err
	}

	printResult(result)
	return nil
}

func printResult(r *trip.Result) {
	fmt.Printf("Route via %s — %.0f km, ~%s\n", r.Summary, r.DistanceKm, fmtMin(r.DurationMin))
	fmt.Printf("Depart %s  →  arrive ~%s\n", r.Depart.Format("Mon 15:04"), r.Arrive.Format("Mon 15:04"))

	if len(r.Stops) == 0 {
		fmt.Println("\nNo stops fell within the trip window. Check your --at times or use --every.")
		return
	}

	for _, stop := range r.Stops {
		fmt.Printf("\n──────────────────────────────────────────────\n")
		fmt.Printf("⏱  %s  (%s into the trip)\n", stop.At.Format("15:04"), fmtMin(stop.OffsetMin))
		fmt.Printf("📍 estimated position: %.5f, %.5f\n", stop.Lat, stop.Lng)
		if w := stop.Weather; w != nil {
			fmt.Printf("%s %s, %.0f°C (feels like %.0f°C), %d%% chance of precipitation\n",
				w.Icon, w.Description, w.TempC, w.FeelsLikeC, w.PrecipPercent)
		}
		for _, cat := range stop.Categories {
			fmt.Printf("\n  %s:\n", cat.Label)
			if len(cat.Places) == 0 {
				fmt.Printf("    (none within range)\n")
				continue
			}
			for _, p := range cat.Places {
				fmt.Printf("    • %s — %.1f km away%s%s\n", p.Name, p.DistanceKm, fmtRating(p.Rating), fmtOpen(p.OpenNow))
				if p.Vicinity != "" {
					fmt.Printf("        %s\n", p.Vicinity)
				}
				fmt.Printf("        %s\n", p.MapsURL)
			}
		}
	}

	fmt.Printf("\n──────────────────────────────────────────────\n")
	fmt.Println("💡 Suggestions:")
	for _, s := range r.Suggestions {
		fmt.Printf("  • %s\n", s)
	}
}

func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return strings.Split(s, ",")
}

func fmtMin(min int) string {
	d := time.Duration(min) * time.Minute
	h := d / time.Hour
	m := (d % time.Hour) / time.Minute
	if h > 0 {
		return fmt.Sprintf("%dh%02dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

func fmtRating(r float32) string {
	if r <= 0 {
		return ""
	}
	return fmt.Sprintf("  ★ %.1f", r)
}

func fmtOpen(openNow *bool) string {
	if openNow == nil {
		return ""
	}
	if *openNow {
		return "  (open now)"
	}
	return "  (closed now)"
}
