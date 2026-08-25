package recommend

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jarviisha/codohue/internal/core/namespace"
	"github.com/jarviisha/codohue/internal/core/nslifecycle"
)

func openTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	u := os.Getenv("DATABASE_URL")
	if u == "" {
		t.Skip("DATABASE_URL not set")
	}
	db, err := pgxpool.New(context.Background(), u)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func seedRecommendEvent(t *testing.T, db *pgxpool.Pool, ns, subjectID, objectID string, weight float64, occurredAt time.Time) {
	t.Helper()
	ensureNamespace(t, db, ns)
	_, err := db.Exec(context.Background(), `
		INSERT INTO events (namespace, subject_id, object_id, action, weight, occurred_at)
		VALUES ($1, $2, $3, 'VIEW', $4, $5)`,
		ns, subjectID, objectID, weight, occurredAt,
	)
	if err != nil {
		t.Fatalf("seedRecommendEvent: %v", err)
	}
}

func cleanupRecommendNS(t *testing.T, db *pgxpool.Pool, ns string) {
	t.Helper()
	ensureNamespace(t, db, ns)
	t.Cleanup(func() {
		db.Exec(context.Background(), //nolint:errcheck // test cleanup, failure is not critical
			`DELETE FROM events WHERE namespace = $1`, ns)
	})
}

func TestRepositoryCountInteractions(t *testing.T) {
	db := openTestDB(t)
	cleanupRecommendNS(t, db, "rec_test_count")

	repo := NewRepository(db)
	ctx := context.Background()
	now := time.Now()

	// No events yet — count should be 0.
	n, err := repo.CountInteractions(ctx, "rec_test_count", "user-1")
	if err != nil {
		t.Fatalf("CountInteractions: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0, got %d", n)
	}

	seedRecommendEvent(t, db, "rec_test_count", "user-1", "item-1", 1.0, now)
	seedRecommendEvent(t, db, "rec_test_count", "user-1", "item-2", 1.0, now)
	seedRecommendEvent(t, db, "rec_test_count", "user-2", "item-3", 1.0, now) // different subject

	n, err = repo.CountInteractions(ctx, "rec_test_count", "user-1")
	if err != nil {
		t.Fatalf("CountInteractions: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2, got %d", n)
	}
}

func TestRepositoryGetSeenItems(t *testing.T) {
	db := openTestDB(t)
	cleanupRecommendNS(t, db, "rec_test_seen")

	repo := NewRepository(db)
	ctx := context.Background()
	now := time.Now()
	old := now.Add(-40 * 24 * time.Hour) // outside 30-day window

	seedRecommendEvent(t, db, "rec_test_seen", "user-1", "recent-item", 1.0, now)
	seedRecommendEvent(t, db, "rec_test_seen", "user-1", "old-item", 1.0, old)

	items, err := repo.GetSeenItems(ctx, "rec_test_seen", "user-1", 30)
	if err != nil {
		t.Fatalf("GetSeenItems: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d: %v", len(items), items)
	}
	if items[0] != "recent-item" {
		t.Errorf("expected recent-item, got %q", items[0])
	}
}

