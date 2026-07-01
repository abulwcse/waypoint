// Decodes a Google polyline-algorithm-encoded string into [lat, lng] pairs.
// The backend re-encodes OSRM's route geometry the same way Google's
// Directions API already does, so this one decoder serves both adapters.
// https://developers.google.com/maps/documentation/utilities/polylinealgorithm
export function decodePolyline(encoded) {
  if (!encoded) return []
  let index = 0
  let lat = 0
  let lng = 0
  const points = []

  while (index < encoded.length) {
    let shift = 0
    let result = 0
    let byte
    do {
      byte = encoded.charCodeAt(index++) - 63
      result |= (byte & 0x1f) << shift
      shift += 5
    } while (byte >= 0x20)
    lat += result & 1 ? ~(result >> 1) : result >> 1

    shift = 0
    result = 0
    do {
      byte = encoded.charCodeAt(index++) - 63
      result |= (byte & 0x1f) << shift
      shift += 5
    } while (byte >= 0x20)
    lng += result & 1 ? ~(result >> 1) : result >> 1

    points.push([lat / 1e5, lng / 1e5])
  }
  return points
}
