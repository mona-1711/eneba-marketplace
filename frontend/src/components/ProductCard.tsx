import { Link } from 'react-router-dom'
import { asset, formatPrice, type ProductSummary } from '../lib/api'

export default function ProductCard({ p }: { p: ProductSummary }) {
  const soldOut = p.priceCents == null
  return (
    <Link to={`/product/${p.slug}`} className="card">
      <div className="card-cover">
        <img src={asset(p.coverUrl)} alt="" loading="lazy" />
        {p.discountPct > 0 && <span className="card-discount">-{p.discountPct}%</span>}
      </div>
      <div className="card-body">
        <div className="card-name" title={p.name}>
          {p.name}
        </div>
        <div className="card-region">{p.region}</div>
        <div className="card-price-row">
          {soldOut ? (
            <span className="card-soldout">Sold out</span>
          ) : (
            <>
              <span className="card-from">From</span>
              {p.discountPct > 0 && <span className="card-retail">{formatPrice(p.retailCents)}</span>}
              <span className="card-price">{formatPrice(p.priceCents)}</span>
            </>
          )}
        </div>
        <div className="card-offers">{p.offerCount} {p.offerCount === 1 ? 'offer' : 'offers'}</div>
      </div>
    </Link>
  )
}

export function CardSkeleton() {
  return (
    <div className="card skeleton">
      <div className="card-cover" />
      <div className="card-body">
        <div className="sk-line" />
        <div className="sk-line short" />
      </div>
    </div>
  )
}
