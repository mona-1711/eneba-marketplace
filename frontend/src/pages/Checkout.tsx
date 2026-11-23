import { useMemo, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api, ApiError, formatPrice, type Order } from '../lib/api'
import { useCart } from '../lib/cart'
import { useQuote } from './Cart'

const fmtCard = (v: string) =>
  v
    .replace(/\D/g, '')
    .slice(0, 19)
    .replace(/(.{4})/g, '$1 ')
    .trim()
const fmtExpiry = (v: string) => {
  const d = v.replace(/\D/g, '').slice(0, 4)
  return d.length > 2 ? `${d.slice(0, 2)}/${d.slice(2)}` : d
}

export default function Checkout() {
  const { items, clear } = useCart()
  const { quote } = useQuote()
  const navigate = useNavigate()
  const [card, setCard] = useState('')
  const [expiry, setExpiry] = useState('')
  const [cvc, setCvc] = useState('')
  const [holder, setHolder] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [paying, setPaying] = useState(false)
  // one key per checkout attempt: retries of the same submit never double-charge
  const idempotencyKey = useMemo(() => crypto.randomUUID(), [])

  if (items.length === 0)
    return (
      <div className="container empty">
        <h2>Nothing to check out</h2>
        <Link to="/store" className="btn btn-primary">
          Browse products
        </Link>
      </div>
    )

  const pay = async (e: React.FormEvent) => {
    e.preventDefault()
    setError(null)
    setPaying(true)
    try {
      const order = await api<Order>('/api/checkout', {
        method: 'POST',
        json: { items, payment: { cardNumber: card, expiry, cvc, holder }, idempotencyKey },
      })
      clear()
      navigate(`/orders/${order.id}?new=1`, { replace: true })
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) navigate('/login?next=/checkout')
      setError((err as Error).message)
      setPaying(false)
    }
  }

  return (
    <div className="container checkout">
      <h1>Checkout</h1>
      <div className="cart-layout">
        <form className="pay-form" onSubmit={pay}>
          <h2>Payment</h2>
          <div className="test-note">
            <strong>Test mode.</strong> No real money moves. Use <code>4242 4242 4242 4242</code>, any future expiry and any CVC.
            Try <code>4000 0000 0000 0002</code> to see a declined card.
          </div>
          <label className="field">
            <span>Card number</span>
            <input inputMode="numeric" autoComplete="cc-number" placeholder="1234 5678 9012 3456" value={card} onChange={(e) => setCard(fmtCard(e.target.value))} required />
          </label>
          <div className="field-row">
            <label className="field">
              <span>Expiry</span>
              <input inputMode="numeric" autoComplete="cc-exp" placeholder="MM/YY" value={expiry} onChange={(e) => setExpiry(fmtExpiry(e.target.value))} required />
            </label>
            <label className="field">
              <span>CVC</span>
              <input inputMode="numeric" autoComplete="cc-csc" placeholder="123" value={cvc} onChange={(e) => setCvc(e.target.value.replace(/\D/g, '').slice(0, 4))} required />
            </label>
          </div>
          <label className="field">
            <span>Name on card</span>
            <input autoComplete="cc-name" value={holder} onChange={(e) => setHolder(e.target.value)} required />
          </label>
          {error && <div className="alert">{error}</div>}
          <button className="btn btn-primary btn-lg btn-block" disabled={paying || !quote}>
            {paying ? 'Processing…' : `Pay ${formatPrice(quote?.totalCents ?? 0)}`}
          </button>
        </form>
        <aside className="summary">
          <h2>Order</h2>
          {(quote?.items ?? []).map((l) => (
            <div className="summary-row" key={l.offerId}>
              <span className="summary-item">
                {l.quantity}× {l.productName}
              </span>
              <span>{formatPrice(l.priceCents * l.quantity)}</span>
            </div>
          ))}
          <div className="summary-row total">
            <span>Total</span>
            <span>{formatPrice(quote?.totalCents ?? 0)}</span>
          </div>
        </aside>
      </div>
    </div>
  )
}
