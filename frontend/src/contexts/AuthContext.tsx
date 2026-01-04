import React, { createContext, useContext, useEffect, useState } from 'react'
import { apiPost, apiGet, setToken as setTokenStorage, clearToken as clearTokenStorage } from '../api'
import type { User, UserAuth } from '../types'

type AuthContextValue = {
  token: string | null
  user: User | null
  loading: boolean
  login: (payload: { email: string, password: string }) => Promise<void>
  signup: (payload: { email: string, password: string, fullname: string }) => Promise<void>
  logout: () => void
  fetchMe: () => Promise<void>
}

const AuthContext = createContext<AuthContextValue | undefined>(undefined)

export function useAuth() {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used within AuthProvider')
  return ctx
}

export const AuthProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [token, setToken] = useState<string | null>(() => {
    try { return sessionStorage.getItem('auth_token') } catch { return null }
  })
  const [user, setUser] = useState<User | null>(null)
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    // if token exists on mount, try to fetch user
    if (token) {
      fetchMe().catch(() => {})
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  async function login(payload: { email: string, password: string }) {
    setLoading(true)
    try {
      const res = await apiPost('/auth/login', payload)
      if (!res.ok) throw new Error(await res.text())
      const data = await res.json()
      if (!data.token) throw new Error('no token')
      setToken(data.token)
      setTokenStorage(data.token)
      await fetchMe()
    } finally {
      setLoading(false)
    }
  }

  async function signup(payload: { email: string, password: string, fullname: string }) {
    setLoading(true)
    try {
      const res = await apiPost('/users', payload)
      if (!res.ok) throw new Error(await res.text())
      // after signup, caller can navigate to login
    } finally {
      setLoading(false)
    }
  }

  function logout() {
    setToken(null)
    setUser(null)
    clearTokenStorage()
  }

  async function fetchMe() {
    setLoading(true)
    try {
      const res = await apiGet('/users/me', { auth: true })
      if (!res.ok) throw new Error(await res.text())
      const me: User = await res.json()
      setUser(me)
      return me
    } finally {
      setLoading(false)
    }
  }

  const value: AuthContextValue = {
    token,
    user,
    loading,
    login,
    signup,
    logout,
    fetchMe,
  }

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}
