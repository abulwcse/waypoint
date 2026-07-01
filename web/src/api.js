// Thin wrapper over the Go HTTP API. In dev these hit the Vite proxy, which
// forwards /api to the Go server.

export async function fetchMapConfig() {
  const res = await fetch('/api/config')
  if (!res.ok) throw new Error('could not load map config')
  return res.json()
}

export async function fetchTypes() {
  const res = await fetch('/api/types')
  if (!res.ok) throw new Error('could not load place types')
  return res.json()
}

export async function suggestPlaces(q, pro = false) {
  const res = await fetch(`/api/suggest?q=${encodeURIComponent(q)}&pro=${pro}`)
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

// --- auth: Google sign-in session, kept in an HTTP-only cookie server-side ---

// fetchMe resolves to the signed-in user, or null if there's no session — a
// 401 here is a normal "not signed in" state, not an error to surface.
export async function fetchMe() {
  const res = await fetch('/api/auth/me')
  if (!res.ok) return null
  return res.json()
}

export async function signInWithGoogle(credential) {
  const res = await fetch('/api/auth/google', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ credential }),
  })
  const data = await res.json().catch(() => ({}))
  if (!res.ok) {
    throw new Error(data.error || `sign-in failed (${res.status})`)
  }
  return data
}

export async function signOut() {
  await fetch('/api/auth/logout', { method: 'POST' })
}
