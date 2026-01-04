import React, { useState, useEffect } from 'react'
import AuthForm from '../components/AuthForm'
import { useAuth } from '../contexts/AuthContext'
import type { UserAuth } from '../types'
import { useNavigate } from 'react-router-dom'

export default function AuthPage() {
  const auth = useAuth()
  const navigate = useNavigate()
  const [mode, setMode] = useState<'login'|'signup'>('login')
  const [email, setEmail] = useState('')
  const [fullname, setFullname] = useState('')
  const [password, setPassword] = useState('')

  async function handleSubmit(e?: React.FormEvent) {
    e?.preventDefault()
    if (mode === 'login') {
      await auth.login({ email, password })
      navigate('/')
    } else {
      await auth.signup({ email, password, fullname })
      setMode('login')
    }
  }

  useEffect(() => {
    if (!auth.loading && auth.user) {
      navigate('/')
    }
  }, [auth.loading, auth.user, navigate])

  return (
    <div>
      <AuthForm
        mode={mode}
        email={email}
        setEmail={setEmail}
        fullname={fullname}
        setFullname={setFullname}
        password={password}
        setPassword={setPassword}
        loading={auth.loading}
        onSubmit={handleSubmit}
        onSwitch={(m) => setMode(m)}
      />
    </div>
  )
}
