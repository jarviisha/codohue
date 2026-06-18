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
	// Mirrors batch_run_logs.entities_processed column (migration 012).
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

	// EventsIngestedTotal counts behavioral events successfully persisted,
	// sliced by namespace + action. Lives in cmd/api's process; the admin
	// plane derives ingest rates from its own event-tail counter, not from
	// this metric (the two run in separate processes).
	EventsIngestedTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "codohue_events_ingested_total",
		Help: "Total behavioral events successfully ingested, by namespace + action.",
	}, []string{"namespace", "action"})

	// IngestErrorsTotal counts events rejected or failed during ingest, by
	// reason. `reason` is one of: "invalid_payload", "unknown_action",
	// "insert" (DB write failed), "decode" (malformed request body).
	IngestErrorsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "codohue_ingest_errors_total",
		Help: "Total ingest errors by namespace + reason.",
	}, []string{"namespace", "reason"})

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

	// CatalogPendingItems is the live count of catalog_items in non-embedded
	// state (pending + in_flight + failed + dead_letter) per namespace.
	// Updated by cmd/embedder's backlog sampler each tick. Operators wire
	// this in alerts ("backlog > 1000 for 5 min").
	CatalogPendingItems = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "codohue_catalog_pending_items",
		Help: "Live count of catalog_items still awaiting (pending+in_flight) or failed (failed+dead_letter), per namespace.",
	}, []string{"namespace", "state"})

	// CatalogConsumerLag is the Redis consumer group pending-entry-list size
	// for catalog:embed:{ns} — the number of XREADGROUP-delivered items the
	// embedder consumer hasn't ACKed yet. Spikes mean the worker is choking
	// or crashed mid-batch.
	CatalogConsumerLag = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "codohue_catalog_consumer_lag",
		Help: "Redis consumer-group PEL depth on catalog:embed:{ns} per namespace.",
	}, []string{"namespace"})

	// Admin-plane self-observability metrics. Live in their own
	// `codohue_admin_*` namespace so dashboards can split operator-facing
	// data-plane metrics from admin-process health (BUILD_PLAN §12.3).

	// AdminSSEConnectionsActive tracks how many SSE handlers are currently
	// streaming per stream kind. `stream` label values: "ops" (global ops
	// bus), "batch_run" (per-run lifecycle stream), "catalog" (per-ns
	// catalog stream), "ping" (foundation health-check stream).
	AdminSSEConnectionsActive = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "codohue_admin_sse_connections_active",
		Help: "Active SSE connections by stream kind on the admin plane.",
	}, []string{"stream"})

	// AdminSSEDroppedTotal counts events dropped on an SSE connection.
	// `reason` is either "backpressure" (subscriber buffer full — bus drops
	// oldest) or "client_slow" (Send failed writing to the client socket;
	// the handler closes the stream after the failure).
	AdminSSEDroppedTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "codohue_admin_sse_dropped_total",
		Help: "Total dropped events on admin SSE streams by stream + reason.",
	}, []string{"stream", "reason"})

	// AdminEventbusPublishTotal counts every event the in-process admin bus
	// fans out, by kind. Mirrors the bus's Publish call site exactly.
	AdminEventbusPublishTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "codohue_admin_eventbus_publish_total",
		Help: "Total events published on the admin event bus by kind.",
	}, []string{"kind"})

	// AdminEventbusSubscribersActive tracks how many subscribers are
	// currently attached to the admin event bus. One SSE connection = one
	// subscriber. A subscriber may filter to multiple kinds — we don't
	// label by kind because that would double-count.
	AdminEventbusSubscribersActive = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "codohue_admin_eventbus_subscribers_active",
		Help: "Active subscriber count on the admin event bus.",
	})
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
		EventsIngestedTotal,
		IngestErrorsTotal,
		CatalogItemsEmbeddedTotal,
		CatalogEmbedDuration,
		CatalogEmbedFailuresTotal,
		CatalogStrategyWorkVolumeTotal,
		CatalogPendingItems,
		CatalogConsumerLag,
		AdminSSEConnectionsActive,
		AdminSSEDroppedTotal,
		AdminEventbusPublishTotal,
		AdminEventbusSubscribersActive,
	)
}
