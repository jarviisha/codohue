// Package metricsroll provides rolling time-window observation slots used by
// the admin plane to compute rate-per-second and percentile statistics from
// Prometheus counters / histograms without scraping the /metrics endpoint.
//
// Two primitives:
//   - [CounterSlot] stores cumulative counter snapshots and computes a rate.
//   - [HistogramSlot] stores raw observations and computes a percentile.
//
// Both expire samples outside the configured window. Implementation is O(n)
// per operation and is intentionally simple; swap [HistogramSlot] to a
// t-digest sketch if observation volume on a hot path grows past the cheap
// linear-scan range.
package metricsroll
