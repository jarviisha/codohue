package nslifecycle

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// testPool connects to the migrated database, or skips. These tests exercise
// the SQL itself: the sibling tests in this package drive a fake store, which
// is precisely why a wrong literal in the INSERT went unnoticed.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set")
	}
	db, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	t.Cleanup(db.Close)
	return db
}

func dropNamespace(t *testing.T, db *pgxpool.Pool, namespace string) {
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = db.Exec(ctx, `DELETE FROM namespace_configs WHERE namespace = $1`, namespace)
		_, _ = db.Exec(ctx, `DELETE FROM namespace_lifecycles WHERE namespace = $1`, namespace)
	})
}

// A generation-1 namespace has exactly one incarnation, so an envelope naming
// no generation can only mean that one. The documented Redis Streams transport
// publishes exactly that shape, so seeding the gate closed makes every stream
// event for a newly created namespace get acked and dropped — silently, with
// nothing returned to the producer.
func TestActivate_NewNamespaceAcceptsLegacyEnvelopes(t *testing.T) {
	db := testPool(t)
	repo := NewRepository(db)
	namespace := "legacy_gate_new_namespace_test"
	dropNamespace(t, db, namespace)

	lifecycle, err := repo.Activate(context.Background(), namespace)
	if err != nil {
		t.Fatalf("activate: %v", err)
	}
	if lifecycle.Generation != 1 {
		t.Fatalf("generation = %d, want 1", lifecycle.Generation)
	}
	if !lifecycle.LegacyMessagesAllowed {
		t.Error("a fresh generation-1 namespace refuses generation-less envelopes, " +
			"which silently drops the entire Redis Streams transport")
	}

	// And the evaluator agrees — the column is only worth anything through it.
	locker, err := NewPostgresLocker(db)
	if err != nil {
		t.Fatalf("new locker: %v", err)
	}
	t.Cleanup(locker.Close)
	svc := NewService(repo, locker)
	disposition, err := svc.EvaluateEnvelope(context.Background(), namespace, nil)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if disposition != EnvelopeProcess {
		t.Errorf("legacy envelope disposition = %v, want EnvelopeProcess", disposition)
	}
}

// Past generation 1 the same envelope is ambiguous: it could belong to the
// incarnation that was deleted. Recreation must close the gate.
func TestActivate_RecreatedNamespaceRefusesLegacyEnvelopes(t *testing.T) {
	db := testPool(t)
	repo := NewRepository(db)
	ctx := context.Background()
	namespace := "legacy_gate_recreated_namespace_test"
	dropNamespace(t, db, namespace)

	first, err := repo.Activate(ctx, namespace)
	if err != nil {
		t.Fatalf("activate: %v", err)
	}
	if _, err := repo.StartDelete(ctx, namespace); err != nil {
		t.Fatalf("start delete: %v", err)
	}
	if err := repo.CompleteDelete(ctx, namespace, first.Generation); err != nil {
		t.Fatalf("complete delete: %v", err)
	}

	second, err := repo.Activate(ctx, namespace)
	if err != nil {
		t.Fatalf("recreate: %v", err)
	}
	if second.Generation != 2 {
		t.Fatalf("generation = %d, want 2", second.Generation)
	}
	if second.LegacyMessagesAllowed {
		t.Error("a recreated namespace still accepts generation-less envelopes, " +
			"so messages aimed at the deleted incarnation land in the new one")
	}
}

