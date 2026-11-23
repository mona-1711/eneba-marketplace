import { HomeBlockView } from '../components/Blocks'
import type { HomeBlock } from '../lib/api'
import { useFetch } from '../lib/useFetch'

// The homepage is fully described by the backend: GET /api/home returns the ordered
// blocks, and each block loads its own content. Reordering, adding or disabling a
// block is a database change, not a frontend change.
export default function Home() {
  const { data, error } = useFetch<{ blocks: HomeBlock[] }>('/api/home')
  if (error) return <div className="container page-msg">Couldn't load the homepage: {error}</div>
  return (
    <div className="container home">
      {data ? data.blocks.map((b) => <HomeBlockView key={b.id} block={b} />) : <div className="hero skeleton" />}
    </div>
  )
}
