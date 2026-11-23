import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'

// The cart is kept in localStorage so guests can shop before signing in.
// It only stores offer IDs + quantities; prices are always re-quoted by the server.

export interface CartItem {
  offerId: number
  quantity: number
}

interface CartState {
  items: CartItem[]
  count: number
  add: (offerId: number, quantity?: number) => void
  setQuantity: (offerId: number, quantity: number) => void
  remove: (offerId: number) => void
  clear: () => void
  lastAdded: number | null
}

const KEY = 'keyport.cart.v1'
const CartContext = createContext<CartState | null>(null)

function load(): CartItem[] {
  try {
    const raw = localStorage.getItem(KEY)
    const parsed = raw ? JSON.parse(raw) : []
    return Array.isArray(parsed) ? parsed.filter((i) => Number.isInteger(i.offerId) && i.quantity > 0) : []
  } catch {
    return []
  }
}

export function CartProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<CartItem[]>(load)
  const [lastAdded, setLastAdded] = useState<number | null>(null)

  useEffect(() => {
    try {
      localStorage.setItem(KEY, JSON.stringify(items))
    } catch {
      /* storage unavailable: cart stays in memory */
    }
  }, [items])

  // keep tabs in sync
  useEffect(() => {
    const onStorage = (e: StorageEvent) => e.key === KEY && setItems(load())
    window.addEventListener('storage', onStorage)
    return () => window.removeEventListener('storage', onStorage)
  }, [])

  const add = useCallback((offerId: number, quantity = 1) => {
    setItems((prev) => {
      const found = prev.find((i) => i.offerId === offerId)
      if (found) return prev.map((i) => (i.offerId === offerId ? { ...i, quantity: Math.min(10, i.quantity + quantity) } : i))
      return [...prev, { offerId, quantity }]
    })
    setLastAdded(Date.now())
  }, [])

  const setQuantity = useCallback((offerId: number, quantity: number) => {
    setItems((prev) =>
      quantity < 1 ? prev.filter((i) => i.offerId !== offerId) : prev.map((i) => (i.offerId === offerId ? { ...i, quantity: Math.min(10, quantity) } : i)),
    )
  }, [])

  const remove = useCallback((offerId: number) => setItems((prev) => prev.filter((i) => i.offerId !== offerId)), [])
  const clear = useCallback(() => setItems([]), [])
  const count = useMemo(() => items.reduce((n, i) => n + i.quantity, 0), [items])

  return <CartContext.Provider value={{ items, count, add, setQuantity, remove, clear, lastAdded }}>{children}</CartContext.Provider>
}

export function useCart() {
  const ctx = useContext(CartContext)
  if (!ctx) throw new Error('useCart outside CartProvider')
  return ctx
}
