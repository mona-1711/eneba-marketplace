import { useState } from 'react'
import { Link, Navigate, useLocation, useNavigate, useSearchParams } from 'react-router-dom'
import { useAuth } from '../lib/auth'
import { Logo } from '../components/Layout'

function safeNext(next: string | null) {
  return next && next.startsWith('/') && !next.startsWith('//') ? next : '/'
}

export function AuthPage({ mode }: { mode: 'login' | 'register' }) {
  const { user, login, register } = useAuth()
  const [params] = useSearchParams()
  const navigate = useNavigate()
  const next = safeNext(params.get('next'))
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [name, setName] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  if (user) return <Navigate to={next} replace />

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError(null)
    setBusy(true)
    try {
      if (mode === 'login') await login(email, password)
      else await register(email, password, name)
      navigate(next, { replace: true })
    } catch (err) {
      setError((err as Error).message)
      setBusy(false)
    }
  }

  const other = mode === 'login' ? '/register' : '/login'
  return (
    <div className="auth">
      <form className="auth-card" onSubmit={submit}>
        <Logo />
        <h1>{mode === 'login' ? 'Sign in' : 'Create account'}</h1>
        {next === '/checkout' && <p className="muted">Sign in to finish your purchase. Your cart is saved.</p>}
        {mode === 'register' && (
          <label className="field">
            <span>Display name</span>
            <input value={name} onChange={(e) => setName(e.target.value)} autoComplete="nickname" />
          </label>
        )}
        <label className="field">
          <span>Email</span>
          <input type="email" value={email} onChange={(e) => setEmail(e.target.value)} autoComplete="email" required />
        </label>
        <label className="field">
          <span>Password</span>
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete={mode === 'login' ? 'current-password' : 'new-password'}
            minLength={mode === 'register' ? 8 : undefined}
            required
          />
          {mode === 'register' && <small className="muted">At least 8 characters</small>}
        </label>
        {error && <div className="alert">{error}</div>}
        <button className="btn btn-primary btn-lg btn-block" disabled={busy}>
          {busy ? 'Please wait…' : mode === 'login' ? 'Sign in' : 'Create account'}
        </button>
        <p className="muted center">
          {mode === 'login' ? "Don't have an account? " : 'Already have an account? '}
          <Link to={`${other}?next=${encodeURIComponent(next)}`}>{mode === 'login' ? 'Create one' : 'Sign in'}</Link>
        </p>
      </form>
    </div>
  )
}

// Wraps routes that need a signed-in user (checkout, orders).
export function RequireAuth({ children }: { children: JSX.Element }) {
  const { user, loading } = useAuth()
  const loc = useLocation()
  if (loading) return <div className="container page-msg">Loading…</div>
  if (!user) return <Navigate to={`/login?next=${encodeURIComponent(loc.pathname + loc.search)}`} replace />
  return children
}
