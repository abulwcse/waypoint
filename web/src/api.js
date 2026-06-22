// Thin wrapper over the Go HTTP API. In dev these hit the Vite proxy, which
// forwards /api to the Go server.

export async function fetchTypes() {
  const res = await fetch('/api/types')
  if (!res.ok) throw new Error('could not load place types')
  return res.json()
}

export async function suggestPlaces(q) {
  const res = await fetch(`/api/suggest?q=${encodeURIComponent(q)}`)
  if (!res.ok) return []
  return res.json().catch(() => [])
}

export async function planTrip(body) {
  const res = await fetch('/api/plan', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  const data = await res.json().catch(() => ({}))
  if (!res.ok) {
    throw new Error(data.error || `request failed (${res.status})`)
  }
  return data
}
