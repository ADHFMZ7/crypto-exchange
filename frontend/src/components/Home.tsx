import React, { useState } from 'react'
import { lightTheme } from '../ui/theme'
import Account from './Account'
import { useAuth } from '../contexts/AuthContext'

export default function Home() {
  const { user, logout } = useAuth()
  const [page, setPage] = useState<'profile'|'orders'|'wallet'|'home'>('home')

  return (
    <div style={{ display: 'grid', gap: 12 }}>
      <nav style={{ display: 'flex', gap: 8, marginBottom: 6 }} aria-label="Main menu">
        <button style={lightTheme.subtleButton} onClick={() => setPage('home')}>Home</button>
        <button style={lightTheme.subtleButton} onClick={() => setPage('profile')}>Profile</button>
        <button style={lightTheme.subtleButton} onClick={() => setPage('orders')}>Orders</button>
        <button style={lightTheme.subtleButton} onClick={() => setPage('wallet')}>Wallet</button>
        <button style={lightTheme.subtleButton} onClick={logout}>Sign out</button>
      </nav>

      <section>
        {page === 'home' && (
          <div>
            <h2 style={{ marginTop: 0 }}>Welcome{user ? `, ${user.fullname}` : ''}.</h2>
            <p style={{ color: '#64748b' }}>Use the menu to view your profile, orders, or wallet.</p>
          </div>
        )}

        {page === 'profile' && (
          <div>
            <Account />
          </div>
        )}

        {page === 'orders' && (
          <div>
            <h3>Orders</h3>
            <p style={{ color: '#64748b' }}>Orders page not implemented yet.</p>
          </div>
        )}

        {page === 'wallet' && (
          <div>
            <h3>Wallet</h3>
            <p style={{ color: '#64748b' }}>Wallet page not implemented yet.</p>
          </div>
        )}
      </section>
    </div>
  )
}
