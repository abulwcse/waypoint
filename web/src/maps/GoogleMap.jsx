import { useEffect, useRef, useState } from 'react'
import { loadGoogleMaps } from './googleLoader.js'
import { decodePolyline } from './polyline.js'
import { ORIGIN_MARKER, DESTINATION_MARKER, STOP_MARKER, markerForCategory, pinDataUrl } from './markers.js'

// GoogleMap renders the paid Google Maps JavaScript API — used when the
// server is configured with MAPS_PROVIDER=google (see internal/maps).
export default function GoogleMap({ result, apiKey }) {
  const containerRef = useRef(null)
  const mapRef = useRef(null)
  const overlaysRef = useRef([])
  const [google, setGoogle] = useState(null)
  const [error, setError] = useState('')

  useEffect(() => {
    let cancelled = false
    loadGoogleMaps(apiKey)
      .then((maps) => { if (!cancelled) setGoogle(maps) })
      .catch((e) => { if (!cancelled) setError(e.message) })
    return () => { cancelled = true }
  }, [apiKey])

  useEffect(() => {
    if (!google || mapRef.current) return
    mapRef.current = new google.Map(containerRef.current, {
      center: { lat: 51.5, lng: -0.12 },
      zoom: 6,
      scrollwheel: false,
    })
  }, [google])

  useEffect(() => {
    const map = mapRef.current
    if (!google || !map || !result) return

    for (const overlay of overlaysRef.current) overlay.setMap(null)
    overlaysRef.current = []

    const bounds = new google.LatLngBounds()
    const icon = (marker) => ({
      url: pinDataUrl(marker),
      scaledSize: new google.Size(30, 40),
      anchor: new google.Point(15, 40),
    })
    const addMarker = (lat, lng, marker, popupHtml) => {
      const position = { lat, lng }
      const m = new google.Marker({ position, map, icon: icon(marker) })
      const info = new google.InfoWindow({ content: popupHtml })
      m.addListener('click', () => info.open({ map, anchor: m }))
      overlaysRef.current.push(m)
      bounds.extend(position)
    }

    const path = decodePolyline(result.polyline).map(([lat, lng]) => ({ lat, lng }))
    if (path.length > 1) {
      const line = new google.Polyline({ path, map, strokeColor: '#2dd4bf', strokeWeight: 4, strokeOpacity: 0.8 })
      overlaysRef.current.push(line)
      for (const p of path) bounds.extend(p)
    }

    if (result.origin) addMarker(result.origin.lat, result.origin.lng, ORIGIN_MARKER, 'Start')
    if (result.destination) addMarker(result.destination.lat, result.destination.lng, DESTINATION_MARKER, 'Destination')

    for (const stop of result.stops || []) {
      addMarker(stop.lat, stop.lng, STOP_MARKER, stopPopup(stop))
      for (const cat of stop.categories || []) {
        const marker = markerForCategory(cat.label)
        for (const place of cat.places || []) {
          addMarker(place.lat, place.lng, marker, placePopup(cat.label, place))
        }
      }
    }

    if (!bounds.isEmpty()) map.fitBounds(bounds, 24)
  }, [google, result])

  if (error) return <div className="map map-error">⚠️ {error}</div>
  return <div className="map" ref={containerRef} />
}

function placePopup(label, place) {
  const ratingHtml = place.rating > 0 ? ` · ★ ${place.rating.toFixed(1)}` : ''
  return `<strong>${escapeHtml(label)}</strong><br/><a href="${escapeAttr(place.mapsUrl)}" target="_blank" rel="noreferrer">${escapeHtml(place.name)}</a><br/>${place.distanceKm.toFixed(1)} km${ratingHtml}`
}

function stopPopup(stop) {
  const weatherHtml = stop.weather
    ? `<br/>${escapeHtml(stop.weather.icon)} ${escapeHtml(stop.weather.description)}, ${Math.round(stop.weather.tempC)}°C`
    : ''
  return `Estimated position at ${fmtTime(stop.at)}${weatherHtml}`
}

function fmtTime(iso) {
  return new Date(iso).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}

function escapeHtml(s) {
  return String(s).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]))
}

function escapeAttr(s) {
  return escapeHtml(s)
}
