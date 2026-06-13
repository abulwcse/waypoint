// Package config loads runtime configuration from the environment.
package config

import (
	"bufio"
	"os"
	"strings"
)

// Config holds the settings the app needs to run. The app uses free, keyless
// OpenStreetMap services, so there is nothing required here — the optional
// NOMINATIM_URL / OSRM_URL / OVERPASS_URL overrides are read directly by the
// maps package from the environment.
type Config struct{}

// Load reads configuration from the environment. If a .env file is present in
// the working directory, its values are loaded first (without overriding any
// variable already set in the real environment). This is where optional
// endpoint overrides can live.
func Load() (*Config, error) {
	loadDotEnv(".env")
	return &Config{}, nil
}

// loadDotEnv does a minimal parse of a KEY=VALUE file. Lines starting with '#'
// and blank lines are ignored. Missing file is not an error.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.Trim(strings.TrimSpace(val), `"'`)
		if _, exists := os.LookupEnv(key); !exists {
			os.Setenv(key, val)
		}
	}
}
