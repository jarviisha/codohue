import type {
  NamespaceConfig,
  NamespaceUpsertRequest,
} from '../../services/namespaces'

// Local form state. The wire types use a `Record<string, number>` for
// action_weights, but the UI needs ordered rows with editable keys, so we
// store an array internally and serialise at submit time.

export type DenseStrategy = 'item2vec' | 'svd' | 'byoe' | 'disabled'

export interface ActionWeightRow {
  action: string
  weight: number
}

export interface NamespaceFormState {
  namespace: string
  action_weights: ActionWeightRow[]
  lambda: number
  gamma: number
  alpha: number
  max_results: number
  seen_items_days: number
  dense_strategy: DenseStrategy
  embedding_dim: number
  dense_distance: string
  trending_window: number
  trending_ttl: number
  lambda_trending: number
}

export const defaultActionWeights: ActionWeightRow[] = [
  { action: 'VIEW', weight: 1.0 },
  { action: 'LIKE', weight: 4.0 },
]

export const defaultFormState: NamespaceFormState = {
  namespace: '',
  action_weights: defaultActionWeights,
  lambda: 0.05,
  gamma: 0.1,
  alpha: 0.7,
  max_results: 50,
  seen_items_days: 7,
  dense_strategy: 'item2vec',
  embedding_dim: 128,
  dense_distance: 'cosine',
  trending_window: 24,
  trending_ttl: 300,
  lambda_trending: 0.1,
}

export function normalizeNamespaceName(value: string): string {
  return value.trim().toLowerCase()
}

/** Build a form state from an existing namespace config. */
export function fromNamespaceConfig(c: NamespaceConfig): NamespaceFormState {
  const validStrategy: DenseStrategy[] = ['item2vec', 'svd', 'byoe', 'disabled']
  const denseStrategy: DenseStrategy = validStrategy.includes(
    c.dense_strategy as DenseStrategy,
  )
    ? (c.dense_strategy as DenseStrategy)
    : 'item2vec'

  return {
    namespace: c.namespace,
    action_weights: Object.entries(c.action_weights ?? {}).map(
      ([action, weight]) => ({
        action,
        weight,
      }),
    ),
    lambda: c.lambda,
    gamma: c.gamma,
    alpha: c.alpha,
    max_results: c.max_results,
    seen_items_days: c.seen_items_days,
    dense_strategy: denseStrategy,
    embedding_dim: c.embedding_dim,
    dense_distance: c.dense_distance || 'cosine',
    trending_window: c.trending_window,
    trending_ttl: c.trending_ttl,
    lambda_trending: c.lambda_trending,
  }
}

/** Serialise a form state into the wire-shaped upsert payload. */
export function toUpsertPayload(state: NamespaceFormState): NamespaceUpsertRequest {
  const action_weights: Record<string, number> = {}
  for (const row of state.action_weights) {
    const key = row.action.trim()
    if (key && Number.isFinite(row.weight)) action_weights[key] = row.weight
  }
  return {
    action_weights,
    lambda: state.lambda,
    gamma: state.gamma,
    alpha: state.alpha,
    max_results: state.max_results,
    seen_items_days: state.seen_items_days,
    dense_strategy: state.dense_strategy,
    embedding_dim: state.embedding_dim,
    dense_distance: state.dense_distance,
    trending_window: state.trending_window,
    trending_ttl: state.trending_ttl,
    lambda_trending: state.lambda_trending,
  }
}

export interface NamespaceFormErrors {
  namespace?: string
  action_weights?: string
  lambda?: string
  gamma?: string
  embedding_dim?: string
  alpha?: string
  max_results?: string
  seen_items_days?: string
  trending_window?: string
  trending_ttl?: string
  lambda_trending?: string
}

const NAMESPACE_RE = /^[a-z0-9_-]+$/

export function validateNamespaceForm(
  state: NamespaceFormState,
  mode: 'create' | 'edit',
): NamespaceFormErrors {
  const errs: NamespaceFormErrors = {}

  if (mode === 'create') {
    const namespace = normalizeNamespaceName(state.namespace)
    if (!namespace) {
      errs.namespace = 'Required.'
    } else if (!NAMESPACE_RE.test(namespace)) {
      errs.namespace = 'Use lowercase letters, digits, _ or -.'
    }
  }

  const hasAction = state.action_weights.some((r) => r.action.trim().length > 0)
  if (!hasAction) {
    errs.action_weights = 'At least one action weight is required.'
  } else {
    const seen = new Set<string>()
    for (const r of state.action_weights) {
      const key = r.action.trim()
      if (!key) continue
      if (seen.has(key)) {
        errs.action_weights = `Duplicate action "${key}".`
        break
      }
      if (!Number.isFinite(r.weight)) {
        errs.action_weights = `Weight for "${key}" must be a number.`
        break
      }
      seen.add(key)
    }
  }

  if (!Number.isFinite(state.lambda) || state.lambda < 0) {
    errs.lambda = 'Must be 0 or greater.'
  }
  if (!Number.isFinite(state.gamma) || state.gamma < 0) {
    errs.gamma = 'Must be 0 or greater.'
  }
  if (!Number.isFinite(state.alpha) || state.alpha < 0 || state.alpha > 1) {
    errs.alpha = 'Must be between 0 and 1.'
  }
  if (!Number.isFinite(state.embedding_dim) || state.embedding_dim <= 0) {
    errs.embedding_dim = 'Must be greater than 0.'
  }
  if (!Number.isFinite(state.max_results) || state.max_results <= 0) {
    errs.max_results = 'Must be greater than 0.'
  }
  if (!Number.isFinite(state.seen_items_days) || state.seen_items_days < 0) {
    errs.seen_items_days = 'Must be 0 or greater.'
  }
  if (!Number.isFinite(state.trending_window) || state.trending_window <= 0) {
    errs.trending_window = 'Must be greater than 0.'
  }
  if (!Number.isFinite(state.trending_ttl) || state.trending_ttl <= 0) {
    errs.trending_ttl = 'Must be greater than 0.'
  }
  if (!Number.isFinite(state.lambda_trending) || state.lambda_trending < 0) {
    errs.lambda_trending = 'Must be 0 or greater.'
  }

  return errs
}

export function hasErrors(errs: NamespaceFormErrors): boolean {
  return Object.keys(errs).length > 0
}
