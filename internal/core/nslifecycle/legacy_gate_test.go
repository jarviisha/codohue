package nslifecycle

import (
	"context"
	"os"
	"testing"

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
	svc := NewService(repo, NewPostgresLocker(db))
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
