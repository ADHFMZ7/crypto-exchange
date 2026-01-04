import React from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../contexts/AuthContext'
import { lightTheme } from '../ui/theme'

export default function Header() {
  const auth = useAuth()
  const navigate = useNavigate()

  return (
    <header style={{ marginBottom: 18 }}>
      <h1 style={lightTheme.title}>Crypto Exchange</h1>
      <nav style={{ display: 'flex', gap: 8, marginTop: 8 }}>
        {!auth.user && (
          <>
            <button style={lightTheme.subtleButton} onClick={() => navigate('/auth')}>Login / Sign Up</button>
          </>
        )}

        {auth.user && (
          <button style={lightTheme.subtleButton} onClick={() => navigate('/')}>My Account</button>
        )}
      </nav>
    </header>
  )
}
