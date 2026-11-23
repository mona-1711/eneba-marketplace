import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { asset, type Banner, type Category, type HomeBlock, type ProductSummary } from '../lib/api'
import { useFetch } from '../lib/useFetch'
import { ChevronLeft, ChevronRight } from './Icons'
import ProductCard, { CardSkeleton } from './ProductCard'

// Each homepage block fetches its own content from the `source` endpoint the
// backend gave it. The frontend only maps a block `kind` to a renderer.

function BlockError({ title }: { title: string }) {
  return <div className="block-error">Couldn't load “{title}”. It will be back shortly.</div>
}

export function BannerBlock({ block }: { block: HomeBlock }) {
  const { data, error } = useFetch<{ items: Banner[] }>(block.source)
  const [i, setI] = useState(0)
  const items = data?.items ?? []

  useEffect(() => {
    if (items.length < 2) return
    const t = setInterval(() => setI((x) => (x + 1) % items.length), 6000)
    return () => clearInterval(t)
  }, [items.length])

  if (error) return <BlockError title={block.title} />
  if (!data) return <div className="hero skeleton" />
  if (items.length === 0) return null
  const b = items[i % items.length]
  return (
    <section className="hero" style={{ ['--h' as string]: b.hue }} aria-roledescription="carousel">
      <div className="hero-content">
        <span className="hero-eyebrow">{block.title}</span>
        <h1>{b.headline}</h1>
        <p>{b.subline}</p>
        <Link to={b.ctaLink} className="btn btn-primary btn-lg">
          {b.ctaLabel}
        </Link>
      </div>
      {b.imageUrl && <img className="hero-art" src={asset(b.imageUrl)} alt="" />}
      {items.length > 1 && (
        <div className="hero-dots">
          {items.map((x, k) => (
            <button key={x.id} className={k === i ? 'on' : ''} onClick={() => setI(k)} aria-label={`Show banner ${k + 1}`} />
          ))}
        </div>
      )}
    </section>
  )
}

export function RailBlock({ block }: { block: HomeBlock }) {
  const { data, error } = useFetch<{ items: ProductSummary[] }>(block.source)
  const track = useRef<HTMLDivElement>(null)
  const scroll = (dir: number) => track.current?.scrollBy({ left: dir * track.current.clientWidth * 0.8, behavior: 'smooth' })

  if (error) return <BlockError title={block.title} />
  if (data && data.items.length === 0) return null
  return (
    <section className="rail">
      <div className="section-head">
        <h2>{block.title}</h2>
        <div className="rail-nav">
          <button onClick={() => scroll(-1)} aria-label="Scroll left">
            <ChevronLeft />
          </button>
          <button onClick={() => scroll(1)} aria-label="Scroll right">
            <ChevronRight />
          </button>
        </div>
      </div>
      <div className="rail-track" ref={track}>
        {data ? data.items.map((p) => <ProductCard key={p.id} p={p} />) : Array.from({ length: 6 }, (_, k) => <CardSkeleton key={k} />)}
      </div>
    </section>
  )
}

export function CategoriesBlock({ block }: { block: HomeBlock }) {
  const { data, error } = useFetch<{ items: Category[] }>(block.source)
  if (error) return <BlockError title={block.title} />
  return (
    <section className="rail">
      <div className="section-head">
        <h2>{block.title}</h2>
      </div>
      <div className="cat-grid">
        {(data?.items ?? []).map((c) => (
          <Link key={c.label} to={c.link} className="cat" style={{ ['--h' as string]: c.hue }}>
            <span className="cat-label">{c.label}</span>
            {c.count > 0 && <span className="cat-count">{c.count} products</span>}
          </Link>
        ))}
      </div>
    </section>
  )
}

const renderers: Record<string, (p: { block: HomeBlock }) => JSX.Element | null> = {
  banner: BannerBlock,
  product_rail: RailBlock,
  categories: CategoriesBlock,
}

export function HomeBlockView({ block }: { block: HomeBlock }) {
  const R = renderers[block.kind]
  return R ? <R block={block} /> : null // unknown kinds are skipped, so the backend can add new ones safely
}
