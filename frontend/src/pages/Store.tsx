import { useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import ProductCard, { CardSkeleton } from '../components/ProductCard'
import { FilterIcon, XIcon } from '../components/Icons'
import type { Facets, ProductPage } from '../lib/api'
import { useFetch } from '../lib/useFetch'

// All store state lives in the URL, so filtered views are shareable and the
// back button works. The sidebar options and counts come from /api/facets.

const SORTS: [string, string][] = [
  ['popular', 'Most popular'],
  ['relevance', 'Best match'],
  ['price_asc', 'Price: low to high'],
  ['price_desc', 'Price: high to low'],
  ['discount', 'Biggest discount'],
  ['newest', 'Newest'],
  ['name', 'Name A–Z'],
]

const FACET_LABELS: [keyof Omit<Facets, 'priceRange'>, string][] = [
  ['platform', 'Platform'],
  ['type', 'Type'],
  ['region', 'Region'],
  ['genre', 'Genre'],
]

export default function Store() {
  const [params, setParams] = useSearchParams()
  const facets = useFetch<Facets>('/api/facets')
  const apiQuery = new URLSearchParams(params)
  apiQuery.set('pageSize', '24')
  const { data, loading, error } = useFetch<ProductPage>(`/api/products?${apiQuery}`)
  const [showFilters, setShowFilters] = useState(false)
  const [minPrice, setMinPrice] = useState(params.get('minPrice') ?? '')
  const [maxPrice, setMaxPrice] = useState(params.get('maxPrice') ?? '')

  useEffect(() => {
    setMinPrice(params.get('minPrice') ?? '')
    setMaxPrice(params.get('maxPrice') ?? '')
  }, [params])

  const q = params.get('q') ?? ''
  const page = Number(params.get('page') ?? '1')
  const sort = params.get('sort') ?? (q ? 'relevance' : 'popular')

  const update = (fn: (p: URLSearchParams) => void) => {
    const next = new URLSearchParams(params)
    fn(next)
    next.delete('page')
    setParams(next)
  }

  const toggle = (key: string, value: string) =>
    update((p) => {
      const vals = p.getAll(key)
      p.delete(key)
      const nextVals = vals.includes(value) ? vals.filter((v) => v !== value) : [...vals, value]
      nextVals.forEach((v) => p.append(key, v))
    })

  const applyPrice = () =>
    update((p) => {
      minPrice ? p.set('minPrice', minPrice) : p.delete('minPrice')
      maxPrice ? p.set('maxPrice', maxPrice) : p.delete('maxPrice')
    })

  const active: [string, string][] = []
  for (const [k] of FACET_LABELS) params.getAll(k).forEach((v) => active.push([k, v]))
  if (params.get('minPrice')) active.push(['minPrice', `From €${params.get('minPrice')}`])
  if (params.get('maxPrice')) active.push(['maxPrice', `Up to €${params.get('maxPrice')}`])
  if (params.get('inStock')) active.push(['inStock', 'In stock'])

  const totalPages = data ? Math.max(1, Math.ceil(data.total / data.pageSize)) : 1

  return (
    <div className="container store">
      <div className="store-head">
        <div>
          <h1>{q ? <>Results for “{q}”</> : 'All products'}</h1>
          {data && (
            <p className="muted">
              {data.total} {data.total === 1 ? 'result' : 'results'}
              {data.didYouMean && (
                <>
                  {' '}
                  · Showing matches for <strong>{data.didYouMean}</strong>
                </>
              )}
            </p>
          )}
        </div>
        <div className="store-controls">
          <button className="btn btn-ghost filters-toggle" onClick={() => setShowFilters((s) => !s)}>
            <FilterIcon size={16} /> Filters {active.length > 0 && `(${active.length})`}
          </button>
          <label className="select">
            <span>Sort by</span>
            <select value={sort} onChange={(e) => update((p) => p.set('sort', e.target.value))}>
              {SORTS.filter(([k]) => k !== 'relevance' || q).map(([k, l]) => (
                <option key={k} value={k}>
                  {l}
                </option>
              ))}
            </select>
          </label>
        </div>
      </div>

      {active.length > 0 && (
        <div className="chips">
          {active.map(([k, v]) => (
            <button
              key={k + v}
              className="chip"
              onClick={() => (['minPrice', 'maxPrice', 'inStock'].includes(k) ? update((p) => p.delete(k)) : toggle(k, v))}
            >
              {v} <XIcon size={14} />
            </button>
          ))}
          <button
            className="chip chip-clear"
            onClick={() =>
              setParams(q ? new URLSearchParams({ q }) : new URLSearchParams())
            }
          >
            Clear all
          </button>
        </div>
      )}

      <div className="store-layout">
        <aside className={`filters ${showFilters ? 'open' : ''}`}>
          <div className="filters-mobile-head">
            <strong>Filters</strong>
            <button onClick={() => setShowFilters(false)} aria-label="Close filters">
              <XIcon />
            </button>
          </div>
          <div className="filter-group">
            <h3>Price (€)</h3>
            <form
              className="price-inputs"
              onSubmit={(e) => {
                e.preventDefault()
                applyPrice()
              }}
            >
              <input inputMode="decimal" placeholder="Min" value={minPrice} onChange={(e) => setMinPrice(e.target.value.replace(/[^\d.]/g, ''))} />
              <span>–</span>
              <input inputMode="decimal" placeholder="Max" value={maxPrice} onChange={(e) => setMaxPrice(e.target.value.replace(/[^\d.]/g, ''))} />
              <button className="btn btn-small">Go</button>
            </form>
            <label className="check">
              <input type="checkbox" checked={params.get('inStock') === '1'} onChange={(e) => update((p) => (e.target.checked ? p.set('inStock', '1') : p.delete('inStock')))} />
              <span>In stock only</span>
            </label>
          </div>
          {FACET_LABELS.map(([key, label]) => (
            <div className="filter-group" key={key}>
              <h3>{label}</h3>
              {(facets.data?.[key] ?? []).map((f) => (
                <label className="check" key={f.value}>
                  <input type="checkbox" checked={params.getAll(key).includes(f.value)} onChange={() => toggle(key, f.value)} />
                  <span>{f.value}</span>
                  <span className="count">{f.count}</span>
                </label>
              ))}
            </div>
          ))}
        </aside>

        <section className="results">
          {error && <div className="page-msg">Something went wrong: {error}</div>}
          {data && data.total === 0 && (
            <div className="empty">
              <h2>No products found</h2>
              <p>Try a different spelling, fewer words, or remove some filters.</p>
              <Link to="/store" className="btn btn-primary">
                Browse everything
              </Link>
            </div>
          )}
          <div className={`grid ${loading && data ? 'dim' : ''}`}>
            {data ? data.items.map((p) => <ProductCard key={p.id} p={p} />) : Array.from({ length: 12 }, (_, k) => <CardSkeleton key={k} />)}
          </div>
          {data && totalPages > 1 && (
            <nav className="pager" aria-label="Pagination">
              <button disabled={page <= 1} onClick={() => setParams((p) => (p.set('page', String(page - 1)), p))}>
                Previous
              </button>
              <span>
                Page {page} of {totalPages}
              </span>
              <button disabled={page >= totalPages} onClick={() => setParams((p) => (p.set('page', String(page + 1)), p))}>
                Next
              </button>
            </nav>
          )}
        </section>
      </div>
    </div>
  )
}