func TestRepositoryGetSeenItems_Deduplicates(t *testing.T) {
	db := openTestDB(t)
	cleanupRecommendNS(t, db, "rec_test_seen_dedup")

	repo := NewRepository(db)
	ctx := context.Background()
	now := time.Now()

	// Two events for the same object — should appear only once.
	seedRecommendEvent(t, db, "rec_test_seen_dedup", "user-1", "item-a", 1.0, now)
	seedRecommendEvent(t, db, "rec_test_seen_dedup", "user-1", "item-a", 2.0, now)

	items, err := repo.GetSeenItems(ctx, "rec_test_seen_dedup", "user-1", 30)
	if err != nil {
		t.Fatalf("GetSeenItems: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("expected 1 deduplicated item, got %d: %v", len(items), items)
	}
}

func TestRepositoryGetPopularItems(t *testing.T) {
	db := openTestDB(t)
	cleanupRecommendNS(t, db, "rec_test_popular")

	repo := NewRepository(db)
	ctx := context.Background()
	now := time.Now()
	old := now.Add(-10 * 24 * time.Hour) // still within 7 days? No, 10 days is outside 7-day window

	// Two events for item-hot (recent), one for item-warm (recent), one old event for item-cold.
	seedRecommendEvent(t, db, "rec_test_popular", "user-1", "item-hot", 2.0, now)
	seedRecommendEvent(t, db, "rec_test_popular", "user-2", "item-hot", 3.0, now)
	seedRecommendEvent(t, db, "rec_test_popular", "user-1", "item-warm", 1.0, now)
	seedRecommendEvent(t, db, "rec_test_popular", "user-1", "item-cold", 5.0, old) // outside 7-day window

	items, err := repo.GetPopularItems(ctx, "rec_test_popular", 10)
	if err != nil {
		t.Fatalf("GetPopularItems: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items (recent only), got %d: %v", len(items), items)
	}
	if items[0] != "item-hot" {
		t.Errorf("expected item-hot first (highest score), got %q", items[0])
	}
}

func TestRepositoryGetPopularItems_Empty(t *testing.T) {
	db := openTestDB(t)
	cleanupRecommendNS(t, db, "rec_test_popular_empty")

	repo := NewRepository(db)
	items, err := repo.GetPopularItems(context.Background(), "rec_test_popular_empty", 10)
	if err != nil {
		t.Fatalf("GetPopularItems: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected empty result, got %v", items)
	}
}

// Attribution lives in the objects table (migration 021), independent of
// catalog_items — that independence is the point, so the fixture writes only
// there and never creates a catalog row.
func seedAuthoredItem(t *testing.T, db *pgxpool.Pool, ns, objectID, author string, createdAt time.Time) {
	t.Helper()
	ensureNamespace(t, db, ns)
	_, err := db.Exec(context.Background(), `
		INSERT INTO objects (namespace, object_id, author_subject_id, created_at, updated_at)
		VALUES ($1, $2, NULLIF($3, ''), $4, $4)`,
		ns, objectID, author, createdAt,
	)
	if err != nil {
		t.Fatalf("seedAuthoredItem: %v", err)
	}
}

func cleanupCatalogNS(t *testing.T, db *pgxpool.Pool, ns string) {
	t.Helper()
	t.Cleanup(func() {
		db.Exec(context.Background(), //nolint:errcheck // test cleanup, failure is not critical
			`DELETE FROM objects WHERE namespace = $1`, ns)
	})
}

func TestRepositoryGetAuthoredObjects(t *testing.T) {
	db := openTestDB(t)
	const ns = "rec_test_authored"
	cleanupCatalogNS(t, db, ns)

	repo := NewRepository(db)
	ctx := context.Background()
	now := time.Now()

	// Nothing authored yet.
	got, truncated, err := repo.GetAuthoredObjects(ctx, ns, "u1")
	if err != nil {
		t.Fatalf("GetAuthoredObjects: %v", err)
	}
	if len(got) != 0 || truncated {
		t.Errorf("expected empty/untruncated, got %v truncated=%v", got, truncated)
	}

	seedAuthoredItem(t, db, ns, "o_old", "u1", now.Add(-2*time.Hour))
	seedAuthoredItem(t, db, ns, "o_new", "u1", now)
	seedAuthoredItem(t, db, ns, "o_other", "u2", now)
	seedAuthoredItem(t, db, ns, "o_none", "", now) // unattributed

	got, truncated, err = repo.GetAuthoredObjects(ctx, ns, "u1")
	if err != nil {
		t.Fatalf("GetAuthoredObjects: %v", err)
	}
	if truncated {
		t.Error("expected truncated=false")
	}
	// Newest first, and scoped to the requested author only.
	if len(got) != 2 || got[0] != "o_new" || got[1] != "o_old" {
		t.Fatalf("got %v, want [o_new o_old]", got)
	}
}

func TestRepositoryGetAuthoredObjects_Truncates(t *testing.T) {
	db := openTestDB(t)
	const ns = "rec_test_authored_cap"
	cleanupCatalogNS(t, db, ns)

	orig := authoredObjectsCap
	authoredObjectsCap = 2
	t.Cleanup(func() { authoredObjectsCap = orig })

	now := time.Now()
	for i, id := range []string{"a", "b", "c"} {
		seedAuthoredItem(t, db, ns, id, "u1", now.Add(-time.Duration(i)*time.Minute))
	}

	got, truncated, err := NewRepository(db).GetAuthoredObjects(context.Background(), ns, "u1")
	if err != nil {
		t.Fatalf("GetAuthoredObjects: %v", err)
	}
	if !truncated {
		t.Error("expected truncated=true when more rows exist than the cap")
	}
	if len(got) != 2 {
		t.Fatalf("expected the result trimmed to the cap, got %v", got)
	}
	// The cap must keep the newest, since those are the ones likely to rank.
	if got[0] != "a" || got[1] != "b" {
		t.Errorf("got %v, want the two newest [a b]", got)
	}
}

// A namespace nobody has attributed anything in simply has no authored
// objects — the query must return empty, not error.
func TestRepositoryGetAuthoredObjects_NonCatalogNamespace(t *testing.T) {
	db := openTestDB(t)
	got, truncated, err := NewRepository(db).GetAuthoredObjects(
		context.Background(), "rec_test_no_catalog_rows", "u1")
	if err != nil {
		t.Fatalf("GetAuthoredObjects: %v", err)
	}
	if len(got) != 0 || truncated {
		t.Errorf("expected empty result, got %v truncated=%v", got, truncated)
	}
}

// ─── generation-qualified physical names ─────────────────────────────────────

// The recommendation cache and the trending ZSET are Redis keys the recompute
// job wrote under the namespace's generation. Reading them under any other
// qualification serves a recreated namespace the previous incarnation's
// results — the exact failure the generation was introduced to prevent.
func TestPhysicalNamespaces_MatchLifecycleResolver(t *testing.T) {
	for _, generation := range []int64{0, 1, 2, 17} {
		cfg := &namespace.Config{Namespace: "tenant", Generation: generation}
		want := max(generation, 1)

		gotRedis := redisPhysicalNamespace("tenant", cfg)
		if wantRedis := nslifecycle.RedisNamespace("tenant", want); gotRedis != wantRedis {
			t.Errorf("generation=%d redis namespace: got %q, want %q", generation, gotRedis, wantRedis)
		}
		gotQdrant := qdrantPhysicalNamespace("tenant", cfg)
		if wantQdrant := nslifecycle.QdrantNamespace("tenant", want); gotQdrant != wantQdrant {
			t.Errorf("generation=%d qdrant namespace: got %q, want %q", generation, gotQdrant, wantQdrant)
		}
	}

	// A nil config is the "config unavailable" path; it must not silently
	// resolve to some other generation's keys.
	if got := redisPhysicalNamespace("tenant", nil); got != "tenant" {
		t.Errorf("nil config redis namespace: got %q, want tenant", got)
	}
}

// Cache keys of two generations must not collide, and generation 1 must keep
// the key shape that is already live in Redis.
func TestRecCacheKey_SeparatesGenerations(t *testing.T) {
	legacy := recCacheKey("tenant", 1, "u1", 10, 0)
	recreated := recCacheKey("tenant", 2, "u1", 10, 0)
	if legacy == recreated {
		t.Errorf("generations share cache key %q", legacy)
	}
	// Generation 1 must keep the key shape that is already live in Redis.
	if want := "rec:v2:dGVuYW50:dTE:limit=10:offset=0"; legacy != want {
		t.Errorf("generation 1 key changed: got %q, want %q", legacy, want)
	}
	// Paging and subject stay part of the key regardless of generation.
	if recCacheKey("tenant", 2, "u1", 10, 0) == recCacheKey("tenant", 2, "u1", 10, 10) {
		t.Error("offset must remain part of the cache key")
	}
}

// The trending key the serving path reads must be byte-identical to the one
// cmd/cron writes for that generation.
func TestTrendingKey_MatchesWriterForEveryGeneration(t *testing.T) {
	for _, generation := range []int64{1, 2, 9} {
		physical := redisPhysicalNamespace("tenant", &namespace.Config{Generation: generation})
		read := nslifecycle.MustPhysicalName(nslifecycle.KindTrending, physical, 1)
		written := nslifecycle.MustPhysicalName(nslifecycle.KindTrending, "tenant", generation)
		if read != written {
			t.Errorf("generation=%d: serving reads %q, cron writes %q", generation, read, written)
		}
	}
}

// ensureNamespace creates the rows a namespace-scoped row depends on.
//
// Migration 025 gave the data tables a foreign key onto namespace_configs,
// which in turn references namespace_lifecycles (024). Seeding a row for an
// invented namespace is therefore an FK error rather than a row — and these
// tests skip unless DATABASE_URL is set, so the break went unseen.
func ensureNamespace(t *testing.T, db *pgxpool.Pool, ns string) {
	t.Helper()
	ctx := context.Background()
	if _, err := db.Exec(ctx, `
		INSERT INTO namespace_lifecycles (namespace, generation, state, activated_at)
		VALUES ($1, 1, 'active', NOW()) ON CONFLICT (namespace) DO NOTHING`, ns); err != nil {
		t.Fatalf("ensure namespace lifecycle %q: %v", ns, err)
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO namespace_configs (namespace) VALUES ($1)
		ON CONFLICT (namespace) DO NOTHING`, ns); err != nil {
		t.Fatalf("ensure namespace config %q: %v", ns, err)
	}
	t.Cleanup(func() {
		clean := context.Background()
		db.Exec(clean, `DELETE FROM namespace_configs WHERE namespace = $1`, ns)    //nolint:errcheck // test cleanup
		db.Exec(clean, `DELETE FROM namespace_lifecycles WHERE namespace = $1`, ns) //nolint:errcheck // test cleanup
	})
}
