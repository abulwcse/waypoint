// Per-category marker styling, shared by the OSM (Leaflet) and Google Maps
// adapters so a "Masjid" pin looks the same regardless of which renderer is
// active. Keyed by the category Label the API returns (see internal/poi).
const CATEGORY_MARKERS = {
  Masjid: { emoji: '🕌', color: '#2dd4bf' },
  Restaurant: { emoji: '🍽️', color: '#f59e0b' },
  Toilet: { emoji: '🚻', color: '#38bdf8' },
  'Petrol station': { emoji: '⛽', color: '#ef4444' },
  Pharmacy: { emoji: '💊', color: '#f472b6' },
  Parking: { emoji: '🅿️', color: '#a78bfa' },
  Cafe: { emoji: '☕', color: '#b45309' },
  ATM: { emoji: '🏧', color: '#22c55e' },
  Hospital: { emoji: '🏥', color: '#dc2626' },
}
const DEFAULT_PLACE_MARKER = { emoji: '📍', color: '#94a3b8' }

export const ORIGIN_MARKER = { emoji: '🟢', color: '#22c55e' }
export const DESTINATION_MARKER = { emoji: '🏁', color: '#1f2c3a' }
export const STOP_MARKER = { emoji: '⏱️', color: '#2dd4bf' }

export function markerForCategory(label) {
  return CATEGORY_MARKERS[label] || DEFAULT_PLACE_MARKER
}

// pinDataUrl renders a teardrop map pin as an inline SVG data URL, colored and
// labelled per marker kind. Used as the icon image for both Leaflet's L.icon
// and Google's google.maps.Marker, so the two adapters share one visual.
export function pinDataUrl({ emoji, color }) {
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="30" height="40" viewBox="0 0 30 40">
  <path d="M15 0C6.7 0 0 6.7 0 15c0 10.5 15 25 15 25s15-14.5 15-25C30 6.7 23.3 0 15 0z" fill="${color}" stroke="#0f1720" stroke-width="1.5"/>
  <circle cx="15" cy="15" r="10.5" fill="#0f1720" fill-opacity="0.18"/>
  <text x="15" y="20.5" font-size="15" text-anchor="middle">${emoji}</text>
</svg>`
  return `data:image/svg+xml;charset=UTF-8,${encodeURIComponent(svg)}`
}
