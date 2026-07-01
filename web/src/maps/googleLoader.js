// Loads the Google Maps JavaScript API exactly once and caches the resulting
// promise, so multiple mounts of GoogleMap don't inject the script twice.
let loadPromise = null

export function loadGoogleMaps(apiKey) {
  if (window.google?.maps) return Promise.resolve(window.google.maps)
  if (loadPromise) return loadPromise

  loadPromise = new Promise((resolve, reject) => {
    const callbackName = '__waypointGoogleMapsLoaded'
    window[callbackName] = () => {
      delete window[callbackName]
      resolve(window.google.maps)
    }

    const script = document.createElement('script')
    script.src = `https://maps.googleapis.com/maps/api/js?key=${encodeURIComponent(apiKey)}&callback=${callbackName}`
    script.async = true
    script.onerror = () => reject(new Error('failed to load Google Maps JavaScript API'))
    document.head.appendChild(script)
  })
  return loadPromise
}
