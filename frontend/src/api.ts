const API_BASE = (import.meta.env.VITE_API_BASE as string) || 'http://localhost:8080'

function baseUrl(path: string) {
  return `${API_BASE.replace(/\/$/, '')}${path.startsWith('/') ? '' : '/'}${path}`
}

export function getToken(): string | null {
  return sessionStorage.getItem('auth_token')
}

export function setToken(token: string) {
  // store in sessionStorage so it is cleared when the tab/window closes
  sessionStorage.setItem('auth_token', token)
}

export function clearToken() {
  sessionStorage.removeItem('auth_token')
}

export function apiPost(path: string, body: unknown, opts: { auth?: boolean } = {}) {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  if (opts.auth) {
    const t = getToken()
    if (t) headers['Authorization'] = `Bearer ${t}`
  }
  return fetch(baseUrl(path), {
    method: 'POST',
    headers,
    body: JSON.stringify(body),
  })
}

export function apiGet(path: string, opts: { auth?: boolean } = {}) {
  const headers: Record<string, string> = {}
  if (opts.auth) {
    const t = getToken()
    if (t) headers['Authorization'] = `Bearer ${t}`
  }
  return fetch(baseUrl(path), { method: 'GET', headers })
}

export default { apiPost, apiGet, setToken, getToken, clearToken }
