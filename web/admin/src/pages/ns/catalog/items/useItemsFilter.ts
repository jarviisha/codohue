import { useSearchParams } from 'react-router-dom'
import type { CatalogItemsStateFilter } from '@/services/catalog'
import { nonNegativeInt, positiveInt } from '@/utils/search-params'

const DEFAULT_LIMIT = 50

export interface ItemsFilter {
  state: CatalogItemsStateFilter
  objectID: string
  limit: number
  offset: number
}

export interface ItemsFilterUpdate {
  state?: CatalogItemsStateFilter
  object_id?: string
  limit?: number
  offset?: number
}

// Reads the items list filter out of the URL and exposes a setFilter helper
// that normalizes defaults so the URL only carries non-default values.
export function useItemsFilter(): {
  filter: ItemsFilter
  setFilter: (next: ItemsFilterUpdate) => void
} {
  const [searchParams, setSearchParams] = useSearchParams()

  const filter: ItemsFilter = {
    state: (searchParams.get('state') || 'all') as CatalogItemsStateFilter,
    objectID: searchParams.get('object_id') ?? '',
    limit: positiveInt(searchParams.get('limit'), DEFAULT_LIMIT),
    offset: nonNegativeInt(searchParams.get('offset'), 0),
  }

  const setFilter = (next: ItemsFilterUpdate) => {
    const sp = new URLSearchParams(searchParams)
    if (next.state !== undefined) {
      if (next.state === 'all') sp.delete('state')
      else sp.set('state', next.state)
      sp.delete('offset')
    }
    if (next.object_id !== undefined) {
      if (next.object_id) sp.set('object_id', next.object_id)
      else sp.delete('object_id')
      sp.delete('offset')
    }
    if (next.limit !== undefined) {
      if (next.limit === DEFAULT_LIMIT) sp.delete('limit')
      else sp.set('limit', String(next.limit))
      sp.delete('offset')
    }
    if (next.offset !== undefined) {
      if (next.offset === 0) sp.delete('offset')
      else sp.set('offset', String(next.offset))
    }
    setSearchParams(sp)
  }

  return { filter, setFilter }
}
