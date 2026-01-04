import { useEffect } from 'react'
import Home from '../components/Home'
import { useAuth } from '../contexts/AuthContext'
import { useNavigate } from 'react-router-dom'

export default function HomePage() {
  const auth = useAuth()
  const navigate = useNavigate()

  useEffect(() => {
    if (!auth.loading && !auth.user) {
      navigate('/auth')
    }
  }, [auth.loading, auth.user, navigate])

  return <Home />
}
