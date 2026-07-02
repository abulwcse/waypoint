// Command server exposes the trip planner over HTTP for the React frontend.
//
// Endpoints:
//
//	GET  /api/config       → map/auth/pro-tier configuration for the frontend
//	GET  /api/types        → the selectable place categories
//	GET  /api/suggest      → place-name autocomplete
//	POST /api/plan         → plan a journey (JSON body, see planRequest)
//	POST /api/auth/google  → sign in with a Google ID token, sets a session cookie
//	GET  /api/auth/me      → the current session's identity, if signed in
//	POST /api/auth/logout  → clear the session cookie
//
// Free by default: the app runs on OSM + Open-Meteo (see internal/maps/osm,
// internal/weather) with no keys needed. If GOOGLE_MAPS_API_KEY and
// OPENWEATHERMAP_API_KEY are both set, a "pro" tier (Google Maps +
// OpenWeatherMap) becomes available to signed-in users — see internal/trip's
// Tier type. There is no real subscription/billing check: any signed-in user
// can request the pro tier, which is a deliberate demo simplification (see
// README).
//
// If ./web/dist exists, it is served as a single-page app at /.
package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/abulhassan/waypoint/internal/auth"
	"github.com/abulhassan/waypoint/internal/config"
	"github.com/abulhassan/waypoint/internal/maps"
	"github.com/abulhassan/waypoint/internal/maps/google"
	"github.com/abulhassan/waypoint/internal/poi"
	"github.com/abulhassan/waypoint/internal/trip"
	"github.com/abulhassan/waypoint/internal/weather"
	"github.com/abulhassan/waypoint/internal/weather/openweather"
)

// sessionTTL is how long a signed-in session lasts before requiring sign-in
// again.
const sessionTTL = 30 * 24 * time.Hour

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

	freeMaps, err := maps.New()
	if err != nil {
		return err
	}
	pro, photos := proTier()
	srv := &server{
		planner:        trip.New(trip.Tier{Maps: freeMaps, Weather: weather.New()}, pro),
		proAvailable:   pro != nil,
		photos:         photos,
		googleClientID: os.Getenv("GOOGLE_CLIENT_ID"),
		sessionSecret:  sessionSecret(),
	}
	if srv.googleClientID != "" {
		srv.googleVerifier = auth.NewGoogleVerifier(srv.googleClientID)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/config", srv.handleConfig)
	mux.HandleFunc("/api/auth/google", srv.handleAuthGoogle)
	mux.HandleFunc("/api/auth/me", srv.handleAuthMe)
	mux.HandleFunc("/api/auth/logout", srv.handleAuthLogout)
	mux.HandleFunc("/api/types", srv.handleTypes)
	mux.HandleFunc("/api/suggest", srv.handleSuggest)
	mux.HandleFunc("/api/plan", srv.handlePlan)
	mux.HandleFunc("/api/photo", srv.handlePhoto)
	registerStatic(mux, *staticDir)

	httpSrv := &http.Server{
		Addr:              *addr,
		Handler:           cors(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("waypoint server listening on %s", *addr)
	return httpSrv.ListenAndServe()
}

// proTier builds the pro (Google Maps + OpenWeatherMap) tier if both
// GOOGLE_MAPS_API_KEY and OPENWEATHERMAP_API_KEY are configured; otherwise it
// returns a nil tier and the server runs free-tier only. The *google.Provider
// is also returned separately so handlePhoto can proxy Places photos even
// though it isn't part of the maps.Provider interface (OSM has no photos).
func proTier() (*trip.Tier, *google.Provider) {
	mapsKey := os.Getenv("GOOGLE_MAPS_API_KEY")
	weatherKey := os.Getenv("OPENWEATHERMAP_API_KEY")
	if mapsKey == "" || weatherKey == "" {
		return nil, nil
	}
	client, err := google.New(mapsKey)
	if err != nil {
		log.Printf("pro tier disabled: %v", err)
		return nil, nil
	}
	return &trip.Tier{Maps: client, Weather: openweather.New(weatherKey)}, client
}

// sessionSecret reads SESSION_SECRET, or falls back to a random secret for
// this process's lifetime — sessions just won't survive a restart, which is
// fine for a demo deployment but worth flagging.
func sessionSecret() []byte {
	if s := os.Getenv("SESSION_SECRET"); s != "" {
		return []byte(s)
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		log.Fatalf("could not generate a session secret: %v", err)
	}
	log.Println("SESSION_SECRET not set; using a random secret for this process (signed-in sessions won't survive a restart)")
	return b
}

type server struct {
	planner        *trip.Planner
	googleVerifier *auth.GoogleVerifier // nil if GOOGLE_CLIENT_ID isn't configured
	photos         *google.Provider     // nil unless the pro tier is configured; used by handlePhoto
	sessionSecret  []byte
	googleClientID string
	proAvailable   bool
}

// mapConfig tells the frontend which map renderer the free tier uses, whether
// the pro tier (always Google-backed) is available, and the IDs/keys needed
// to load Google Identity Services and the Google Maps JavaScript API in the
// browser. googleMapsBrowserKey is deliberately a separate key from
// GOOGLE_MAPS_API_KEY (used server-side for Directions/Places) — set
// GOOGLE_MAPS_BROWSER_KEY to a key restricted by HTTP referrer.
type mapConfig struct {
	FreeProvider         string `json:"freeProvider"`
	ProAvailable         bool   `json:"proAvailable"`
	GoogleClientID       string `json:"googleClientId,omitempty"`
	GoogleMapsBrowserKey string `json:"googleMapsBrowserKey,omitempty"`
}

func (s *server) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "use GET")
		return
	}
	cfg := mapConfig{
		FreeProvider:   maps.ResolvedName(),
		ProAvailable:   s.proAvailable,
		GoogleClientID: s.googleClientID,
	}
	if cfg.FreeProvider == "google" || s.proAvailable {
		cfg.GoogleMapsBrowserKey = envOr("GOOGLE_MAPS_BROWSER_KEY", os.Getenv("GOOGLE_MAPS_API_KEY"))
	}
	writeJSON(w, http.StatusOK, cfg)
}

