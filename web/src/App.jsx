import { useCallback, useEffect, useId, useRef, useState } from 'react'
import { fetchMapConfig, fetchMe, fetchTypes, planTrip, signInWithGoogle, signOut, suggestPlaces } from './api.js'
import MapView from './maps/MapView.jsx'
import GoogleSignIn from './auth/GoogleSignIn.jsx'

const DEFAULT_TYPES = ['masjid', 'toilet', 'restaurant']

export default function App() {
  const [types, setTypes] = useState([])
  const [selected, setSelected] = useState(new Set(DEFAULT_TYPES))
  const [from, setFrom] = useState('')
  const [to, setTo] = useState('')
  const [depart, setDepart] = useState('now')
  const [mode, setMode] = useState('at') // 'at' | 'every'
  const [at, setAt] = useState('13:15')
  const [every, setEvery] = useState('2h')
  const [radiusMi, setRadiusMi] = useState(3)
  const [top, setTop] = useState(3)

  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [result, setResult] = useState(null)

  const [config, setConfig] = useState(null)
  const [user, setUser] = useState(null)
  const [pro, setPro] = useState(false)

  useEffect(() => {
    fetchTypes()
      .then((opts) => {
        setTypes(opts)
        // keep only defaults that the server actually offers
        setSelected((prev) => new Set(opts.filter((o) => prev.has(o.alias)).map((o) => o.alias)))
      })
      .catch((e) => setError(e.message))
  }, [])

  useEffect(() => {
    fetchMapConfig().then(setConfig).catch(() => {})
    fetchMe().then(setUser).catch(() => {})
  }, [])

  const handleCredential = useCallback((credential) => {
    signInWithGoogle(credential).then(setUser).catch((e) => setError(e.message))
  }, [])

  async function handleSignOut() {
    await signOut()
    setUser(null)
    setPro(false)
  }

  function toggleType(alias) {
    setSelected((prev) => {
      const next = new Set(prev)
      next.has(alias) ? next.delete(alias) : next.add(alias)
      return next
    })
  }

  const isPro = pro && !!user

  async function onSubmit(e) {
    e.preventDefault()
    setError('')
    setResult(null)

    if (!from || !to) {
      setError('Please enter both a start and a destination.')
      return
    }
    if (selected.size === 0) {
      setError('Pick at least one type of place to look for.')
      return
    }

    const body = {
      from,
      to,
      depart,
      types: [...selected],
      radius: Math.round(Number(radiusMi) * 1609.34), // miles → metres (server works in metres)
      top: Number(top),
      at: mode === 'at' ? at.split(',').map((s) => s.trim()).filter(Boolean) : [],
      every: mode === 'every' ? every : '',
      pro: isPro,
    }

    setLoading(true)
    try {
      setResult(await planTrip(body))
    } catch (err) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="page">
      <header className="hero">
        <div className="hero-top">
          <div>
            <h1>🧭 waypoint</h1>
            <p>Plan stops along your route — find a masjid, toilet, restaurant, pharmacy and more, right when you'll be passing.</p>
          </div>
          <AuthArea
            config={config}
            user={user}
            pro={pro}
            onProChange={setPro}
            onCredential={handleCredential}
            onSignOut={handleSignOut}
          />
        </div>
      </header>

      <form className="card form" onSubmit={onSubmit}>
        <div className="row">
          <CityInput label="From" value={from} onChange={setFrom} placeholder="Manchester, UK" pro={isPro} />
          <CityInput label="To" value={to} onChange={setTo} placeholder="London, UK" pro={isPro} />
        </div>

        <div className="row">
          <label className="field">
            <span>Depart</span>
            <input value={depart} onChange={(e) => setDepart(e.target.value)} placeholder="now or 09:00" />
          </label>

          <div className="field">
            <span>When to stop</span>
            <div className="seg">
              <button type="button" className={mode === 'at' ? 'on' : ''} onClick={() => setMode('at')}>At times</button>
              <button type="button" className={mode === 'every' ? 'on' : ''} onClick={() => setMode('every')}>Every</button>
            </div>
          </div>

          {mode === 'at' ? (
            <label className="field">
              <span>Stop times (HH:MM, comma-separated)</span>
              <input value={at} onChange={(e) => setAt(e.target.value)} placeholder="13:15, 15:30" />
            </label>
          ) : (
            <label className="field">
              <span>Interval</span>
              <input value={every} onChange={(e) => setEvery(e.target.value)} placeholder="2h" />
            </label>
          )}
        </div>

        <div className="field">
          <span>Looking for</span>
          <div className="chips">
            {types.map((opt) => (
              <button
                type="button"
                key={opt.alias}
                className={`chip ${selected.has(opt.alias) ? 'on' : ''}`}
                onClick={() => toggleType(opt.alias)}
              >
                {opt.label}
              </button>
            ))}
          </div>
        </div>

        <div className="row">
          <label className="field small">
            <span>Radius (mi)</span>
            <input type="number" min="0.1" step="0.1" value={radiusMi} onChange={(e) => setRadiusMi(e.target.value)} />
          </label>
          <label className="field small">
            <span>Results per type</span>
            <input type="number" min="1" max="10" value={top} onChange={(e) => setTop(e.target.value)} />
          </label>
          <button className="go" type="submit" disabled={loading}>
            {loading ? 'Planning…' : 'Plan my stops'}
          </button>
        </div>
      </form>

      {error && <div className="card error">⚠️ {error}</div>}

      {result && <Results result={result} config={config} />}
    </div>
  )
}

