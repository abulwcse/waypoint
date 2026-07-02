import { useEffect, useRef, useState } from 'react'
import { loadGoogleMaps } from './googleLoader.js'
import { decodePolyline } from './polyline.js'
import { ORIGIN_MARKER, DESTINATION_MARKER, STOP_MARKER, markerForCategory, pinDataUrl } from './markers.js'
import { placePopupHtml, stopPopupHtml } from './popups.js'

// GoogleMap renders the paid Google Maps JavaScript API — used when the
// server is configured with MAPS_PROVIDER=google (see internal/maps).
export default function GoogleMap({ result, apiKey }) {
  const containerRef = useRef(null)
  const mapRef = useRef(null)
  const overlaysRef = useRef([])
  const infoWindowRef = useRef(null)
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
    // One shared InfoWindow, reused across markers, so opening a new popup
    // closes whichever one was open before — matches Leaflet's default
    // behavior and real Google Maps' single-info-window UX.
    infoWindowRef.current = new google.InfoWindow()
  }, [google])

  useEffect(() => {
    const map = mapRef.current
    if (!google || !map || !result) return

    for (const overlay of overlaysRef.current) overlay.setMap(null)
    overlaysRef.current = []
    infoWindowRef.current.close()

    const bounds = new google.LatLngBounds()
    const icon = (marker) => ({
      url: pinDataUrl(marker),
      scaledSize: new google.Size(30, 40),
      anchor: new google.Point(15, 40),
    })
    const addMarker = (lat, lng, marker, popupHtml) => {
      const position = { lat, lng }
      const m = new google.Marker({ position, map, icon: icon(marker) })
      m.addListener('click', () => {
        infoWindowRef.current.setContent(popupHtml)
        infoWindowRef.current.open({ map, anchor: m })
      })
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
      addMarker(stop.lat, stop.lng, STOP_MARKER, stopPopupHtml(stop))
      for (const cat of stop.categories || []) {
        const marker = markerForCategory(cat.label)
        for (const place of cat.places || []) {
          addMarker(place.lat, place.lng, marker, placePopupHtml(cat.label, place))
        }
      }
    }

    if (!bounds.isEmpty()) map.fitBounds(bounds, 24)
  }, [google, result])

  if (error) return <div className="map map-error">⚠️ {error}</div>
  return <div className="map" ref={containerRef} />
}
