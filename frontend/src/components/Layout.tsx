import { useEffect, useState } from 'react'
import { Link, NavLink, Outlet, useLocation } from 'react-router-dom'
import { useAuth } from '../lib/auth'
import { useCart } from '../lib/cart'
import { CartIcon, UserIcon, BoltIcon, ShieldIcon, GlobeIcon } from './Icons'
import SearchBox from './SearchBox'

export function Logo() {
  return (
    <Link to="/" className="logo" aria-label="keyport home">
      <span className="logo-mark">k</span>
      <span className="logo-word">keyport</span>
    </Link>
  )
}

function AccountMenu() {
  const { user, logout } = useAuth()
  const [open, setOpen] = useState(false)
  const loc = useLocation()
  useEffect(() => {
    setOpen(false)
  }, [loc.pathname])

  if (!user)
    return (
      <Link to={`/login?next=${encodeURIComponent(loc.pathname + loc.search)}`} className="header-btn">
        <UserIcon />
        <span className="hide-sm">Sign in</span>
      </Link>
    )
  return (
    <div className="account">
      <button className="header-btn" onClick={() => setOpen((o) => !o)} aria-expanded={open}>
        <span className="avatar">{user.name.slice(0, 1).toUpperCase()}</span>
        <span className="hide-sm">{user.name}</span>
      </button>
      {open && (
        <div className="account-menu">
          <div className="account-email">{user.email}</div>
          <Link to="/orders">My orders & keys</Link>
          <button onClick={() => logout()}>Sign out</button>
        </div>
      )}
    </div>
  )
}

export default function Layout() {
  const { count, lastAdded } = useCart()
  const [bump, setBump] = useState(false)
  const loc = useLocation()

  useEffect(() => {
    if (!lastAdded) return
    setBump(true)
    const t = setTimeout(() => setBump(false), 400)
    return () => clearTimeout(t)
  }, [lastAdded])

  useEffect(() => {
    window.scrollTo(0, 0)
  }, [loc.pathname])

  return (
    <>
      <header className="header">
        <div className="container header-row">
          <Logo />
          <SearchBox />
          <nav className="header-actions">
            <AccountMenu />
            <Link to="/cart" className={`header-btn cart-btn ${bump ? 'bump' : ''}`} aria-label={`Cart, ${count} items`}>
              <CartIcon />
              {count > 0 && <span className="badge">{count}</span>}
            </Link>
          </nav>
        </div>
        <div className="subnav">
          <div className="container subnav-row">
            <NavLink to="/store" end>All products</NavLink>
            <NavLink to="/store?platform=Steam">Steam</NavLink>
            <NavLink to="/store?platform=Xbox+Live">Xbox</NavLink>
            <NavLink to="/store?platform=PSN">PlayStation</NavLink>
            <NavLink to="/store?platform=Nintendo+eShop">Nintendo</NavLink>
            <NavLink to="/store?type=Gift+Card">Gift cards</NavLink>
            <NavLink to="/store?type=Subscription">Subscriptions</NavLink>
            <NavLink to="/store?sort=discount">Deals</NavLink>
          </div>
        </div>
      </header>
      <main className="main">
        <Outlet />
      </main>
      <section className="trust">
        <div className="container trust-row">
          <div>
            <BoltIcon /> <span><strong>Instant delivery</strong> Keys appear right after payment</span>
          </div>
          <div>
            <ShieldIcon /> <span><strong>Buyer protection</strong> Every purchase is covered</span>
          </div>
          <div>
            <GlobeIcon /> <span><strong>Region info</strong> Always shown before you buy</span>
          </div>
        </div>
      </section>
      <footer className="footer">
        <div className="container footer-row">
          <Logo />
          <p>
            A demo marketplace built for the Eneba internship assignment. Payments are simulated, keys are fake, no real
            money is involved. Game names belong to their respective owners.
          </p>
        </div>
      </footer>
    </>
  )
}