// AuthArea shows a "Sign in with Google" button when signed out, or the
// user's name/avatar plus (when the server has a pro tier configured) a Pro
// toggle when signed in. There's no real subscription check behind the
// toggle — see cmd/server/main.go's package doc.
function AuthArea({ config, user, pro, onProChange, onCredential, onSignOut }) {
  if (!config?.googleClientId) return null

  if (!user) {
    return (
      <div className="auth-area">
        <GoogleSignIn clientId={config.googleClientId} onCredential={onCredential} />
      </div>
    )
  }

  return (
    <div className="auth-area signed-in">
      {config.proAvailable && (
        <label className="pro-toggle">
          <input type="checkbox" checked={pro} onChange={(e) => onProChange(e.target.checked)} />
          <span>Pro</span>
        </label>
      )}
      {user.picture && <img className="avatar" src={user.picture} alt="" referrerPolicy="no-referrer" />}
      <span className="muted">{user.name || user.email}</span>
      <button type="button" className="link-button" onClick={onSignOut}>Sign out</button>
    </div>
  )
}

// CityInput is a text field with debounced place-name autocomplete, backed by
// /api/suggest. It renders a native <datalist> so the dropdown, filtering, and
// keyboard handling come for free.
function CityInput({ label, value, onChange, placeholder, pro }) {
  const [options, setOptions] = useState([])
  const listId = useId()
  const lastQuery = useRef('')

  useEffect(() => {
    const q = value.trim()
    // Skip short queries and the case where the field already holds a chosen
    // suggestion (avoids a redundant fetch right after selection).
    if (q.length < 3 || options.includes(value)) return
    if (q === lastQuery.current) return

    const timer = setTimeout(() => {
      lastQuery.current = q
      suggestPlaces(q, pro).then(setOptions).catch(() => {})
    }, 300)
    return () => clearTimeout(timer)
  }, [value, pro]) // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <label className="field">
      <span>{label}</span>
      <input
        list={listId}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        autoComplete="off"
      />
      <datalist id={listId}>
        {options.map((o) => <option key={o} value={o} />)}
      </datalist>
    </label>
  )
}

function Results({ result, config }) {
  return (
    <div className="results">
      <div className="card summary">
        <h2>
          {result.summary || 'Your route'}
          {result.tier === 'pro' && <span className="tier-badge">Pro</span>}
        </h2>
        <p>
          {result.distanceKm.toFixed(0)} km · ~{fmtMin(result.durationMin)} · depart{' '}
          {fmtTime(result.depart)} → arrive ~{fmtTime(result.arrive)}
        </p>
      </div>

      <div className="card map-card">
        <MapView result={result} config={config} />
      </div>

      {result.stops.length === 0 && (
        <div className="card">No stops fell within the trip window. Try other times or an interval.</div>
      )}

      {result.stops.map((stop, i) => (
        <div className="card stop" key={i}>
          <div className="stop-head">
            <span className="time">⏱ {fmtTime(stop.at)}</span>
            {stop.weather && (
              <span className="weather" title={weatherTitle(stop.weather)}>
                {stop.weather.icon} {Math.round(stop.weather.tempC)}°C
                {stop.weather.precipPercent > 0 && <> · 💧{stop.weather.precipPercent}%</>}
                {stop.weather.humidityPct != null && <> · 💦{stop.weather.humidityPct}%</>}
              </span>
            )}
            <span className="muted">{fmtMin(stop.offsetMin)} into the trip · {stop.lat.toFixed(4)}, {stop.lng.toFixed(4)}</span>
          </div>
          {stop.categories.map((cat) => (
            <div className="cat" key={cat.label}>
              <h4>{cat.label}</h4>
              {(!cat.places || cat.places.length === 0) ? (
                <p className="muted">None within range.</p>
              ) : (
                <ul>
                  {cat.places.map((p, j) => (
                    <li key={j}>
                      <a href={p.mapsUrl} target="_blank" rel="noreferrer">{p.name}</a>
                      <span className="meta">
                        {p.distanceKm.toFixed(1)} km
                        {p.rating > 0 && <> · ★ {p.rating.toFixed(1)}</>}
                        {p.openNow === true && <> · <span className="open">open</span></>}
                        {p.openNow === false && <> · <span className="closed">closed</span></>}
                      </span>
                      {p.vicinity && <div className="muted small-text">{p.vicinity}</div>}
                    </li>
                  ))}
                </ul>
              )}
            </div>
          ))}
        </div>
      ))}

      {result.suggestions?.length > 0 && (
        <div className="card tips">
          <h3>💡 Suggestions</h3>
          <ul>
            {result.suggestions.map((s, i) => <li key={i}>{s}</li>)}
          </ul>
        </div>
      )}
    </div>
  )
}

function fmtTime(iso) {
  const d = new Date(iso)
  return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}

function fmtMin(min) {
  const h = Math.floor(min / 60)
  const m = min % 60
  return h > 0 ? `${h}h${String(m).padStart(2, '0')}m` : `${m}m`
}

// weatherTitle builds the hover tooltip for a stop's weather badge. The extra
// fields (UV index, visibility) only come from the pro (OpenWeatherMap)
// provider — see internal/weather.Conditions — so they're omitted for free.
function weatherTitle(w) {
  const parts = [w.description, `feels like ${Math.round(w.feelsLikeC)}°C`, `wind ${Math.round(w.windKph)} km/h`]
  if (w.uvIndex != null) parts.push(`UV ${w.uvIndex.toFixed(1)}`)
  if (w.visibilityKm != null) parts.push(`visibility ${w.visibilityKm.toFixed(1)} km`)
  parts.push(`source: ${w.source}`)
  return parts.join(' · ')
}
