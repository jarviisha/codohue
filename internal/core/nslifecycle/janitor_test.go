package nslifecycle

import (
	"context"
	"errors"
	"testing"
	"time"
)

type janitorSource struct {
	system     *SystemLifecycle
	candidates []CleanupCandidate
	err        error
}

func (s *janitorSource) GetSystem(context.Context) (*SystemLifecycle, error) { return s.system, s.err }
func (s *janitorSource) ListCleanupCandidates(context.Context, int) ([]CleanupCandidate, error) {
	return s.candidates, s.err
}

type janitorCleaner struct {
	redis, qdrant       []CleanupCandidate
	redisErr, qdrantErr error
}

func (c *janitorCleaner) DeleteRedisGeneration(_ context.Context, candidate CleanupCandidate) error {
	c.redis = append(c.redis, candidate)
	return c.redisErr
}
func (c *janitorCleaner) DeleteQdrantGeneration(_ context.Context, candidate CleanupCandidate) error {
	c.qdrant = append(c.qdrant, candidate)
	return c.qdrantErr
}

func TestJanitorRequiresClosedLegacyGateAndIsBounded(t *testing.T) {
	source := &janitorSource{system: &SystemLifecycle{State: SystemActive}, candidates: []CleanupCandidate{{Namespace: "a", Generation: 1}}}
	cleaner := &janitorCleaner{}
	janitor := NewJanitor(source, cleaner)
	if _, err := janitor.RunOnce(context.Background(), 10); !errors.Is(err, ErrLegacyEnvelopesOpen) {
		t.Fatalf("error = %v", err)
	}
	if len(cleaner.redis)+len(cleaner.qdrant) != 0 {
		t.Fatal("janitor mutated with open gate")
	}

	now := time.Now()
	source.system.LegacyEnvelopesDisabledAt = &now
	source.candidates = []CleanupCandidate{{Namespace: "a", Generation: 1}, {Namespace: "b", Generation: 2}}
	cleaned, err := janitor.RunOnce(context.Background(), 1)
	if err != nil || cleaned != 1 {
		t.Fatalf("cleaned=%d err=%v", cleaned, err)
	}
	if len(cleaner.redis) != 1 || len(cleaner.qdrant) != 1 {
		t.Fatalf("redis=%v qdrant=%v", cleaner.redis, cleaner.qdrant)
	}
}

func TestJanitorStopsOnDependencyFailure(t *testing.T) {
	now := time.Now()
	source := &janitorSource{system: &SystemLifecycle{State: SystemActive, LegacyEnvelopesDisabledAt: &now}, candidates: []CleanupCandidate{{Namespace: "a", Generation: 1}}}
	cleaner := &janitorCleaner{qdrantErr: errors.New("qdrant down")}
	cleaned, err := NewJanitor(source, cleaner).RunOnce(context.Background(), 10)
	if err == nil || cleaned != 0 {
		t.Fatalf("cleaned=%d err=%v", cleaned, err)
	}
}
