import React from 'react'
import { lightTheme } from '../ui/theme'
import { useAuth } from '../contexts/AuthContext'

export default function Account() {
  const { user, loading, fetchMe, logout } = useAuth()
  return (
    <section style={{ display: 'grid', gap: 12 }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <div>
            <div style={{ fontSize: 13, color: '#64748b' }}>Signed in as</div>
            <div style={{ fontWeight: 600 }}>{user?.fullname ?? '—'}</div>
            <div style={{ fontSize: 13, color: '#94a3b8' }}>{user?.email}</div>
          </div>

          <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
          <button style={lightTheme.button} onClick={fetchMe} disabled={loading}>Refresh</button>
          <button style={lightTheme.subtleButton} onClick={logout}>Sign out</button>
        </div>
      </div>

      <div style={{ fontSize: 13, color: '#475569' }}>
        Member ID: {user?.id ?? '—'}
      </div>
    </section>
  )
}
