import { useEffect, useRef } from 'react'
import L from 'leaflet'
import 'leaflet/dist/leaflet.css'
import { decodePolyline } from './polyline.js'
import { ORIGIN_MARKER, DESTINATION_MARKER, STOP_MARKER, markerForCategory, pinDataUrl } from './markers.js'

function pinIcon(marker) {
  return L.icon({
    iconUrl: pinDataUrl(marker),
    iconSize: [30, 40],
    iconAnchor: [15, 40],
    popupAnchor: [0, -36],
  })
}

// OsmMap renders the free, keyless OpenStreetMap tile stack via Leaflet —
// the default adapter (see internal/maps: MAPS_PROVIDER=osm).
export default function OsmMap({ result }) {
  const containerRef = useRef(null)
  const mapRef = useRef(null)
  const layerRef = useRef(null)

  useEffect(() => {
    const map = L.map(containerRef.current, { scrollWheelZoom: false })
    L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
      attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors',
      maxZoom: 19,
    }).addTo(map)
    mapRef.current = map
    layerRef.current = L.layerGroup().addTo(map)
    return () => map.remove()
  }, [])

  useEffect(() => {
    const map = mapRef.current
    const layer = layerRef.current
    if (!map || !layer || !result) return
    layer.clearLayers()

    const bounds = []
    const addMarker = (lat, lng, marker, popupHtml) => {
      L.marker([lat, lng], { icon: pinIcon(marker) }).bindPopup(popupHtml).addTo(layer)
      bounds.push([lat, lng])
    }

    const path = decodePolyline(result.polyline)
    if (path.length > 1) {
      L.polyline(path, { color: '#2dd4bf', weight: 4, opacity: 0.8 }).addTo(layer)
      bounds.push(...path)
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

    if (bounds.length > 0) map.fitBounds(bounds, { padding: [24, 24] })
  }, [result])

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
