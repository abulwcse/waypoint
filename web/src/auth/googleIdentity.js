// Loads Google Identity Services (the "Sign in with Google" button + One Tap)
// exactly once and caches the resulting promise, mirroring
// ../maps/googleLoader.js's approach for the Maps JavaScript API.
let loadPromise = null

export function loadGoogleIdentity() {
  if (window.google?.accounts?.id) return Promise.resolve(window.google)
  if (loadPromise) return loadPromise

  loadPromise = new Promise((resolve, reject) => {
    const script = document.createElement('script')
    script.src = 'https://accounts.google.com/gsi/client'
    script.async = true
    script.defer = true
    script.onload = () => resolve(window.google)
    script.onerror = () => reject(new Error('failed to load Google Identity Services'))
    document.head.appendChild(script)
  })
  return loadPromise
}
