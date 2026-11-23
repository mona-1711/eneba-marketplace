// Small inline icon set (no icon font dependency).
type P = { size?: number }
const svg = (d: string) =>
  function Icon({ size = 20 }: P) {
    return (
      <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" aria-hidden>
        <path d={d} />
      </svg>
    )
  }

export const SearchIcon = svg('M11 19a8 8 0 1 0 0-16 8 8 0 0 0 0 16zM21 21l-4.35-4.35')
export const CartIcon = svg('M6 6h15l-1.5 9h-12zM6 6 5 3H2M9 20a1 1 0 1 0 0-2 1 1 0 0 0 0 2zM18 20a1 1 0 1 0 0-2 1 1 0 0 0 0 2z')
export const UserIcon = svg('M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2M12 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8z')
export const ChevronLeft = svg('M15 18l-6-6 6-6')
export const ChevronRight = svg('M9 18l6-6-6-6')
export const CheckIcon = svg('M20 6 9 17l-5-5')
export const BoltIcon = svg('M13 2 3 14h9l-1 8 10-12h-9z')
export const ShieldIcon = svg('M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z')
export const CopyIcon = svg('M9 9h11v11H9zM5 15H4V4h11v1')
export const FilterIcon = svg('M22 3H2l8 9.46V19l4 2v-8.54z')
export const XIcon = svg('M18 6 6 18M6 6l12 12')
export const StarIcon = svg('M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01z')
export const GlobeIcon = svg('M12 22a10 10 0 1 0 0-20 10 10 0 0 0 0 20zM2 12h20M12 2a15 15 0 0 1 0 20M12 2a15 15 0 0 0 0 20')
