package nslifecycle

import (
	"context"
	"fmt"
)

type JanitorSource interface {
	GetSystem(context.Context) (*SystemLifecycle, error)
	ListCleanupCandidates(context.Context, int) ([]CleanupCandidate, error)
}

type GenerationCleaner interface {
	DeleteRedisGeneration(context.Context, CleanupCandidate) error
	DeleteQdrantGeneration(context.Context, CleanupCandidate) error
}

// Janitor removes only generations that the durable ledger identifies as old.
type Janitor struct {
	source  JanitorSource
	cleaner GenerationCleaner
}

func NewJanitor(source JanitorSource, cleaner GenerationCleaner) *Janitor {
	return &Janitor{source: source, cleaner: cleaner}
}

// RunOnce is bounded by limit and refuses all mutation until legacy envelopes
// have been globally disabled.
func (j *Janitor) RunOnce(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		return 0, nil
	}
	system, err := j.source.GetSystem(ctx)
	if err != nil {
		return 0, fmt.Errorf("read janitor gate: %w", err)
	}
	if system.LegacyEnvelopesDisabledAt == nil {
		return 0, ErrLegacyEnvelopesOpen
	}
	candidates, err := j.source.ListCleanupCandidates(ctx, limit)
	if err != nil {
		return 0, fmt.Errorf("list cleanup candidates: %w", err)
	}
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	cleaned := 0
	for _, candidate := range candidates {
		if err := j.cleaner.DeleteRedisGeneration(ctx, candidate); err != nil {
			return cleaned, fmt.Errorf("clean Redis generation %s/%d: %w", candidate.Namespace, candidate.Generation, err)
		}
		if err := j.cleaner.DeleteQdrantGeneration(ctx, candidate); err != nil {
			return cleaned, fmt.Errorf("clean Qdrant generation %s/%d: %w", candidate.Namespace, candidate.Generation, err)
		}
		cleaned++
	}
	return cleaned, nil
}
