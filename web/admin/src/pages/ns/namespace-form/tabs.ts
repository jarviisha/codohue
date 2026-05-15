import type { NamespaceFormErrors } from '../configForm'
import type { TabId } from './types'

export const TAB_LABEL: Record<TabId, string> = {
  identity: 'identity',
  actions: 'actions',
  scoring: 'scoring',
  dense: 'dense',
  trending: 'trending',
}

export function tabsForMode(mode: 'create' | 'edit'): TabId[] {
  return mode === 'create'
    ? ['identity', 'actions', 'scoring', 'dense', 'trending']
    : ['actions', 'scoring', 'dense', 'trending']
}

export function firstErrorTab(
  errors: NamespaceFormErrors,
  mode: 'create' | 'edit',
): TabId | null {
  if (mode === 'create' && errors.namespace) return 'identity'
  if (errors.action_weights) return 'actions'
  if (
    errors.lambda ||
    errors.gamma ||
    errors.alpha ||
    errors.max_results ||
    errors.seen_items_days
  ) {
    return 'scoring'
  }
  if (errors.embedding_dim) return 'dense'
  if (
    errors.trending_window ||
    errors.trending_ttl ||
    errors.lambda_trending
  ) {
    return 'trending'
  }
  return null
}

export function tabHasError(tab: TabId, errors: NamespaceFormErrors): boolean {
  switch (tab) {
    case 'identity':
      return Boolean(errors.namespace)
    case 'actions':
      return Boolean(errors.action_weights)
    case 'scoring':
      return Boolean(
        errors.lambda ||
          errors.gamma ||
          errors.alpha ||
          errors.max_results ||
          errors.seen_items_days,
      )
    case 'dense':
      return Boolean(errors.embedding_dim)
    case 'trending':
      return Boolean(
        errors.trending_window ||
          errors.trending_ttl ||
          errors.lambda_trending,
      )
  }
}
