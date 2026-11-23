import { useEffect, useRef, useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { api, asset, formatPrice, type ProductSummary } from '../lib/api'
import { SearchIcon } from './Icons'

// Global search in the header: debounced typeahead against the typo-tolerant
// backend, Enter goes to the full store results.
export default function SearchBox() {
  const [params] = useSearchParams()
  const [q, setQ] = useState(params.get('q') ?? '')
  const [items, setItems] = useState<ProductSummary[]>([])
  const [didYouMean, setDidYouMean] = useState('')
  const [open, setOpen] = useState(false)
  const [active, setActive] = useState(-1)
  const navigate = useNavigate()
  const wrap = useRef<HTMLDivElement>(null)

  useEffect(() => {
    setQ(params.get('q') ?? '')
  }, [params])

  useEffect(() => {
    const term = q.trim()
    if (!term) {
      setItems([])
      setDidYouMean('')
      return
    }
    const ctrl = new AbortController()
    const t = setTimeout(() => {
      api<{ items: ProductSummary[]; didYouMean: string }>(`/api/search/suggest?q=${encodeURIComponent(term)}`, { signal: ctrl.signal })
        .then((r) => {
          setItems(r.items)
          setDidYouMean(r.didYouMean)
          setActive(-1)
        })
        .catch(() => {})
    }, 150)
    return () => {
      clearTimeout(t)
      ctrl.abort()
    }
  }, [q])

  useEffect(() => {
    const onClick = (e: MouseEvent) => wrap.current && !wrap.current.contains(e.target as Node) && setOpen(false)
    document.addEventListener('mousedown', onClick)
    return () => document.removeEventListener('mousedown', onClick)
  }, [])

  const submit = () => {
    setOpen(false)
    if (active >= 0 && items[active]) navigate(`/product/${items[active].slug}`)
    else navigate(q.trim() ? `/store?q=${encodeURIComponent(q.trim())}` : '/store')
  }

  return (
    <div className="search" ref={wrap}>
      <form
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
        role="search"
      >
        <span className="search-icon">
          <SearchIcon size={18} />
        </span>
        <input
          value={q}
          onChange={(e) => {
            setQ(e.target.value)
            setOpen(true)
          }}
          onFocus={() => setOpen(true)}
          onKeyDown={(e) => {
            if (e.key === 'ArrowDown') {
              e.preventDefault()
              setActive((a) => Math.min(items.length - 1, a + 1))
            } else if (e.key === 'ArrowUp') {
              e.preventDefault()
              setActive((a) => Math.max(-1, a - 1))
            } else if (e.key === 'Escape') setOpen(false)
          }}
          placeholder="Search games, gift cards, subscriptions…"
          aria-label="Search"
          aria-autocomplete="list"
        />
      </form>
      {open && q.trim() && (
        <div className="search-dropdown" role="listbox">
          {didYouMean && (
            <div className="search-hint">
              Showing results for <strong>{didYouMean}</strong>
            </div>
          )}
          {items.length === 0 ? (
            <div className="search-empty">No matches for “{q}”</div>
          ) : (
            items.map((p, i) => (
              <Link
                key={p.id}
                to={`/product/${p.slug}`}
                className={`search-item ${i === active ? 'active' : ''}`}
                onClick={() => setOpen(false)}
                role="option"
                aria-selected={i === active}
              >
                <img src={asset(p.coverUrl)} alt="" />
                <span className="search-item-name">{p.name}</span>
                <span className="search-item-price">{formatPrice(p.priceCents)}</span>
              </Link>
            ))
          )}
          {items.length > 0 && (
            <button className="search-all" onClick={submit} type="button">
              See all results for “{q}”
            </button>
          )}
        </div>
      )}
    </div>
  )
}
