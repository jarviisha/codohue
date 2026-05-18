import { useSearchParams } from 'react-router-dom'
import { nonNegativeInt, positiveInt } from '@/utils/searchParams'

const DEFAULT_LIMIT = 100

export interface EventsFilter {
  subjectID: string
  limit: number
  offset: number
}

export interface EventsFilterUpdate {
  subject_id?: string
  limit?: number
  offset?: number
}

// Reads the events list filter out of the URL and exposes a setFilter helper
// that normalizes defaults so the URL only carries non-default values.
export function useEventsFilter(): {
  filter: EventsFilter
  setFilter: (next: EventsFilterUpdate) => void
} {
  const [searchParams, setSearchParams] = useSearchParams()

  const filter: EventsFilter = {
    subjectID: searchParams.get('subject_id') ?? '',
    limit: positiveInt(searchParams.get('limit'), DEFAULT_LIMIT),
    offset: nonNegativeInt(searchParams.get('offset'), 0),
  }

  const setFilter = (next: EventsFilterUpdate) => {
    const sp = new URLSearchParams(searchParams)
    if (next.subject_id !== undefined) {
      if (next.subject_id) sp.set('subject_id', next.subject_id)
      else sp.delete('subject_id')
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
