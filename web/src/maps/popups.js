// Shared popup markup for both map renderers (OsmMap and GoogleMap), so a
// place card looks the same regardless of which one is active.

export function placePopupHtml(label, place) {
  const ratingHtml = place.rating > 0
    ? `★ ${place.rating.toFixed(1)}${place.reviewCount ? ` (${place.reviewCount})` : ''}`
    : ''
  const openHtml = place.openNow === true
    ? '<span class="popup-open">Open now</span>'
    : place.openNow === false
      ? '<span class="popup-closed">Closed</span>'
      : ''
  const metaParts = [ratingHtml, openHtml].filter(Boolean).join(' · ')

  return `<div class="popup-card">
    <div class="popup-body">
      <div class="popup-label">${escapeHtml(label)}</div>
      <div class="popup-name">${escapeHtml(place.name)}</div>
      ${place.vicinity ? `<div class="popup-address">${escapeHtml(place.vicinity)}</div>` : ''}
      ${metaParts ? `<div class="popup-meta">${metaParts}</div>` : ''}
      <div class="popup-meta">${place.distanceKm.toFixed(1)} km away</div>
      <a class="popup-link" href="${escapeAttr(place.mapsUrl)}" target="_blank" rel="noreferrer">View on Google Maps →</a>
    </div>
  </div>`
}

export function stopPopupHtml(stop) {
  const weatherHtml = stop.weather
    ? `<br/>${escapeHtml(stop.weather.icon)} ${escapeHtml(stop.weather.description)}, ${Math.round(stop.weather.tempC)}°C`
    : ''
  return `<div class="popup-card"><div class="popup-body popup-meta">Estimated position at ${fmtTime(stop.at)}${weatherHtml}</div></div>`
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
