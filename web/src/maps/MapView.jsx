import OsmMap from './OsmMap.jsx'
import GoogleMap from './GoogleMap.jsx'

// MapView is the adapter: it renders Google Maps for a plan that actually
// used the pro tier (which is always Google-backed — see internal/trip.Tier),
// or when the server's free tier itself is configured for Google
// (MAPS_PROVIDER=google); otherwise the free, keyless OSM/Leaflet renderer.
// config comes from the server's /api/config (see App.jsx).
export default function MapView({ result, config }) {
  if (!config) return <div className="map map-loading">Loading map…</div>

  const usesGoogle = (result.tier === 'pro' || config.freeProvider === 'google') && config.googleMapsBrowserKey
  if (usesGoogle) {
    return <GoogleMap result={result} apiKey={config.googleMapsBrowserKey} />
  }
  return <OsmMap result={result} />
}
