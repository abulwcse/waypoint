import { useEffect, useId, useRef, useState } from 'react'
import { fetchTypes, planTrip, suggestPlaces } from './api.js'

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

  useEffect(() => {
    fetchTypes()
      .then((opts) => {
        setTypes(opts)
        // keep only defaults that the server actually offers
        setSelected((prev) => new Set(opts.filter((o) => prev.has(o.alias)).map((o) => o.alias)))
      })
      .catch((e) => setError(e.message))
  }, [])

  function toggleType(alias) {
    setSelected((prev) => {
      const next = new Set(prev)
      next.has(alias) ? next.delete(alias) : next.add(alias)
      return next
    })
  }

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
        <h1>🧭 waypoint</h1>
        <p>Plan stops along your route — find a masjid, toilet, restaurant, pharmacy and more, right when you'll be passing.</p>
      </header>

      <form className="card form" onSubmit={onSubmit}>
        <div className="row">
          <CityInput label="From" value={from} onChange={setFrom} placeholder="Manchester, UK" />
          <CityInput label="To" value={to} onChange={setTo} placeholder="London, UK" />
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

      {result && <Results result={result} />}
    </div>
  )
}

// CityInput is a text field with debounced place-name autocomplete, backed by
// /api/suggest. It renders a native <datalist> so the dropdown, filtering, and
// keyboard handling come for free.
function CityInput({ label, value, onChange, placeholder }) {
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
      suggestPlaces(q).then(setOptions).catch(() => {})
    }, 300)
    return () => clearTimeout(timer)
  }, [value]) // eslint-disable-line react-hooks/exhaustive-deps

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

function Results({ result }) {
  return (
    <div className="results">
      <div className="card summary">
        <h2>{result.summary || 'Your route'}</h2>
        <p>
          {result.distanceKm.toFixed(0)} km · ~{fmtMin(result.durationMin)} · depart{' '}
          {fmtTime(result.depart)} → arrive ~{fmtTime(result.arrive)}
        </p>
      </div>

      {result.stops.length === 0 && (
        <div className="card">No stops fell within the trip window. Try other times or an interval.</div>
      )}

      {result.stops.map((stop, i) => (
        <div className="card stop" key={i}>
          <div className="stop-head">
            <span className="time">⏱ {fmtTime(stop.at)}</span>
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
