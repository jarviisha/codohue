package catalog

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type fakeRow struct {
	scanFn func(dest ...any) error
}

func (f fakeRow) Scan(dest ...any) error { return f.scanFn(dest...) }

func setString(dest any, v string) error {
	ptr, ok := dest.(*string)
	if !ok {
		return errors.New("expected *string")
	}
	*ptr = v
	return nil
}

func setInt64(dest any, v int64) error {
	ptr, ok := dest.(*int64)
	if !ok {
		return errors.New("expected *int64")
	}
	*ptr = v
	return nil
}

func setInt(dest any, v int) error {
	ptr, ok := dest.(*int)
	if !ok {
		return errors.New("expected *int")
	}
	*ptr = v
	return nil
}

func setBool(dest any, v bool) error {
	ptr, ok := dest.(*bool)
	if !ok {
		return errors.New("expected *bool")
	}
	*ptr = v
	return nil
}

func setBytes(dest any, v []byte) error {
	ptr, ok := dest.(*[]byte)
	if !ok {
		return errors.New("expected *[]byte")
	}
	*ptr = v
	return nil
}

func setState(dest any, v string) error {
	ptr, ok := dest.(*State)
	if !ok {
		return errors.New("expected *State")
	}
	*ptr = State(v)
	return nil
}

func setTime(dest any, v time.Time) error {
	ptr, ok := dest.(*time.Time)
	if !ok {
		return errors.New("expected *time.Time")
	}
	*ptr = v
	return nil
}

func setEmbeddedAtNil(dest any) error {
	ptr, ok := dest.(**time.Time)
	if !ok {
		return errors.New("expected **time.Time")
	}
	*ptr = nil
	return nil
}

// fillScanRow populates the 15-field scan row used by Repository.Upsert.
// Field positions match the SELECT in repository.go. Tests call this then
// override specific fields they care about (state, content_hash, needsPublish).
func fillScanRow(dest []any, contentHash, metadata []byte, state string, needsPublish bool, now time.Time) error {
	if len(dest) != 15 {
		return errors.New("expected 15 scan targets")
	}
	if err := setInt64(dest[0], 42); err != nil {
		return err
	}
	if err := setString(dest[1], "ns"); err != nil {
		return err
	}
	if err := setString(dest[2], "obj1"); err != nil {
		return err
	}
	if err := setString(dest[3], "hello world"); err != nil {
		return err
	}
	if err := setBytes(dest[4], contentHash); err != nil {
		return err
	}
	if err := setBytes(dest[5], metadata); err != nil {
		return err
	}
	if err := setState(dest[6], state); err != nil {
		return err
	}
	if err := setString(dest[7], ""); err != nil {
		return err
	}
	if err := setString(dest[8], ""); err != nil {
		return err
	}
	if err := setEmbeddedAtNil(dest[9]); err != nil {
		return err
	}
	if err := setInt(dest[10], 0); err != nil {
		return err
	}
	if err := setString(dest[11], ""); err != nil {
		return err
	}
	if err := setTime(dest[12], now); err != nil {
		return err
	}
	if err := setTime(dest[13], now); err != nil {
		return err
	}
	return setBool(dest[14], needsPublish)
}

func TestNewRepository(t *testing.T) {
	if NewRepository(nil) == nil {
		t.Fatal("expected repository")
	}
}

func TestContentHash_Determinism(t *testing.T) {
	a := ContentHash("the quick brown fox")
	b := ContentHash("the quick brown fox")
	if !bytes.Equal(a, b) {
		t.Fatalf("ContentHash not deterministic: %x vs %x", a, b)
	}
	if len(a) != 32 {
		t.Fatalf("expected 32-byte sha256, got %d", len(a))
	}
}

func TestContentHash_DifferentContentDiffers(t *testing.T) {
	a := ContentHash("hello")
	b := ContentHash("hello!")
	if bytes.Equal(a, b) {
		t.Fatal("ContentHash collision on trivially different inputs")
	}
}

