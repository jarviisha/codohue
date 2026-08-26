package nslifecycle

import (
	"context"
	"fmt"
)

// JanitorSource is the read side of the ledger the janitor works from: it
// never derives cleanup candidates from what happens to exist in Redis or
// Qdrant, only from what the durable ledger says is superseded.
type JanitorSource interface {
	GetSystem(context.Context) (*SystemLifecycle, error)
	ListCleanupCandidates(context.Context, CleanupCandidate, int) ([]CleanupCandidate, error)
}

// GenerationCleaner removes one superseded generation's physical resources.
// Implemented at the wiring layer so this package keeps no store dependencies.
type GenerationCleaner interface {
	DeleteRedisGeneration(context.Context, CleanupCandidate) error
	DeleteQdrantGeneration(context.Context, CleanupCandidate) error
}

// Janitor removes only generations that the durable ledger identifies as old.
type Janitor struct {
	source  JanitorSource
	cleaner GenerationCleaner
	// cursor is the keyset position the next pass resumes from, so successive
	// bounded passes walk the whole candidate space instead of re-cleaning the
	// first page. It resets on a short page, making the walk cyclic.
	cursor CleanupCandidate
}

// NewJanitor wires the ledger read side to the physical cleaner.
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
	candidates, err := j.source.ListCleanupCandidates(ctx, j.cursor, limit)
	if err != nil {
		return 0, fmt.Errorf("list cleanup candidates: %w", err)
	}
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	// A short page means the walk reached the end; start the next pass over so
	// generations superseded in the meantime are picked up.
	if len(candidates) < limit {
		defer func() { j.cursor = CleanupCandidate{} }()
	}
	cleaned := 0
	for _, candidate := range candidates {
		if err := j.cleaner.DeleteRedisGeneration(ctx, candidate); err != nil {
			return cleaned, fmt.Errorf("clean Redis generation %s/%d: %w", candidate.Namespace, candidate.Generation, err)
		}
		if err := j.cleaner.DeleteQdrantGeneration(ctx, candidate); err != nil {
			return cleaned, fmt.Errorf("clean Qdrant generation %s/%d: %w", candidate.Namespace, candidate.Generation, err)
		}
		// Advance only past fully cleaned candidates so a failure mid-page
		// resumes on the one that failed rather than skipping it.
		j.cursor = candidate
		cleaned++
	}
	return cleaned, nil
}
