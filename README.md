# waypoint

[![CI](https://github.com/abulwcse/waypoint/actions/workflows/ci.yml/badge.svg)](https://github.com/abulwcse/waypoint/actions/workflows/ci.yml)

**🌍 Live demo: https://waypoint-c72m.onrender.com/**
_(free Render instance — the first request after it's been idle may take ~30s to wake up)_

A route-aware pit-stop planner. Give it a start, an end, when you're leaving,
and the times you want to stop — it works out **where you'll be at each time**
and finds the kinds of places you choose nearby: **masjids, toilets,
restaurants, pharmacies, parking, petrol** and more.

## How it works

1. A **routing** lookup finds the best driving route and per-step durations
   (addresses are geocoded to coordinates first).
2. Given your departure time, it interpolates along the route's steps to
   estimate your position at each target time.
3. A **places** lookup finds your chosen place types around each estimated
   position, sorted by distance.

## Map providers

The actual map lookups go through a pluggable **provider** (an adapter behind a
small interface), selected by the `MAPS_PROVIDER` environment variable:

| Provider | `MAPS_PROVIDER` | Backed by | Cost |
|----------|-----------------|-----------|------|
| **OSM** (default) | `osm` | Nominatim + OSRM + Overpass ([OpenStreetMap](https://www.openstreetmap.org)) | **Free, no key** |
| **Google** | `google` | Google Maps Directions + Places APIs | Paid (key + billing) |

Each adapter lives in its own package (`internal/maps/osm`, `internal/maps/google`)
and implements `maps.Provider`; `maps.New()` picks one. Adding another backend
is just a third package implementing the same interface.

## Setup

Requires Go 1.26+. With the default (OSM) provider that's it — no key, no account.

```bash
go build -o bin/waypoint ./cmd/waypoint
```

**OSM (default):** uses the public community servers. To point at your own
(self-hosted) instances, copy `.env.example` to `.env` and set `NOMINATIM_URL`,
`OSRM_URL`, and/or `OVERPASS_URL`. Please be considerate of the free public
servers — they're shared community infrastructure, so keep request rates low.

**Google:** set `MAPS_PROVIDER=google` and `GOOGLE_MAPS_API_KEY` (a key with the
Directions API and Places API enabled, billing on). See `.env.example`.

## Usage

```bash
# Stop at specific clock times
./bin/waypoint \
  --from "Manchester, UK" \
  --to "London, UK" \
  --depart 09:00 \
  --at 11:30,13:15 \
  --types masjid,toilet,pharmacy

# Or break on a fixed interval
./bin/waypoint --from "Leeds" --to "Dover" --depart now --every 2h \
  --types parking,restaurant

# Restaurants only, wider search, more options
./bin/waypoint --from "Birmingham" --to "Cardiff" --at 12:45 \
  --types restaurant --radius 8000 --top 5
```

### Flags

| Flag       | Default                    | Meaning |
|------------|----------------------------|---------|
| `--from`   | _(required)_               | Start — address or `lat,lng` |
| `--to`     | _(required)_               | End — address or `lat,lng` |
| `--depart` | `now`                      | `now`, `HH:MM` (today), or RFC3339 |
| `--at`     | —                          | Comma-separated clock times to stop |
| `--every`  | —                          | Fixed interval instead of `--at`, e.g. `2h` |
| `--types`  | `masjid,toilet,restaurant` | See **place types** below |
| `--radius` | `5000`                     | Search radius per stop, metres |
| `--top`    | `3`                        | Max results per type per stop |

If neither `--at` nor `--every` is given, it plans one stop near the trip's
midpoint.

### Place types

`masjid`, `toilet`, `restaurant`, `cafe`, `pharmacy`, `parking`, `fuel`
(`petrol`), `atm`, `hospital`. Several aliases map to the same category
(e.g. `chemist`→pharmacy, `coffee`→cafe, `car_park`→parking).

## Web app

There's also a browser UI (React) backed by a small Go HTTP API. All map
lookups happen server-side against the OpenStreetMap services.

**1. Start the API server:**

```bash
go run ./cmd/server        # listens on :8080
```

**2. Start the frontend** (dev mode, proxies /api → :8080):

```bash
cd web
npm install
npm run dev                # open http://localhost:5173
```

For a single-process production build, build the frontend and let Go serve it:

```bash
cd web && npm run build && cd ..
go run ./cmd/server        # serves web/dist at http://localhost:8080
```

### HTTP API

| Method | Path         | Body / result |
|--------|--------------|---------------|
| `GET`  | `/api/types` | List of selectable categories `[{alias,label}]` |
| `POST` | `/api/plan`  | `{from,to,depart,at[],every,types[],radius,top}` → full plan JSON |

## Deploy (free hosting)

The server is a single process that serves both the API and the built frontend,
so it deploys as one **web service** (not a static site — it makes server-side
calls to the OSM services). A multi-stage `Dockerfile` builds the React app and
the Go binary into a small runtime image that works on any container host.

**Render (one-click, no card):**

1. Push this repo to GitHub.
2. In [Render](https://render.com): **New → Blueprint**, pick the repo. It reads
   `render.yaml`, builds the `Dockerfile`, and gives you a public URL.

   A live instance built this way runs at <https://waypoint-c72m.onrender.com/>.

The free instance sleeps after ~15 min idle (≈30s cold start). It also works
as-is on **Koyeb**, **Fly.io**, or **Google Cloud Run** — they all build the
same `Dockerfile` and inject `$PORT`, which the server reads automatically.

**Run the container locally:**

```bash
docker build -t waypoint .
docker run -p 8080:8080 waypoint   # open http://localhost:8080
```

> ⚠️ The public OSM demo servers rate-limit shared cloud IPs, so a hosted demo
> can get flaky under load. For anything heavier, self-host the services (set
> `NOMINATIM_URL` / `OSRM_URL` / `OVERPASS_URL` / `PHOTON_URL`) or use
> `MAPS_PROVIDER=google` with a key.

## Project layout

```
cmd/waypoint        CLI entry point and terminal output
cmd/server          HTTP API + serves the built frontend
web/                React (Vite) frontend
internal/config     loads GOOGLE_MAPS_API_KEY (and optional .env)
internal/maps       provider interface + factory (selects osm or google)
internal/maps/osm   OpenStreetMap adapter (Nominatim + OSRM + Overpass)
internal/maps/google Google Maps adapter (Directions + Places)
internal/planner    ETA interpolation along the route (unit-tested)
internal/poi        place-type → Places query mapping, distance helper
internal/trip       orchestration shared by the CLI and the API
```

## Notes & ideas

- Positions are estimates from typical driving times (OSRM has no live-traffic
  model), so real arrival drifts — leave a buffer around prayer times.
- OpenStreetMap place data is community-sourced; coverage and addresses vary by
  area, and there are no ratings or reliable open-now hours (unlike Google).
- Possible extensions: pull real **prayer times** (e.g. Aladhan API) and auto-target
  Dhuhr/Asr; score stops by detour off the route rather than straight-line distance;
  cache Overpass/Nominatim lookups to be kinder to the public servers.
```