// The fleet-wide gate closes once, deliberately, against adoption evidence. A
// namespace created afterwards must not reopen what that decision closed.
func TestActivate_RespectsTheClosedGlobalLegacyGate(t *testing.T) {
	db := testPool(t)
	repo := NewRepository(db)
	ctx := context.Background()
	namespace := "legacy_gate_after_global_close_test"
	dropNamespace(t, db, namespace)

	var disabledAt *string
	if err := db.QueryRow(ctx, `
		SELECT legacy_envelopes_disabled_at::text FROM system_lifecycle WHERE singleton = TRUE`).
		Scan(&disabledAt); err != nil {
		t.Fatalf("read system lifecycle: %v", err)
	}
	if disabledAt != nil {
		t.Skip("global legacy gate already closed in this environment")
	}

	// Close it, assert, then restore — this row is a singleton shared by every
	// other test in the database.
	if _, err := db.Exec(ctx, `
		UPDATE system_lifecycle
		SET legacy_envelopes_disabled_at = NOW(), legacy_adoption_evidence = 'test'
		WHERE singleton = TRUE`); err != nil {
		t.Fatalf("close global gate: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `
			UPDATE system_lifecycle
			SET legacy_envelopes_disabled_at = NULL, legacy_adoption_evidence = NULL
			WHERE singleton = TRUE`)
	})

	lifecycle, err := repo.Activate(ctx, namespace)
	if err != nil {
		t.Fatalf("activate: %v", err)
	}
	if lifecycle.LegacyMessagesAllowed {
		t.Error("a namespace created after the global gate closed reopened it")
	}
}

// TestPostgresLockerDoesNotDeadlockASaturatedPool is the regression for a
// permanent API hang.
//
// A fenced write used to take the global lock and the namespace lock on two
// separate pooled connections. With N concurrent writers against a pool of N,
// every writer took the global lock — exhausting the pool — and then blocked
// forever waiting for a connection to take the namespace lock, which only
// another blocked writer could release. Nothing timed out: requests hung until
// the client gave up.
//
// It only appeared where the pool was small. pgxpool defaults MaxConns to
// max(4, NumCPU), so a 16-core dev box never reproduced what a 4-vCPU CI runner
// hit every time. MaxConns is pinned to 4 here so the machine does not decide
// whether the bug is visible.
func TestPostgresLockerDoesNotDeadlockASaturatedPool(t *testing.T) {
	db := testPool(t)
	cfg := db.Config().Copy()
	cfg.MaxConns = 4
	saturated, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("build saturated pool: %v", err)
	}
	defer saturated.Close()

	locker, err := NewPostgresLocker(saturated)
	if err != nil {
		t.Fatalf("new locker: %v", err)
	}
	defer locker.Close()

	const writers = 4
	var acquired atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := context.Background()
			lock, err := locker.AcquirePair(ctx, "deadlock_probe", LockShared)
			if err != nil {
				t.Errorf("AcquirePair: %v", err)
				return
			}
			acquired.Add(1)
			// Hold both locks while doing work on the *caller's* pool. This is
			// the second half of the original cycle: a lock holder that cannot
			// get a work connection is just as stuck.
			var one int
			if err := saturated.QueryRow(ctx, `SELECT 1`).Scan(&one); err != nil {
				t.Errorf("work query while holding the pair: %v", err)
			}
			if err := lock.Release(ctx); err != nil {
				t.Errorf("Release: %v", err)
			}
		}()
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatalf("deadlock: %d/%d writers took the lock pair on a %d-connection pool",
			acquired.Load(), writers, cfg.MaxConns)
	}
}

// A released pair must leave nothing behind on the session it borrowed, or the
// next request to draw that connection inherits locks it never took.
func TestPostgresLockerReleasesBothKeys(t *testing.T) {
	db := testPool(t)
	locker, err := NewPostgresLocker(db)
	if err != nil {
		t.Fatalf("new locker: %v", err)
	}
	defer locker.Close()

	ctx := context.Background()
	lock, err := locker.AcquirePair(ctx, "release_probe", LockExclusive)
	if err != nil {
		t.Fatalf("AcquirePair: %v", err)
	}
	if err := lock.Release(ctx); err != nil {
		t.Fatalf("Release: %v", err)
	}
	// Release is idempotent — a second call must not return the connection twice.
	if err := lock.Release(ctx); err != nil {
		t.Fatalf("second Release: %v", err)
	}

	var held int
	if err := db.QueryRow(ctx, `
		SELECT count(*) FROM pg_locks
		WHERE locktype = 'advisory' AND objid = $1::bigint & 4294967295`,
		lockKey("namespace:release_probe")).Scan(&held); err != nil {
		t.Fatalf("count advisory locks: %v", err)
	}
	if held != 0 {
		t.Errorf("%d advisory lock(s) still held after release", held)
	}

	// The exclusive lock must be re-takeable immediately; if the first pair
	// leaked, this blocks until the test times out.
	again, err := locker.AcquirePair(ctx, "release_probe", LockExclusive)
	if err != nil {
		t.Fatalf("re-acquire after release: %v", err)
	}
	if err := again.Release(ctx); err != nil {
		t.Fatalf("release re-acquired pair: %v", err)
	}
}
