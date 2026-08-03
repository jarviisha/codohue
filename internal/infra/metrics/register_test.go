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

	if gotCount != 21 {
		t.Fatalf("expected 21 collectors, got %d", gotCount)
	}
}
