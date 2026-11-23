import { useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api, asset, formatPrice, type QuotedLine } from '../lib/api'
import { useAuth } from '../lib/auth'
import { useCart } from '../lib/cart'

export function useQuote() {
  const { items } = useCart()
  const [quote, setQuote] = useState<{ items: QuotedLine[]; totalCents: number } | null>(null)
  const [error, setError] = useState<string | null>(null)
  const key = JSON.stringify(items)
  useEffect(() => {
    let cancelled = false
    api<{ items: QuotedLine[]; totalCents: number }>('/api/cart/quote', { method: 'POST', json: { items } })
      .then((q) => !cancelled && setQuote(q))
      .catch((e: Error) => !cancelled && setError(e.message))
    return () => {
      cancelled = true
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key])
  return { quote, error }
}

export default function Cart() {
  const { items, setQuantity, remove } = useCart()
  const { user } = useAuth()
  const { quote, error } = useQuote()
  const navigate = useNavigate()

  if (items.length === 0)
    return (
      <div className="container empty">
        <h2>Your cart is empty</h2>
        <p>Find something you'll love in the store.</p>
        <Link to="/store" className="btn btn-primary">
          Browse products
        </Link>
      </div>
    )

  const unavailable = quote?.items.some((l) => !l.available)

  return (
    <div className="container cart-page">
      <h1>Cart</h1>
      {error && <div className="alert">{error}</div>}
      <div className="cart-layout">
        <div className="cart-lines">
          {(quote?.items ?? []).map((l) => (
            <div key={l.offerId} className={`cart-line ${l.available ? '' : 'unavailable'}`}>
              <img src={asset(l.coverUrl)} alt="" />
              <div className="cart-line-info">
                <Link to={`/product/${l.productSlug}`}>{l.productName}</Link>
                <span className="muted small">Seller: {l.sellerName}</span>
                {!l.available && <span className="warn small">{l.stock === 0 ? 'Sold out' : `Only ${l.stock} available`}</span>}
              </div>
              <div className="qty">
                <button onClick={() => setQuantity(l.offerId, l.quantity - 1)} aria-label="Decrease">
                  −
                </button>
                <span>{l.quantity}</span>
                <button onClick={() => setQuantity(l.offerId, l.quantity + 1)} disabled={l.quantity >= Math.min(10, l.stock)} aria-label="Increase">
                  +
                </button>
              </div>
              <div className="cart-line-price">{formatPrice(l.priceCents * l.quantity)}</div>
              <button className="link-btn" onClick={() => remove(l.offerId)}>
                Remove
              </button>
            </div>
          ))}
        </div>
        <aside className="summary">
          <h2>Summary</h2>
          <div className="summary-row">
            <span>Items</span>
            <span>{items.reduce((n, i) => n + i.quantity, 0)}</span>
          </div>
          <div className="summary-row total">
            <span>Total</span>
            <span>{formatPrice(quote?.totalCents ?? 0)}</span>
          </div>
          <button className="btn btn-primary btn-lg btn-block" disabled={!quote || unavailable} onClick={() => navigate(user ? '/checkout' : '/login?next=/checkout')}>
            {user ? 'Proceed to checkout' : 'Sign in to checkout'}
          </button>
          {unavailable && <p className="warn small">Adjust unavailable items to continue.</p>}
        </aside>
      </div>
    </div>
  )
}
