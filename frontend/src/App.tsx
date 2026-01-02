import { useState, useEffect } from 'react'
import './App.css'
import { apiPost, apiGet, setToken, getToken, clearToken } from './api'
import type { UserAuth, User } from './types'

function App() {
  const [mode, setMode] = useState<'login' | 'signup' | 'me'>('login')

  const [email, setEmail] = useState('')
  const [fullname, setFullname] = useState('')
  const [password, setPassword] = useState('')

  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [user, setUser] = useState<User | null>(null)

  useEffect(() => {
    const token = getToken()
    if (token) setMode('me')
  }, [])

  const clearForm = () => {
    setEmail('')
    setFullname('')
    setPassword('')
    setError(null)
  }

  async function handleSignup(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    if (!email || !password || !fullname) {
      setError('Please provide fullname, email and password')
      return
    }
    setLoading(true)
    try {
      const payload: UserAuth = { email, password, fullname }
      const res = await apiPost('/users', payload)
      if (!res.ok) throw new Error(await res.text())
      clearForm()
      setMode('login')
    } catch (err: any) {
      setError(err.message || 'Signup failed')
    } finally {
      setLoading(false)
    }
  }

  async function handleLogin(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    if (!email || !password) {
      setError('Please provide email and password')
      return
    }
    setLoading(true)
    try {
      const payload: UserAuth = { email, password }
      const res = await apiPost('/auth/login', payload)
      if (!res.ok) throw new Error(await res.text())
      const data = await res.json()
      if (data.token) {
        setToken(data.token)
        setMode('me')
        await fetchMe()
      } else {
        throw new Error('No token returned')
      }
    } catch (err: any) {
      setError(err.message || 'Login failed')
    } finally {
      setLoading(false)
    }
  }

  async function fetchMe() {
    setError(null)
    setLoading(true)
    try {
      const res = await apiGet('/users/me', { auth: true })
      if (!res.ok) throw new Error(await res.text())
      const me: User = await res.json()
      setUser(me)
    } catch (err: any) {
      setError(err.message || 'Failed to fetch user')
      clearToken()
      setMode('login')
    } finally {
      setLoading(false)
    }
  }

  function handleLogout() {
    clearToken()
    setUser(null)
    setMode('login')
    clearForm()
  }

  return (
    <div className="app-root">
      <header>
        <h1>Crypto Exchange — Auth</h1>
        <nav>
          <button onClick={() => { setMode('login'); clearForm(); }}>Login</button>
          <button onClick={() => { setMode('signup'); clearForm(); }}>Sign Up</button>
          <button onClick={() => { setMode('me'); fetchMe(); }}>My Account</button>
        </nav>
      </header>

      <main>
        {error && <div className="error">{error}</div>}

        {mode === 'signup' && (
          <form onSubmit={handleSignup} className="form">
            <h2>Create account</h2>
            <label>
              Full name
              <input value={fullname} onChange={e => setFullname(e.target.value)} />
            </label>
            <label>
              Email
              <input type="email" value={email} onChange={e => setEmail(e.target.value)} />
            </label>
            <label>
              Password
              <input type="password" value={password} onChange={e => setPassword(e.target.value)} />
            </label>
            <button type="submit" disabled={loading}>{loading ? 'Creating…' : 'Create account'}</button>
          </form>
        )}

        {mode === 'login' && (
          <form onSubmit={handleLogin} className="form">
            <h2>Sign in</h2>
            <label>
              Email
              <input type="email" value={email} onChange={e => setEmail(e.target.value)} />
            </label>
            <label>
              Password
              <input type="password" value={password} onChange={e => setPassword(e.target.value)} />
            </label>
            <button type="submit" disabled={loading}>{loading ? 'Signing in…' : 'Sign in'}</button>
          </form>
        )}

        {mode === 'me' && (
          <section className="me">
            <h2>Account</h2>
            {loading && <div>Loading…</div>}
            {user ? (
              <div>
                <p><strong>ID:</strong> {user.id}</p>
                <p><strong>Full name:</strong> {user.fullname}</p>
                <p><strong>Email:</strong> {user.email}</p>
                <button onClick={handleLogout}>Sign out</button>
              </div>
            ) : (
              <div>
                <p>No user loaded.</p>
                <button onClick={() => fetchMe()}>Fetch me</button>
              </div>
            )}
          </section>
        )}
      </main>

      <footer>
        <small>Dev mode — tokens stored in sessionStorage for the session only.</small>
      </footer>
    </div>
  )
}

export default App
