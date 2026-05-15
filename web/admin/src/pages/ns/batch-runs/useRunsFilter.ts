import { useSearchParams } from 'react-router-dom'
import type { BatchRunStatusFilter } from '@/services/batchRuns'
import { nonNegativeInt, positiveInt } from '@/utils/searchParams'

const DEFAULT_LIMIT = 20

export interface RunsFilter {
  status: BatchRunStatusFilter
  limit: number
  offset: number
}

export interface RunsFilterUpdate {
  status?: BatchRunStatusFilter
  limit?: number
  offset?: number
}

// Shared URL-state for the CF runs / Re-embeds tabs. Both tabs use the same
// (status, limit, offset) trio so switching tabs preserves the operator's
// pagination intent.
export function useRunsFilter(): {
  filter: RunsFilter
  setFilter: (next: RunsFilterUpdate) => void
} {
  const [searchParams, setSearchParams] = useSearchParams()

  const status = (searchParams.get('status') || '') as BatchRunStatusFilter
  const filter: RunsFilter = {
    status,
    limit: positiveInt(searchParams.get('limit'), DEFAULT_LIMIT),
    offset: nonNegativeInt(searchParams.get('offset'), 0),
  }

  const setFilter = (next: RunsFilterUpdate) => {
    const sp = new URLSearchParams(searchParams)
    if (next.status !== undefined) {
      if (next.status === '') sp.delete('status')
      else sp.set('status', next.status)
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
