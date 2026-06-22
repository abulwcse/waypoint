// Command server exposes the trip planner over HTTP for the React frontend.
//
// Endpoints:
//
//	GET  /api/types  → the selectable place categories
//	POST /api/plan   → plan a journey (JSON body, see planRequest)
//
// If ./web/dist exists, it is served as a single-page app at /.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/abulhassan/waypoint/internal/config"
	"github.com/abulhassan/waypoint/internal/maps"
	"github.com/abulhassan/waypoint/internal/poi"
	"github.com/abulhassan/waypoint/internal/trip"
)

func main() {
	if err := run(); err != nil {
		log.Fatalln("error:", err)
	}
}

func run() error {
	addr := flag.String("addr", defaultAddr(), "listen address")
	staticDir := flag.String("static", "web/dist", "directory of built frontend to serve (optional)")
	flag.Parse()

	if _, err := config.Load(); err != nil { // loads optional .env (endpoint overrides)
		return err
	}
	client, err := maps.New()
	if err != nil {
		return err
	}
	srv := &server{planner: trip.New(client)}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/types", srv.handleTypes)
	mux.HandleFunc("/api/suggest", srv.handleSuggest)
	mux.HandleFunc("/api/plan", srv.handlePlan)
	registerStatic(mux, *staticDir)

	httpSrv := &http.Server{
		Addr:              *addr,
		Handler:           cors(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("waypoint server listening on %s", *addr)
	return httpSrv.ListenAndServe()
}

type server struct {
	planner *trip.Planner
}

func (s *server) handleTypes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "use GET")
		return
	}
	writeJSON(w, http.StatusOK, poi.Options())
}

func (s *server) handleSuggest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "use GET")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	out, err := s.planner.Suggest(ctx, r.URL.Query().Get("q"))
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if out == nil {
		out = []string{}
	}
	writeJSON(w, http.StatusOK, out)
}

type planRequest struct {
	From   string   `json:"from"`
	To     string   `json:"to"`
	Depart string   `json:"depart"`
	At     []string `json:"at"`
	Every  string   `json:"every"`
	Types  []string `json:"types"`
	Radius uint     `json:"radius"`
	Top    int      `json:"top"`
}

func (s *server) handlePlan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "use POST")
		return
	}
	var body planRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	req, err := buildRequest(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	result, err := s.planner.Plan(ctx, req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func buildRequest(body planRequest) (trip.Request, error) {
	depart, err := trip.ParseDeparture(body.Depart)
	if err != nil {
		return trip.Request{}, err
	}
	targets, err := trip.ParseClockTimes(body.At, depart)
	if err != nil {
		return trip.Request{}, err
	}
	cats, err := trip.ResolveTypes(body.Types)
	if err != nil {
		return trip.Request{}, err
	}
	var every time.Duration
	if body.Every != "" {
		every, err = time.ParseDuration(body.Every)
		if err != nil {
			return trip.Request{}, fmt.Errorf("invalid interval %q: %w", body.Every, err)
		}
	}
	return trip.Request{
		From:       body.From,
		To:         body.To,
		Depart:     depart,
		Targets:    targets,
		Every:      every,
		Categories: cats,
		Radius:     body.Radius,
		Top:        body.Top,
	}, nil
}

// registerStatic serves a built SPA from dir if it exists, with index.html as
// the fallback for client-side routes.
func registerStatic(mux *http.ServeMux, dir string) {
	index := filepath.Join(dir, "index.html")
	if _, err := os.Stat(index); errors.Is(err, os.ErrNotExist) {
		return
	}
	fs := http.FileServer(http.Dir(dir))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if _, err := os.Stat(filepath.Join(dir, filepath.Clean(r.URL.Path))); errors.Is(err, os.ErrNotExist) {
			http.ServeFile(w, r, index) // SPA fallback
			return
		}
		fs.ServeHTTP(w, r)
	})
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// defaultAddr picks the listen address. Most hosts (Render, Cloud Run, Koyeb)
// inject the port to bind via $PORT; fall back to $ADDR, then :8080 for local.
func defaultAddr() string {
	if p := os.Getenv("PORT"); p != "" {
		return ":" + p
	}
	return envOr("ADDR", ":8080")
}