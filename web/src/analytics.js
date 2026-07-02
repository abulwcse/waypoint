// Google Analytics (GA4 / gtag.js), loaded only when the server supplies a
// measurement ID via /api/config. With no ID configured, nothing is injected
// and every helper below is a no-op — so the app runs unchanged for anyone
// who hasn't set GA_MEASUREMENT_ID.
//
// Consent gating uses GA4 Consent Mode v2: on load we default all consent to
// "denied", so GA4 stores no cookies and buffers events until the user makes a
// choice in the banner (see ConsentBanner in App.jsx). Granting consent calls
// gtag('consent', 'update', …); the choice is remembered in localStorage so we
// only ask once.

let measurementId = null

// Where the user's accept/decline choice is remembered. Value is 'granted' or
// 'denied'; absent means we haven't asked yet (show the banner).
const CONSENT_KEY = 'waypoint.analytics-consent'

// consentDecision returns the stored choice ('granted' | 'denied') or null if
// the user hasn't decided yet. Wrapped in try/catch because localStorage can
// throw in private-mode / storage-disabled browsers.
export function consentDecision() {
  try {
    return localStorage.getItem(CONSENT_KEY)
  } catch {
    return null
  }
}

// initAnalytics injects gtag.js for the given GA4 measurement ID. It's safe to
// call more than once — the script is loaded at most once per id (App fetches
// config once, but StrictMode double-invokes effects in dev).
export function initAnalytics(id) {
  if (!id || measurementId || typeof window === 'undefined') return
  measurementId = id

  window.dataLayer = window.dataLayer || []
  // gtag pushes arguments verbatim onto dataLayer; keep the classic shape.
  function gtag() {
    window.dataLayer.push(arguments)
  }
  window.gtag = gtag

  // Consent Mode v2: default everything to denied. If the user already opted
  // in on a previous visit, honour that immediately so we don't drop data
  // while waiting for a fresh click.
  const granted = consentDecision() === 'granted'
  gtag('consent', 'default', consentState(granted))

  gtag('js', new Date())
  gtag('config', id)

  const s = document.createElement('script')
  s.async = true
  s.src = `https://www.googletagmanager.com/gtag/js?id=${encodeURIComponent(id)}`
  document.head.appendChild(s)
}

// setConsent records the user's choice and updates gtag's consent state. Call
// with true when the user accepts, false when they decline.
export function setConsent(granted) {
  try {
    localStorage.setItem(CONSENT_KEY, granted ? 'granted' : 'denied')
  } catch {
    // storage disabled — the choice just won't persist across visits.
  }
  if (!measurementId || typeof window === 'undefined' || !window.gtag) return
  window.gtag('consent', 'update', consentState(granted))
}

// consentState maps a single grant/deny into the four Consent Mode v2 signals.
// We only ever use analytics here (no ads), but GA4 expects all four to be set.
function consentState(granted) {
  const value = granted ? 'granted' : 'denied'
  return {
    ad_storage: value,
    ad_user_data: value,
    ad_personalization: value,
    analytics_storage: value,
  }
}

// trackEvent reports a GA4 event, or does nothing if analytics isn't loaded.
// Events raised before consent is granted are buffered by gtag and only sent
// (cookieless) once/if consent allows.
export function trackEvent(name, params = {}) {
  if (!measurementId || typeof window === 'undefined' || !window.gtag) return
  window.gtag('event', name, params)
}
