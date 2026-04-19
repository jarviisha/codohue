package metrics_test

import (
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
		metrics.BatchSubjectsProcessed,
		metrics.IDMappingErrors,
	)
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