func TestRepositoryUpsert_QueryError(t *testing.T) {
	repo := &Repository{
		queryRowFn: func(_ context.Context, _ string, _ ...any) rowScanner {
			return fakeRow{scanFn: func(_ ...any) error { return errors.New("query failed") }}
		},
	}
	_, err := repo.Upsert(context.Background(), "ns", "obj1", "hello", ContentHash("hello"), nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRepositoryUpsert_FreshInsertNeedsPublish(t *testing.T) {
	now := time.Now()
	hash := ContentHash("hello world")
	repo := &Repository{
		queryRowFn: func(_ context.Context, _ string, _ ...any) rowScanner {
			return fakeRow{scanFn: func(dest ...any) error {
				return fillScanRow(dest, hash, []byte("{}"), "pending", true, now)
			}}
		},
	}
	res, err := repo.Upsert(context.Background(), "ns", "obj1", "hello world", hash, nil)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if !res.NeedsPublish {
		t.Error("expected NeedsPublish=true on fresh insert")
	}
	if res.Item.State != StatePending {
		t.Errorf("expected pending state, got %s", res.Item.State)
	}
	if !bytes.Equal(res.Item.ContentHash, hash) {
		t.Errorf("content hash mismatch")
	}
}

func TestRepositoryUpsert_IdempotentSameContent(t *testing.T) {
	now := time.Now()
	hash := ContentHash("hello world")
	repo := &Repository{
		queryRowFn: func(_ context.Context, _ string, _ ...any) rowScanner {
			return fakeRow{scanFn: func(dest ...any) error {
				return fillScanRow(dest, hash, []byte("{}"), "embedded", false, now)
			}}
		},
	}
	res, err := repo.Upsert(context.Background(), "ns", "obj1", "hello world", hash, nil)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if res.NeedsPublish {
		t.Error("expected NeedsPublish=false when content hash unchanged")
	}
	if res.Item.State != StateEmbedded {
		t.Errorf("expected state to remain 'embedded' on idempotent re-ingest, got %s", res.Item.State)
	}
}

func TestRepositoryUpsert_NewContentResetsState(t *testing.T) {
	now := time.Now()
	hash := ContentHash("brand new content")
	repo := &Repository{
		queryRowFn: func(_ context.Context, _ string, _ ...any) rowScanner {
			return fakeRow{scanFn: func(dest ...any) error {
				return fillScanRow(dest, hash, []byte("{}"), "pending", true, now)
			}}
		},
	}
	res, err := repo.Upsert(context.Background(), "ns", "obj1", "brand new content", hash, nil)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if !res.NeedsPublish {
		t.Error("expected NeedsPublish=true when content hash differs")
	}
	if res.Item.State != StatePending {
		t.Errorf("expected state to reset to 'pending' on new content, got %s", res.Item.State)
	}
	if res.Item.AttemptCount != 0 {
		t.Errorf("expected attempt_count reset to 0, got %d", res.Item.AttemptCount)
	}
}

func TestRepositoryUpsert_MetadataRoundTrip(t *testing.T) {
	now := time.Now()
	hash := ContentHash("hello")
	meta := []byte(`{"lang":"vi","tags":"news"}`)
	var gotArgs []any
	repo := &Repository{
		queryRowFn: func(_ context.Context, _ string, args ...any) rowScanner {
			gotArgs = args
			return fakeRow{scanFn: func(dest ...any) error {
				return fillScanRow(dest, hash, meta, "pending", true, now)
			}}
		},
	}
	res, err := repo.Upsert(context.Background(), "ns", "obj1", "hello", hash,
		map[string]any{"lang": "vi", "tags": "news"})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	// Attribution moved to the objects table, so it must not appear here.
	if len(gotArgs) != 5 {
		t.Errorf("expected 5 bind args after author moved out, got %v", gotArgs)
	}
	if res.Item.Metadata["lang"] != "vi" || res.Item.Metadata["tags"] != "news" {
		t.Errorf("metadata round-trip: got %v", res.Item.Metadata)
	}
}

func TestRepositoryUpsert_MalformedMetadataReturnsError(t *testing.T) {
	now := time.Now()
	hash := ContentHash("hello")
	repo := &Repository{
		queryRowFn: func(_ context.Context, _ string, _ ...any) rowScanner {
			return fakeRow{scanFn: func(dest ...any) error {
				return fillScanRow(dest, hash, []byte("not-json"), "pending", true, now)
			}}
		},
	}
	_, err := repo.Upsert(context.Background(), "ns", "obj1", "hello", hash, nil)
	if err == nil {
		t.Fatal("expected error on malformed metadata")
	}
}

func TestMarshalMetadata_NilProducesEmptyObject(t *testing.T) {
	b, err := marshalMetadata(nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "{}" {
		t.Fatalf("expected '{}', got %q", string(b))
	}
}

func openCatalogTestDB(t *testing.T) *pgxpool.Pool {
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

func TestRepositoryListObjects_PagingAndChangedSince(t *testing.T) {
	db := openCatalogTestDB(t)
	repo := NewRepository(db)
	ns := "catalog_listobjects_test"
	t.Cleanup(func() {
		db.Exec(context.Background(), //nolint:errcheck // test cleanup
			`DELETE FROM catalog_items WHERE namespace = $1`, ns)
	})

	ctx := context.Background()
	for _, obj := range []string{"o1", "o2", "o3"} {
		if _, err := repo.Upsert(ctx, ns, obj, "content "+obj, ContentHash("content "+obj), nil); err != nil {
			t.Fatalf("seed %s: %v", obj, err)
		}
	}

	rows, total, err := repo.ListObjects(ctx, ns, nil, 2, 0, nil)
	if err != nil {
		t.Fatalf("ListObjects: %v", err)
	}
	if total != 3 || len(rows) != 2 {
		t.Fatalf("paging wrong: total=%d rows=%d", total, len(rows))
	}
	rows2, _, err := repo.ListObjects(ctx, ns, nil, 2, 2, nil)
	if err != nil {
		t.Fatalf("ListObjects offset: %v", err)
	}
	if len(rows2) != 1 {
		t.Fatalf("offset page wrong: %d", len(rows2))
	}

	// changed_since after every row's updated_at → empty.
	future := time.Now().Add(time.Hour)
	rows3, total3, err := repo.ListObjects(ctx, ns, &future, 10, 0, nil)
	if err != nil {
		t.Fatalf("ListObjects changed_since: %v", err)
	}
	if total3 != 0 || len(rows3) != 0 {
		t.Fatalf("future changed_since must be empty: total=%d rows=%d", total3, len(rows3))
	}
}

// ─── keyset cursor ───────────────────────────────────────────────────────────

// The reconciliation read is ordered by updated_at, and a batch ingest gives
// many rows the same timestamp. Offset paging over a set that is still being
// written re-sends rows or skips them; a keyset over (updated_at, id) is
// stable because id breaks the tie deterministically.
func TestObjectCursor_RoundTripsAndBindsToItsQuery(t *testing.T) {
	updatedAt := time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC)
	raw, err := encodeObjectCursor(objectCursor{
		Version: 1, Namespace: "ns", ChangedSince: "2026-08-01T00:00:00Z",
		UpdatedAt: updatedAt, ID: 42,
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	got, err := decodeObjectCursor(raw, "ns", "2026-08-01T00:00:00Z")
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ID != 42 || !got.UpdatedAt.Equal(updatedAt) {
		t.Errorf("round trip lost the key: %+v", got)
	}

	// A cursor is only meaningful for the query that produced it. Replaying a
	// cursor from one namespace against another, or after changing
	// changed_since, would silently page through a different result set.
	if _, err := decodeObjectCursor(raw, "other-ns", "2026-08-01T00:00:00Z"); !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("cursor accepted for a different namespace: %v", err)
	}
	if _, err := decodeObjectCursor(raw, "ns", "2026-01-01T00:00:00Z"); !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("cursor accepted for a different changed_since: %v", err)
	}
}

// A malformed cursor is a client error, not a silent restart from the
// beginning: restarting would re-send the whole corpus without saying so.
func TestObjectCursor_MalformedIsRejected(t *testing.T) {
	valid, err := encodeObjectCursor(objectCursor{
		Version: 1, Namespace: "ns", UpdatedAt: time.Now().UTC(), ID: 1,
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	zeroID, err := encodeObjectCursor(objectCursor{Version: 1, Namespace: "ns", UpdatedAt: time.Now().UTC(), ID: 0})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	futureVersion, err := encodeObjectCursor(objectCursor{Version: 99, Namespace: "ns", UpdatedAt: time.Now().UTC(), ID: 1})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	for _, tc := range []struct{ name, raw string }{
		{"not base64", "!!!not-base64!!!"},
		{"base64 but not JSON", "bm90LWpzb24"},
		{"zero id", zeroID},
		{"unknown version", futureVersion},
		{"truncated", valid[:len(valid)/2]},
	} {
		if _, err := decodeObjectCursor(tc.raw, "ns", ""); !errors.Is(err, ErrInvalidRequest) {
			t.Errorf("%s: expected ErrInvalidRequest, got %v", tc.name, err)
		}
	}
}

// An absent cursor is the first page, not an error — the legacy offset caller
// keeps working through the same endpoint.
func TestObjectCursor_EmptyMeansFirstPage(t *testing.T) {
	got, err := decodeObjectCursor("", "ns", "")
	if err != nil {
		t.Fatalf("empty cursor: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil cursor, got %+v", got)
	}
}
