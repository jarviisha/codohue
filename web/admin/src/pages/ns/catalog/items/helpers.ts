import type { CatalogItemState, CatalogItemsStateFilter } from '@/services/catalog'
import type { StatusState } from '@/components/ui'

export const ITEM_STATES: CatalogItemsStateFilter[] = [
  'all',
  'pending',
  'in_flight',
  'embedded',
  'failed',
  'dead_letter',
]

export function stateToken(state: CatalogItemState): StatusState {
  switch (state) {
    case 'embedded':
      return 'ok'
    case 'in_flight':
      return 'run'
    case 'failed':
    case 'dead_letter':
      return 'fail'
    case 'pending':
      return 'pend'
    default:
      return 'idle'
  }
}

export function canRedrive(state: CatalogItemState): boolean {
  return state === 'failed' || state === 'dead_letter'
}
