import { useState } from 'react'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { CheckIcon, CopyIcon } from '../components/Icons'
import { asset, formatPrice, type Order } from '../lib/api'
import { useFetch } from '../lib/useFetch'

const fmtDate = (s: string) => new Date(s.replace(' ', 'T') + 'Z').toLocaleString()

function KeyCode({ code }: { code: string }) {
  const [revealed, setRevealed] = useState(false)
  const [copied, setCopied] = useState(false)
  if (!revealed)
    return (
      <button className="btn btn-small btn-primary" onClick={() => setRevealed(true)}>
        Reveal key
      </button>
    )
  return (
    <span className="key">
      <code>{code}</code>
      <button
        className="icon-btn"
        aria-label="Copy key"
        onClick={() => {
          navigator.clipboard?.writeText(code).then(() => {
            setCopied(true)
            setTimeout(() => setCopied(false), 1500)
          })
        }}
      >
        {copied ? <CheckIcon size={16} /> : <CopyIcon size={16} />}
      </button>
    </span>
  )
}

function OrderCard({ o }: { o: Order }) {
  return (
    <div className="order">
      <div className="order-head">
        <div>
          <strong>Order #{o.id}</strong> <span className="muted small">{fmtDate(o.createdAt)}</span>
        </div>
        <div className="muted small">
          Paid {formatPrice(o.totalCents)} · card •••• {o.cardLast4} · <span className="status">{o.status}</span>
        </div>
      </div>
      {o.items.map((it, i) => (
        <div className="order-item" key={i}>
          <img src={asset(it.coverUrl)} alt="" />
          <div className="order-item-info">
            <Link to={`/product/${it.productSlug}`}>{it.productName}</Link>
            <span className="muted small">
              {it.sellerName} · {formatPrice(it.priceCents)}
            </span>
          </div>
          <KeyCode code={it.keyCode} />
        </div>
      ))}
    </div>
  )
}

export function OrderDetail() {
  const { id } = useParams()
  const [params] = useSearchParams()
  const { data, error } = useFetch<Order>(`/api/orders/${id}`)
  if (error) return <div className="container page-msg">{error}</div>
  if (!data) return <div className="container page-msg">Loading…</div>
  return (
    <div className="container orders">
      {params.get('new') && (
        <div className="success">
          <span className="success-icon">
            <CheckIcon size={28} />
          </span>
          <div>
            <h1>Payment successful</h1>
            <p>Your keys are ready below. They're also saved in your order history.</p>
          </div>
        </div>
      )}
      <OrderCard o={data} />
      <Link to="/orders" className="btn btn-ghost">
        All orders
      </Link>
    </div>
  )
}

export default function Orders() {
  const { data, error } = useFetch<{ items: Order[] }>('/api/orders')
  if (error) return <div className="container page-msg">{error}</div>
  return (
    <div className="container orders">
      <h1>My orders & keys</h1>
      {data && data.items.length === 0 && (
        <div className="empty">
          <p>No orders yet.</p>
          <Link to="/store" className="btn btn-primary">
            Start shopping
          </Link>
        </div>
      )}
      {data?.items.map((o) => <OrderCard key={o.id} o={o} />)}
    </div>
  )
}
