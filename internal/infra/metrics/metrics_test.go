package metrics_test

import (
	"slices"
	"testing"

	"github.com/jarviisha/codohue/internal/infra/metrics"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestRegisterDoesNotPanic(t *testing.T) {
	// Use a fresh registry per test to avoid conflicts with the default registry.
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		metrics.BatchJobLagSeconds,
		metrics.QdrantQueryDuration,
		metrics.RedisCacheRequests,
		metrics.RecommendRequests,
		metrics.BatchEntitiesProcessed,
		metrics.IDMappingErrors,
	)
}

func TestRemediationMetricsUseBoundedLabels(t *testing.T) {
	registry := prometheus.NewRegistry()
	registry.MustRegister(
		metrics.StreamLength,
		metrics.StreamPending,
		metrics.StreamUndelivered,
		metrics.StreamRetentionFrontierMilliseconds,
		metrics.StreamTrimmedTotal,
		metrics.StreamRetentionErrorsTotal,
		metrics.StreamUnexpectedGroups,
		metrics.StreamReclaimedTotal,
		metrics.StreamReclaimCyclesTotal,
		metrics.StaleGenerationTotal,
		metrics.NamespaceLifecycleOperationsTotal,
		metrics.IDMappingRepairItems,
	)

	metrics.StreamLength.WithLabelValues("events", "").Set(10)
	metrics.StreamRetentionErrorsTotal.WithLabelValues("catalog", "", "pel").Inc()
	metrics.StreamReclaimCyclesTotal.WithLabelValues("embed", "tenant-a", "terminal").Inc()
	metrics.StaleGenerationTotal.WithLabelValues("event", "mismatch").Inc()
	metrics.NamespaceLifecycleOperationsTotal.WithLabelValues("delete", "success").Inc()
	metrics.IDMappingRepairItems.WithLabelValues("quarantined", "object").Set(1)

	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("gather remediation metrics: %v", err)
	}
	want := []string{
		"codohue_idmap_repair_items",
		"codohue_namespace_lifecycle_operations_total",
		"codohue_stale_generation_total",
		"codohue_stream_length",
		"codohue_stream_reclaim_cycles_total",
		"codohue_stream_retention_errors_total",
	}
	var got []string
	for _, family := range families {
		got = append(got, family.GetName())
	}
	for _, name := range want {
		if !slices.Contains(got, name) {
			t.Errorf("metric family %q was not gathered; got %v", name, got)
		}
	}
}

func TestCounterIncrements(t *testing.T) {
	metrics.RedisCacheRequests.WithLabelValues("hit").Inc()
	metrics.RedisCacheRequests.WithLabelValues("miss").Inc()
	metrics.RedisCacheRequests.WithLabelValues("miss").Inc()

	// Collect the metric and verify the counter values.
	ch := make(chan prometheus.Metric, 10)
	metrics.RedisCacheRequests.Collect(ch)
	close(ch)

	counts := map[string]float64{}
	for m := range ch {
		var d dto.Metric
		if err := m.Write(&d); err != nil {
			t.Fatalf("write metric: %v", err)
		}
		for _, lp := range d.GetLabel() {
			if lp.GetName() == "result" {
				counts[lp.GetValue()] = d.GetCounter().GetValue()
			}
		}
	}

	if counts["hit"] < 1 {
		t.Errorf("expected hit count >= 1, got %v", counts["hit"])
	}
	if counts["miss"] < 2 {
		t.Errorf("expected miss count >= 2, got %v", counts["miss"])
	}
}

func TestGaugeSet(t *testing.T) {
	metrics.BatchJobLagSeconds.Set(42)

	ch := make(chan prometheus.Metric, 1)
	metrics.BatchJobLagSeconds.Collect(ch)
	close(ch)

	var d dto.Metric
	m := <-ch
	if err := m.Write(&d); err != nil {
		t.Fatalf("write metric: %v", err)
	}
	if got := d.GetGauge().GetValue(); got != 42 {
		t.Errorf("expected gauge 42, got %v", got)
	}
}
