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

	// BatchSubjectsProcessed tracks the number of subjects processed per batch run.
	BatchSubjectsProcessed = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "codohue_batch_subjects_processed",
		Help: "Number of subjects processed in the last batch run.",
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
)

// Register registers all Codohue metrics with the default Prometheus registry.
func Register() {
	mustRegisterFn(
		BatchJobLagSeconds,
		QdrantQueryDuration,
		RedisCacheRequests,
		RecommendRequests,
		BatchSubjectsProcessed,
		IDMappingErrors,
		TrendingItemsTotal,
		TrendingRequestsTotal,
	)
}
