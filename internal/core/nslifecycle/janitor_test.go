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
	// asked records the cursor each pass resumed from.
	asked []CleanupCandidate
	// paged serves candidates as a real keyset page when set.
	paged bool
}

func (s *janitorSource) GetSystem(context.Context) (*SystemLifecycle, error) { return s.system, s.err }
func (s *janitorSource) ListCleanupCandidates(_ context.Context, after CleanupCandidate, limit int) ([]CleanupCandidate, error) {
	s.asked = append(s.asked, after)
	if s.err != nil {
		return nil, s.err
	}
	if !s.paged {
		return s.candidates, nil
	}
	var page []CleanupCandidate
	for _, candidate := range s.candidates {
		if candidate.Namespace > after.Namespace ||
			(candidate.Namespace == after.Namespace && candidate.Generation > after.Generation) {
			page = append(page, candidate)
		}
		if len(page) == limit {
			break
		}
	}
	return page, nil
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

// TestJanitorWalksEveryCandidateAcrossPasses pins the reason the cursor exists:
// nothing marks a generation reclaimed, so a fixed LIMIT would re-clean the
// first page forever and never reach the tail.
func TestJanitorWalksEveryCandidateAcrossPasses(t *testing.T) {
	now := time.Now()
	all := []CleanupCandidate{
		{Namespace: "a", Generation: 1},
		{Namespace: "a", Generation: 2},
		{Namespace: "b", Generation: 1},
		{Namespace: "c", Generation: 1},
		{Namespace: "c", Generation: 2},
	}
	source := &janitorSource{
		system:     &SystemLifecycle{State: SystemActive, LegacyEnvelopesDisabledAt: &now},
		candidates: all,
		paged:      true,
	}
	cleaner := &janitorCleaner{}
	janitor := NewJanitor(source, cleaner)

	for pass := 0; pass < 3; pass++ {
		if _, err := janitor.RunOnce(context.Background(), 2); err != nil {
			t.Fatalf("pass %d: %v", pass, err)
		}
	}

	if len(cleaner.qdrant) != len(all) {
		t.Fatalf("cleaned %d candidates, want all %d", len(cleaner.qdrant), len(all))
	}
	for i, want := range all {
		if cleaner.qdrant[i] != want {
			t.Errorf("pass order[%d] = %v, want %v", i, cleaner.qdrant[i], want)
		}
	}

	// The final short page rewinds, so a generation superseded later is picked
	// up rather than stranded behind an exhausted cursor.
	if _, err := janitor.RunOnce(context.Background(), 2); err != nil {
		t.Fatalf("wrap pass: %v", err)
	}
	if got := source.asked[len(source.asked)-1]; got != (CleanupCandidate{}) {
		t.Errorf("cursor after a short page = %v, want a rewind to the start", got)
	}
}

// TestJanitorResumesOnTheFailedCandidate keeps a mid-page failure from being
// skipped by the cursor on the following pass.
func TestJanitorResumesOnTheFailedCandidate(t *testing.T) {
	now := time.Now()
	source := &janitorSource{
		system:     &SystemLifecycle{State: SystemActive, LegacyEnvelopesDisabledAt: &now},
		candidates: []CleanupCandidate{{Namespace: "a", Generation: 1}, {Namespace: "b", Generation: 1}},
		paged:      true,
	}
	cleaner := &janitorCleaner{qdrantErr: errors.New("qdrant down")}
	janitor := NewJanitor(source, cleaner)

	if cleaned, err := janitor.RunOnce(context.Background(), 2); err == nil || cleaned != 0 {
		t.Fatalf("cleaned=%d err=%v", cleaned, err)
	}
	cleaner.qdrantErr = nil
	if _, err := janitor.RunOnce(context.Background(), 2); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if got := source.asked[1]; got != (CleanupCandidate{}) {
		t.Fatalf("retry resumed from %v, want the failed candidate still in range", got)
	}
	if len(cleaner.qdrant) != 3 || cleaner.qdrant[1].Namespace != "a" {
		t.Fatalf("retry did not revisit the failed candidate: %v", cleaner.qdrant)
	}
}
