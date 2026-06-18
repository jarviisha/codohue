package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestRegister(t *testing.T) {
	orig := mustRegisterFn
	t.Cleanup(func() { mustRegisterFn = orig })

	var gotCount int
	mustRegisterFn = func(cs ...prometheus.Collector) {
		gotCount = len(cs)
	}

	Register()

	if gotCount != 20 {
		t.Fatalf("expected 20 collectors, got %d", gotCount)
	}
}
