import React from 'react'
import { lightTheme } from '../ui/theme'
import type { UserAuth } from '../types'

type Props = {
  mode: 'login'|'signup'
  email: string
  setEmail: (s: string) => void
  fullname: string
  setFullname: (s: string) => void
  password: string
  setPassword: (s: string) => void
  loading: boolean
  onSubmit: (e?: React.FormEvent) => void
  onSwitch: (mode: 'login'|'signup') => void
}

export default function AuthForm({ mode, email, setEmail, fullname, setFullname, password, setPassword, loading, onSubmit, onSwitch }: Props) {
  return (
    <form style={lightTheme.form} onSubmit={onSubmit}>
      {mode === 'signup' ? <h2 style={{ margin: 0 }}>Create account</h2> : <h2 style={{ margin: 0 }}>Sign in</h2>}

      {mode === 'signup' && (
        <label>
          <div style={{ fontSize: 13, marginBottom: 6 }}>Full name</div>
          <input style={lightTheme.input} value={fullname} onChange={e => setFullname(e.target.value)} required placeholder="Jane Doe" />
        </label>
      )}

      <label>
        <div style={{ fontSize: 13, marginBottom: 6 }}>Email</div>
        <input style={lightTheme.input} type="email" value={email} onChange={e => setEmail(e.target.value)} required placeholder="you@example.com" />
      </label>

      <label>
        <div style={{ fontSize: 13, marginBottom: 6 }}>Password</div>
        <input style={lightTheme.input} type="password" value={password} onChange={e => setPassword(e.target.value)} required placeholder="••••••••" />
      </label>

      <div style={{ display: 'flex', gap: 8 }}>
        <button style={lightTheme.button} disabled={loading}>{loading ? (mode === 'signup' ? 'Creating…' : 'Signing in…') : (mode === 'signup' ? 'Create account' : 'Sign in')}</button>
        <button type="button" style={lightTheme.subtleButton} onClick={() => onSwitch(mode === 'signup' ? 'login' : 'signup')}>{mode === 'signup' ? 'Back' : 'Create account'}</button>
      </div>
    </form>
  )
}
