package metrics

import "github.com/prometheus/client_golang/prometheus"

var mustRegisterFn = prometheus.MustRegister

var (
	// BatchJobLagSeconds tracks time since the last successful batch job run.
	BatchJobLagSeconds = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "codohue_batch_job_lag_seconds",
		Help: "Seconds since last successful batch job run.",
	})

	// QdrantQueryDuration tracks Qdrant search query duration.
	QdrantQueryDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "codohue_qdrant_query_duration_seconds",
		Help:    "Qdrant search query duration in seconds.",
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.2, 0.5},
	}, []string{"namespace", "collection"})

	// RedisCacheRequests counts Redis cache hits and misses.
	RedisCacheRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "codohue_redis_cache_requests_total",
		Help: "Total Redis cache requests by result (hit/miss).",
	}, []string{"result"})

	// RecommendRequests counts recommendation requests by source strategy.
	RecommendRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "codohue_recommend_requests_total",
		Help: "Total recommendation requests by source.",
	}, []string{"namespace", "source"})

	// BatchEntitiesProcessed tracks the number of entities processed per batch run.
	// For CF runs this counts subjects; future re-embed runs would count items.
	// Mirrors batch_run_logs.entities_processed column (migration 014).
	BatchEntitiesProcessed = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "codohue_batch_entities_processed",
		Help: "Number of entities processed in the last batch run.",
	}, []string{"namespace"})

	// IDMappingErrors counts ID mapping lookup/create errors by entity type.
	IDMappingErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "codohue_id_mapping_errors_total",
		Help: "Total ID mapping lookup/create errors.",
	}, []string{"entity_type"})

	// TrendingItemsTotal tracks the number of items with a trending score per namespace.
	TrendingItemsTotal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "codohue_trending_items_total",
		Help: "Number of items with a trending score, per namespace.",
	}, []string{"namespace"})

	// TrendingRequestsTotal counts GET /v1/trending requests by namespace.
	TrendingRequestsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "codohue_trending_requests_total",
		Help: "Total GET /v1/trending requests by namespace.",
	}, []string{"namespace"})

	// Catalog auto-embedding metrics (feature 004-catalog-embedder).

	// CatalogItemsEmbeddedTotal counts catalog items successfully embedded
	// per namespace + strategy. The strategy label set is identical to the
	// label set on duration / failures / work_volume so dashboards can join
	// across them.
	CatalogItemsEmbeddedTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "codohue_catalog_items_embedded_total",
		Help: "Total catalog items successfully embedded.",
	}, []string{"namespace", "strategy_id", "strategy_version"})

	// CatalogEmbedDuration is the per-item embed wall time histogram.
	// Buckets are tuned for the V1 in-process strategy (sub-millisecond to
	// hundreds of ms); future external-LLM strategies may exceed the top
	// bucket but Prometheus' "+Inf" bucket still captures them.
	CatalogEmbedDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "codohue_catalog_embed_duration_seconds",
		Help:    "Per-item catalog embed wall-clock duration in seconds.",
		Buckets: []float64{0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5},
	}, []string{"namespace", "strategy_id", "strategy_version"})

	// CatalogEmbedFailuresTotal counts embed failures by reason. Reason is
	// one of: "zero_norm", "dim_mismatch", "input_too_large", "transient",
	// "strategy_resolve", "qdrant", "idmap", "ensure_collections",
	// "mark_embedded", "max_attempts", "unknown".
	CatalogEmbedFailuresTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "codohue_catalog_embed_failures_total",
		Help: "Total catalog embed failures by reason.",
	}, []string{"namespace", "strategy_id", "strategy_version", "reason"})

	// CatalogStrategyWorkVolumeTotal is the strategy-specific work-volume
	// indicator. The `unit` label MUST be incrementally extensible without
	// metric redesign (FR-015 / SC-007): V1 ships unit="items"; future
	// external-LLM strategies will add unit="billed_tokens" and
	// unit="billed_cost_micro_usd" through the SAME metric.
	CatalogStrategyWorkVolumeTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "codohue_catalog_strategy_work_volume_total",
		Help: "Total strategy-specific work volume (units depend on strategy: items, billed_tokens, billed_cost_micro_usd, etc.).",
	}, []string{"namespace", "strategy_id", "strategy_version", "unit"})
)

// Register registers all Codohue metrics with the default Prometheus registry.
func Register() {
	mustRegisterFn(
		BatchJobLagSeconds,
		QdrantQueryDuration,
		RedisCacheRequests,
		RecommendRequests,
		BatchEntitiesProcessed,
		IDMappingErrors,
		TrendingItemsTotal,
		TrendingRequestsTotal,
		CatalogItemsEmbeddedTotal,
		CatalogEmbedDuration,
		CatalogEmbedFailuresTotal,
		CatalogStrategyWorkVolumeTotal,
	)
}
