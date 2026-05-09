package embedder

import (
	"context"
	"errors"
	"testing"

	"github.com/jarviisha/codohue/internal/core/embedstrategy"
)

// init() in strategy.go registers the V1 hashing strategy against
// embedstrategy.DefaultRegistry() at package import time. These tests
// verify that registration is visible and that the registered strategy
// behaves correctly through the public Registry surface.

func TestStrategyRegistration_HasV1(t *testing.T) {
	if !embedstrategy.DefaultRegistry().Has(hashingNgramsID, hashingNgramsVersion) {
		t.Fatalf("expected DefaultRegistry to have %s@%s registered after import",
			hashingNgramsID, hashingNgramsVersion)
	}
}

func TestStrategyRegistration_BuildAtEachValidDim(t *testing.T) {
	for _, dim := range hashingNgramsValidDims {
		dim := dim
		s, err := embedstrategy.DefaultRegistry().Build(
			hashingNgramsID, hashingNgramsVersion,
			embedstrategy.Params{"dim": dim},
		)
		if err != nil {
			t.Fatalf("dim=%d: Build error %v", dim, err)
		}
		if s.Dim() != dim {
			t.Errorf("dim=%d: built strategy has Dim()=%d", dim, s.Dim())
		}
		if s.ID() != hashingNgramsID {
			t.Errorf("dim=%d: ID()=%q", dim, s.ID())
		}
		if s.Version() != hashingNgramsVersion {
			t.Errorf("dim=%d: Version()=%q", dim, s.Version())
		}
	}
}

func TestStrategyRegistration_BuildRejectsMissingDim(t *testing.T) {
	_, err := embedstrategy.DefaultRegistry().Build(
		hashingNgramsID, hashingNgramsVersion,
		embedstrategy.Params{},
	)
	if err == nil {
		t.Fatal("expected error when dim missing from params")
	}
}

func TestStrategyRegistration_BuildRejectsInvalidDim(t *testing.T) {
	_, err := embedstrategy.DefaultRegistry().Build(
		hashingNgramsID, hashingNgramsVersion,
		embedstrategy.Params{"dim": 100},
	)
	if err == nil {
		t.Fatal("expected error for non-supported dim")
	}
}

func TestStrategyRegistration_ListExposesAllFourVariants(t *testing.T) {
	descriptors := embedstrategy.DefaultRegistry().List()
	gotDims := make(map[int]bool)
	for _, d := range descriptors {
		if d.ID == hashingNgramsID && d.Version == hashingNgramsVersion {
			gotDims[d.Dim] = true
		}
	}
	for _, want := range hashingNgramsValidDims {
		if !gotDims[want] {
			t.Errorf("expected variant dim=%d in registry list", want)
		}
	}
}

func TestStrategyRegistration_BuiltStrategyEmbedsAndIsDeterministic(t *testing.T) {
	s, err := embedstrategy.DefaultRegistry().Build(
		hashingNgramsID, hashingNgramsVersion,
		embedstrategy.Params{"dim": 128},
	)
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.Embed(context.Background(), "hello world")
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Embed(context.Background(), "hello world")
	if err != nil {
		t.Fatal(err)
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("non-deterministic at position %d: %v vs %v", i, a[i], b[i])
		}
	}
}

func TestStrategyRegistration_UnknownVersionReturnsSentinel(t *testing.T) {
	_, err := embedstrategy.DefaultRegistry().Build(
		hashingNgramsID, "v999-nonexistent",
		embedstrategy.Params{"dim": 128},
	)
	if !errors.Is(err, embedstrategy.ErrUnknownStrategy) {
		t.Fatalf("expected ErrUnknownStrategy, got %v", err)
	}
}
