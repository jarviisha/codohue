import type { NamespaceConfig, UpsertNamespacePayload } from '../types'

export interface NamespaceFormState {
  name: string
  action_weights: Record<string, number>
  lambda: number
  gamma: number
  alpha: number
  max_results: number
  seen_items_days: number
  dense_strategy: string
  embedding_dim: number
  dense_distance: string
  trending_window: number
  trending_ttl: number
  lambda_trending: number
}

export function defaultNamespaceForm(): NamespaceFormState {
  return {
    name: '',
    action_weights: { VIEW: 1, LIKE: 5, COMMENT: 8, SHARE: 10, SKIP: -2 },
    lambda: 0.05,
    gamma: 0.02,
    alpha: 0.7,
    max_results: 50,
    seen_items_days: 30,
    dense_strategy: 'item2vec',
    embedding_dim: 64,
    dense_distance: 'cosine',
    trending_window: 24,
    trending_ttl: 600,
    lambda_trending: 0.1,
  }
}

export function namespaceConfigToForm(config: NamespaceConfig): NamespaceFormState {
  return {
    name: config.namespace,
    action_weights: config.action_weights || defaultNamespaceForm().action_weights,
    lambda: config.lambda,
    gamma: config.gamma,
    alpha: config.alpha,
    max_results: config.max_results,
    seen_items_days: config.seen_items_days,
    dense_strategy: config.dense_strategy,
    embedding_dim: config.embedding_dim,
    dense_distance: config.dense_distance,
    trending_window: config.trending_window,
    trending_ttl: config.trending_ttl,
    lambda_trending: config.lambda_trending,
  }
}

export function namespaceFormToPayload(form: NamespaceFormState): UpsertNamespacePayload {
  return {
    action_weights: form.action_weights,
    lambda: form.lambda,
    gamma: form.gamma,
    alpha: form.alpha,
    max_results: form.max_results,
    seen_items_days: form.seen_items_days,
    dense_strategy: form.dense_strategy,
    embedding_dim: form.embedding_dim,
    dense_distance: form.dense_distance,
    trending_window: form.trending_window,
    trending_ttl: form.trending_ttl,
    lambda_trending: form.lambda_trending,
  }
}
