// Typed API client. VITE_API_URL lets the frontend be hosted separately
// (e.g. Vercel); by default it calls the same origin that served it.
const BASE = (import.meta.env.VITE_API_URL as string | undefined)?.replace(/\/$/, '') ?? ''

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message)
  }
}

export async function api<T>(path: string, init?: RequestInit & { json?: unknown }): Promise<T> {
  const { json, ...rest } = init ?? {}
  const res = await fetch(BASE + path, {
    credentials: 'include',
    ...rest,
    headers: { ...(json !== undefined ? { 'Content-Type': 'application/json' } : {}), ...rest.headers },
    body: json !== undefined ? JSON.stringify(json) : rest.body,
  })
  if (res.status === 204) return undefined as T
  const data = await res.json().catch(() => ({}))
  if (!res.ok) throw new ApiError(res.status, data.error ?? `Request failed (${res.status})`)
  return data as T
}

export const asset = (path: string) => (path.startsWith('/api/') ? BASE + path : path)

export interface ProductSummary {
  id: number
  slug: string
  name: string
  title: string
  platform: string
  device: string
  region: string
  type: string
  genre: string
  releaseYear: number
  coverUrl: string
  priceCents: number | null
  retailCents: number
  discountPct: number
  offerCount: number
  sales: number
}

export interface Offer {
  id: number
  sellerId: number
  sellerName: string
  sellerRating: number
  sellerReviews: number
  priceCents: number
  stock: number
}

export interface ProductDetail extends ProductSummary {
  description: string
  developer: string
  offers: Offer[]
  editions: ProductSummary[]
  related: ProductSummary[]
}

export interface ProductPage {
  items: ProductSummary[]
  total: number
  page: number
  pageSize: number
  didYouMean: string
}

export interface Facet {
  value: string
  count: number
}

export interface Facets {
  platform: Facet[]
  region: Facet[]
  type: Facet[]
  genre: Facet[]
  priceRange: { minCents: number; maxCents: number }
}

export interface HomeBlock {
  id: number
  kind: 'banner' | 'product_rail' | 'categories' | string
  title: string
  source: string
}

export interface Banner {
  id: number
  headline: string
  subline: string
  ctaLabel: string
  ctaLink: string
  imageUrl: string
  hue: number
}

export interface Category {
  label: string
  link: string
  hue: number
  count: number
}

export interface User {
  id: number
  email: string
  name: string
}

export interface QuotedLine {
  offerId: number
  quantity: number
  productSlug: string
  productName: string
  coverUrl: string
  sellerName: string
  priceCents: number
  stock: number
  available: boolean
}

export interface OrderItem {
  productSlug: string
  productName: string
  coverUrl: string
  sellerName: string
  priceCents: number
  keyCode: string
}

export interface Order {
  id: number
  totalCents: number
  status: string
  paymentRef: string
  cardLast4: string
  createdAt: string
  items: OrderItem[]
}

export const formatPrice = (cents: number | null | undefined) =>
  cents == null ? '—' : new Intl.NumberFormat('en-IE', { style: 'currency', currency: 'EUR' }).format(cents / 100)