// authUser is what the frontend gets back for both sign-in and "who am I".
type authUser struct {
	Email   string `json:"email"`
	Name    string `json:"name"`
	Picture string `json:"picture"`
}

func (s *server) handleAuthGoogle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "use POST")
		return
	}
	if s.googleVerifier == nil {
		writeError(w, http.StatusServiceUnavailable, "sign-in is not configured on this server")
		return
	}
	var body struct {
		Credential string `json:"credential"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil || body.Credential == "" {
		writeError(w, http.StatusBadRequest, "missing credential")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	user, err := s.googleVerifier.Verify(ctx, body.Credential)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "sign-in failed: "+err.Error())
		return
	}

	sess := auth.Session{Email: user.Email, Name: user.Name, Picture: user.Picture}
	if err := auth.SetSessionCookie(w, s.sessionSecret, sess, sessionTTL, isSecure(r)); err != nil {
		writeError(w, http.StatusInternalServerError, "could not start session")
		return
	}
	writeJSON(w, http.StatusOK, authUser{Email: sess.Email, Name: sess.Name, Picture: sess.Picture})
}

func (s *server) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "use GET")
		return
	}
	sess, err := auth.ReadSession(r, s.sessionSecret)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "not signed in")
		return
	}
	writeJSON(w, http.StatusOK, authUser{Email: sess.Email, Name: sess.Name, Picture: sess.Picture})
}

func (s *server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "use POST")
		return
	}
	auth.ClearSessionCookie(w, isSecure(r))
	w.WriteHeader(http.StatusNoContent)
}

// signedIn reports whether r carries a valid session cookie. It's used to
// gate the pro tier server-side — the "pro" flag alone isn't trusted, since a
// signed-out client could otherwise spoof it to reach the paid backends.
func (s *server) signedIn(r *http.Request) bool {
	_, err := auth.ReadSession(r, s.sessionSecret)
	return err == nil
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

	pro := r.URL.Query().Get("pro") == "true" && s.signedIn(r)
	out, err := s.planner.Suggest(ctx, r.URL.Query().Get("q"), pro)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if out == nil {
		out = []string{}
	}
	writeJSON(w, http.StatusOK, out)
}

// handlePhoto proxies a Google Places photo by reference. The frontend can't
// call the Places Photo endpoint directly with GOOGLE_MAPS_BROWSER_KEY — that
// key is scoped for the Maps JavaScript API, not the Places API — so the
// server fetches it with GOOGLE_MAPS_API_KEY and streams the bytes through.
func (s *server) handlePhoto(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "use GET")
		return
	}
	if s.photos == nil {
		writeError(w, http.StatusServiceUnavailable, "photos are not configured on this server")
		return
	}
	ref := r.URL.Query().Get("ref")
	if ref == "" {
		writeError(w, http.StatusBadRequest, "missing ref")
		return
	}
	width := uint(320)
	if wq := r.URL.Query().Get("w"); wq != "" {
		if n, err := strconv.ParseUint(wq, 10, 32); err == nil && n > 0 && n <= 1600 {
			width = uint(n)
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	contentType, data, err := s.photos.Photo(ctx, ref, width)
	if err != nil {
		writeError(w, http.StatusBadGateway, "could not fetch photo: "+err.Error())
		return
	}
	defer data.Close()
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	io.Copy(w, data)
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
	Pro    bool     `json:"pro"`
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
	if req.Pro && !s.signedIn(r) {
		req.Pro = false // pro requires being signed in; silently fall back rather than erroring
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
		Pro:        body.Pro,
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

// isSecure reports whether the request arrived over HTTPS, directly or via a
// reverse proxy (Render and similar hosts terminate TLS in front of the app
// and forward this header). The session cookie's Secure flag depends on it —
// hardcoding Secure=true would make sign-in silently fail over plain-HTTP
// local development.
func isSecure(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
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
