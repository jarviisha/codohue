package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	goredis "github.com/redis/go-redis/v9"

	"github.com/jarviisha/codohue/internal/admin"
)

// catalogStateCounter is the Postgres-side dependency of the backlog adapter.
// *admin.Repository satisfies it via CountCatalogItemStates; tests inject a fake.
type catalogStateCounter interface {
	CountCatalogItemStates(ctx context.Context, namespace string) (admin.CatalogItemStateCounts, error)
}

// catalogBacklogAdapter bridges admin.Service to admin.Repository (Postgres
// state counts) + Redis (XLEN of catalog:embed:{ns}) for the catalog backlog
// snapshot panel. Lives in cmd/admin because Redis is a wiring-layer concern
// here — admin domain already imports go-redis but the stream key convention
// is owned by the catalog/embedder feature, not the admin domain.
type catalogBacklogAdapter struct {
	counter catalogStateCounter
	redis   *goredis.Client
}

func newCatalogBacklogAdapter(counter catalogStateCounter, redis *goredis.Client) *catalogBacklogAdapter {
	return &catalogBacklogAdapter{counter: counter, redis: redis}
}

// Read returns the operational backlog snapshot for one namespace. Redis is
// optional: when nil or unavailable the stream_len count stays at zero so
// the admin panel still renders the Postgres-side state breakdown.
func (a *catalogBacklogAdapter) Read(ctx context.Context, namespace string) (admin.CatalogBacklog, error) {
	counts, err := a.counter.CountCatalogItemStates(ctx, namespace)
	if err != nil {
		return admin.CatalogBacklog{}, fmt.Errorf("count catalog item states: %w", err)
	}

	out := admin.CatalogBacklog{
		Pending:    counts.Pending,
		InFlight:   counts.InFlight,
		Embedded:   counts.Embedded,
		Failed:     counts.Failed,
		DeadLetter: counts.DeadLetter,
	}

	if a.redis != nil {
		stream := "catalog:embed:" + namespace

		n, err := a.redis.XLen(ctx, stream).Result()
		switch {
		case err == nil:
			out.StreamLen = int(n)
		case errors.Is(err, goredis.Nil):
			// Stream key does not exist yet → treat as empty.
		default:
			// Non-fatal: log and leave StreamLen=0 so the rest of the
			// panel still renders.
			slog.Debug("catalog backlog xlen failed", "namespace", namespace, "error", err)
		}

		// ConsumerLag = PEL depth of the embedder consumer group on this
		// stream. Same non-fatal envelope as XLEN — missing stream/group
		// is normal during cold start.
		if groups, err := a.redis.XInfoGroups(ctx, stream).Result(); err == nil {
			for _, g := range groups {
				if g.Name == "embedder" {
					out.ConsumerLag = int(g.Pending)
					break
				}
			}
		} else if !errors.Is(err, goredis.Nil) {
			slog.Debug("catalog backlog xinfo groups failed", "namespace", namespace, "error", err)
		}
	}

	return out, nil
}
