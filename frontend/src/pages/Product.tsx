import { useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import ProductCard from '../components/ProductCard'
import { BoltIcon, CheckIcon, GlobeIcon, ShieldIcon, StarIcon } from '../components/Icons'
import { asset, formatPrice, type Offer, type ProductDetail } from '../lib/api'
import { useCart } from '../lib/cart'
import { useFetch } from '../lib/useFetch'

export default function Product() {
  const { slug } = useParams()
  const { data: p, error, loading } = useFetch<ProductDetail>(`/api/products/${slug}`)
  const { add } = useCart()
  const navigate = useNavigate()
  const [added, setAdded] = useState<number | null>(null)

  if (error) {
    return (
      <div className="container empty">
        <h2>Product not found</h2>
        <Link to="/store" className="btn btn-primary">
          Back to store
        </Link>
      </div>
    )
  }
  if (loading || !p) return <div className="container"><div className="product skeleton-block" /></div>

  const inStock = p.offers.filter((o) => o.stock > 0)
  const best = inStock[0]

  const addOffer = (o: Offer, buyNow = false) => {
    add(o.id)
    if (buyNow) navigate('/cart')
    else {
      setAdded(o.id)
      setTimeout(() => setAdded((x) => (x === o.id ? null : x)), 2000)
    }
  }

  return (
    <div className="container">
      <nav className="crumbs">
        <Link to="/">Home</Link> / <Link to={`/store?platform=${encodeURIComponent(p.platform)}`}>{p.platform}</Link> /{' '}
        <Link to={`/store?genre=${encodeURIComponent(p.genre)}`}>{p.genre}</Link>
      </nav>
      <div className="product">
        <img className="product-cover" src={asset(p.coverUrl)} alt={`${p.title} cover`} />
        <div className="product-info">
          <h1>{p.name}</h1>
          <div className="tags">
            <span className="tag tag-region">
              <GlobeIcon size={14} /> {p.region}
            </span>
            <span className="tag">{p.platform}</span>
            <span className="tag">{p.device}</span>
            <span className="tag">{p.type}</span>
          </div>
          <ul className="facts">
            <li>
              <CheckIcon size={16} /> Can be activated in: <strong>{p.region === 'GLOBAL' ? 'All regions' : p.region.toLowerCase()}</strong>
            </li>
            <li>
              <BoltIcon size={16} /> Digital key, <strong>instant delivery</strong>
            </li>
            <li>
              <ShieldIcon size={16} /> Redeem on <strong>{p.platform}</strong>
            </li>
          </ul>
          {p.editions.length > 0 && (
            <div className="editions">
              <span className="muted">Other editions:</span>
              {p.editions.map((e) => (
                <Link key={e.id} to={`/product/${e.slug}`} className="edition">
                  {e.platform} · {e.region}
                  <strong>{formatPrice(e.priceCents)}</strong>
                </Link>
              ))}
            </div>
          )}
        </div>
        <aside className="buybox">
          {best ? (
            <>
              {p.discountPct > 0 && (
                <div className="buybox-retail">
                  <s>{formatPrice(p.retailCents)}</s> <span className="pct">-{p.discountPct}%</span>
                </div>
              )}
              <div className="buybox-price">{formatPrice(best.priceCents)}</div>
              <div className="buybox-seller">
                Sold by <strong>{best.sellerName}</strong> · <StarIcon size={13} /> {best.sellerRating.toFixed(1)}%
              </div>
              <button className="btn btn-primary btn-lg btn-block" onClick={() => addOffer(best, true)}>
                Buy now
              </button>
              <button className="btn btn-ghost btn-block" onClick={() => addOffer(best)}>
                {added === best.id ? 'Added to cart ✓' : 'Add to cart'}
              </button>
            </>
          ) : (
            <div className="buybox-price">Sold out</div>
          )}
        </aside>
      </div>

      <section className="offers">
        <div className="section-head">
          <h2>
            {p.offers.length} offers from sellers <span className="muted">· compare and pick</span>
          </h2>
        </div>
        <div className="offer-list">
          {p.offers.map((o, i) => (
            <div key={o.id} className={`offer ${o.stock === 0 ? 'out' : ''} ${i === 0 && o.stock > 0 ? 'best' : ''}`}>
              <div className="offer-seller">
                <span className="seller-avatar">{o.sellerName[0]}</span>
                <div>
                  <strong>{o.sellerName}</strong>
                  <div className="muted small">
                    <StarIcon size={12} /> {o.sellerRating.toFixed(1)}% positive · {o.sellerReviews.toLocaleString()} reviews
                  </div>
                </div>
                {i === 0 && o.stock > 0 && <span className="best-tag">Best price</span>}
              </div>
              <div className="offer-stock muted small">{o.stock === 0 ? 'Out of stock' : o.stock < 5 ? `Only ${o.stock} left` : 'In stock'}</div>
              <div className="offer-price">{formatPrice(o.priceCents)}</div>
              <button className="btn btn-primary" disabled={o.stock === 0} onClick={() => addOffer(o)}>
                {added === o.id ? 'Added ✓' : 'Add to cart'}
              </button>
            </div>
          ))}
        </div>
      </section>

      <section className="about">
        <h2>About {p.title}</h2>
        <p>{p.description}</p>
        <dl>
          <dt>Developer</dt>
          <dd>{p.developer}</dd>
          <dt>Release year</dt>
          <dd>{p.releaseYear}</dd>
          <dt>Genre</dt>
          <dd>{p.genre}</dd>
        </dl>
      </section>

      {p.related.length > 0 && (
        <section className="rail">
          <div className="section-head">
            <h2>You might also like</h2>
          </div>
          <div className="rail-track">
            {p.related.map((r) => (
              <ProductCard key={r.id} p={r} />
            ))}
          </div>
        </section>
      )}
    </div>
  )
}
