import { useQuery } from '@tanstack/react-query'
import { apiFetch } from './http'

// ---------------------------------------------------------------------------
// Wire types — mirror internal/admin/types.go::MetricsSummaryResponse.
// Recommend/embedder slices are intentionally absent: those counters live in
// separate processes and are scraped by Prometheus directly.
// ---------------------------------------------------------------------------

type MetricsSummaryIngest = {
  events_per_sec_1m: Record<string, number>
  events_per_sec_5m: Record<string, number>
}

type MetricsSummaryCron = {
  batch_lag_seconds: number
}

export type MetricsSummaryResponse = {
  generated_at: string
  ingest: MetricsSummaryIngest
  cron: MetricsSummaryCron
}

const metricsKeys = {
  summary: ['metrics', 'summary'] as const,
}

/**
 * useMetricsSummary polls the curated rolling-window metrics that back the
 * fleet "events/s" tile. Refetches every 10s to match the server-side sampler.
 */
export function useMetricsSummary() {
  return useQuery({
    queryKey: metricsKeys.summary,
    queryFn: () => apiFetch<MetricsSummaryResponse>('/api/admin/v1/metrics/summary'),
    refetchInterval: 10_000,
  })
}

/**
 * sumRates totals a per-namespace rate map — used to show fleet-wide events/s.
 */
export function sumRates(rates: Record<string, number> | undefined): number {
  if (!rates) return 0
  return Object.values(rates).reduce((acc, n) => acc + n, 0)
}
