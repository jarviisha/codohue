package catalog

import (
	"context"
	"errors"
	"testing"
	"time"
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
func fillScanRow(dest []any, contentHash []byte, metadata []byte, state string, needsPublish bool, now time.Time) error {
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
	if string(a) != string(b) {
		t.Fatalf("ContentHash not deterministic: %x vs %x", a, b)
	}
	if len(a) != 32 {
		t.Fatalf("expected 32-byte sha256, got %d", len(a))
	}
}

func TestContentHash_DifferentContentDiffers(t *testing.T) {
	a := ContentHash("hello")
	b := ContentHash("hello!")
	if string(a) == string(b) {
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
	if string(res.Item.ContentHash) != string(hash) {
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
	meta := []byte(`{"author":"u1","lang":"vi"}`)
	repo := &Repository{
		queryRowFn: func(_ context.Context, _ string, _ ...any) rowScanner {
			return fakeRow{scanFn: func(dest ...any) error {
				return fillScanRow(dest, hash, meta, "pending", true, now)
			}}
		},
	}
	res, err := repo.Upsert(context.Background(), "ns", "obj1", "hello", hash, map[string]any{"author": "u1", "lang": "vi"})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if res.Item.Metadata["author"] != "u1" {
		t.Errorf("metadata author: got %v", res.Item.Metadata["author"])
	}
	if res.Item.Metadata["lang"] != "vi" {
		t.Errorf("metadata lang: got %v", res.Item.Metadata["lang"])
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
