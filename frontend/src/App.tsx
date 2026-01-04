import { AuthProvider } from './contexts/AuthContext'
import { BrowserRouter, Routes, Route } from 'react-router-dom'
import AuthPage from './pages/AuthPage'
import HomePage from './pages/HomePage'
import Header from './components/Header'
import { lightTheme } from './ui/theme'

export default function App() {
  return (
    <div style={lightTheme.page}>
      <div style={lightTheme.card}>
        <AuthProvider>
          <BrowserRouter>
            <Header />
            <div style={{ padding: 16 }}>
              <Routes>
                <Route path="/" element={<HomePage />} />
                <Route path="/auth" element={<AuthPage />} />
              </Routes>
            </div>
          </BrowserRouter>
        </AuthProvider>
      </div>
    </div>
  )
}