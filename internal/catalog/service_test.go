package catalog

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/redis/go-redis/v9"

	"github.com/jarviisha/codohue/internal/core/namespace"
	"github.com/jarviisha/codohue/internal/core/nslifecycle"
	"github.com/jarviisha/codohue/pkg/codohuetypes"
)

// fakeRepo records calls and returns canned values.
type fakeRepo struct {
	res         *UpsertResult
	err         error
	called      int
	lastNS      string
	lastObj     string
	lastContent string
	lastHash    []byte
	lastMeta    map[string]any

	listRows   []ObjectRow
	listTotal  int
	listErr    error
	lastSince  *time.Time
	lastLimit  int
	lastOffset int

	authorHookCalls int
	rolledBack      bool
}

func (f *fakeRepo) Upsert(_ context.Context, ns, obj, content string, hash []byte, meta map[string]any) (*UpsertResult, error) {
	f.called++
	f.lastNS = ns
	f.lastObj = obj
	f.lastContent = content
	f.lastHash = hash
	f.lastMeta = meta
	return f.res, f.err
}

// UpsertWithAttribution mirrors the real repository: the content write and the
// attribution hook succeed or fail together, so a failing hook must leave the
// caller with an error and no result.
func (f *fakeRepo) UpsertWithAttribution(ctx context.Context, ns, obj, content string, hash []byte, meta map[string]any, writeAuthor AttributionWriter) (*UpsertResult, error) {
	res, err := f.Upsert(ctx, ns, obj, content, hash, meta)
	if err != nil {
		return nil, err
	}
	if writeAuthor != nil {
		f.authorHookCalls++
		if hookErr := writeAuthor(ctx, nil); hookErr != nil {
			f.rolledBack = true
			return nil, hookErr
		}
	}
	return res, nil
}

func (f *fakeRepo) ListObjects(_ context.Context, _ string, since *time.Time, limit, offset int, _ *objectCursor) ([]ObjectRow, int, error) {
	f.lastSince = since
	f.lastLimit = limit
	f.lastOffset = offset
	return f.listRows, f.listTotal, f.listErr
}

// fakeAuthorWriter captures the write-through into the objects domain.
type fakeAuthorWriter struct {
	calls []string // "ns/object/author" per call
	err   error
}

func (f *fakeAuthorWriter) SetAuthorTx(_ context.Context, _ pgx.Tx, ns, obj, author string) error {
	f.calls = append(f.calls, ns+"/"+obj+"/"+author)
	return f.err
}

// fakeNSConfig returns canned namespace configs.
type fakeNSConfig struct {
	cfg *namespace.Config
	err error
}

func (f *fakeNSConfig) Get(_ context.Context, _ string) (*namespace.Config, error) {
	return f.cfg, f.err
}

// fakeXAdder records every XAdd call.
type fakeXAdder struct {
	calls []*redis.XAddArgs
	err   error
}

func (f *fakeXAdder) XAdd(_ context.Context, args *redis.XAddArgs) *redis.StringCmd {
	f.calls = append(f.calls, args)
	cmd := redis.NewStringCmd(context.Background(), "XADD")
	if f.err != nil {
		cmd.SetErr(f.err)
	} else {
		cmd.SetVal("0-1")
	}
	return cmd
}

// helpers ------------------------------------------------------------------

func enabledCfg() *namespace.Config {
	return &namespace.Config{
		Namespace:              "ns",
		Generation:             7,
		DenseSource:            "catalog",
		CatalogStrategyID:      "internal-hashing-ngrams",
		CatalogStrategyVersion: "v1",
		CatalogMaxContentBytes: 32768,
		EmbeddingDim:           128,
	}
}

func newSvc(repo catalogRepository, nsCfg nsConfigGetter, pub xAdder) *Service {
	return &Service{
		repo:        repo,
		nsConfigSvc: nsCfg,
		publisher:   pub,
		clock:       func() time.Time { return time.Date(2026, 5, 9, 0, 0, 0, 0, time.UTC) },
	}
}

// tests --------------------------------------------------------------------

func TestServiceIngest_RejectsEmptyNamespace(t *testing.T) {
	svc := newSvc(&fakeRepo{}, &fakeNSConfig{}, &fakeXAdder{})
	_, err := svc.Ingest(context.Background(), "", &IngestRequest{ObjectID: "o1", Content: "x"})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest, got %v", err)
	}
}

func TestServiceIngest_RejectsNilRequest(t *testing.T) {
	svc := newSvc(&fakeRepo{}, &fakeNSConfig{}, &fakeXAdder{})
	_, err := svc.Ingest(context.Background(), "ns", nil)
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest, got %v", err)
	}
}

func TestServiceIngest_RejectsMissingObjectID(t *testing.T) {
	svc := newSvc(&fakeRepo{}, &fakeNSConfig{}, &fakeXAdder{})
	_, err := svc.Ingest(context.Background(), "ns", &IngestRequest{Content: "hello"})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest, got %v", err)
	}
}

func TestServiceIngest_RejectsEmptyContent(t *testing.T) {
	svc := newSvc(&fakeRepo{}, &fakeNSConfig{}, &fakeXAdder{})
	for _, c := range []string{"", "   ", "\t\n  "} {
		_, err := svc.Ingest(context.Background(), "ns", &IngestRequest{ObjectID: "o1", Content: c})
		if !errors.Is(err, ErrEmptyContent) {
			t.Errorf("content=%q: expected ErrEmptyContent, got %v", c, err)
		}
	}
}

// PostgreSQL text stores neither NUL nor invalid UTF-8. Before this was
// stripped, the INSERT failed with SQLSTATE 22021, which the catalog stream
// worker cannot distinguish from a transient failure — so it left the entry
// pending and redelivered it forever, pinning the stream against XTRIM.
func TestServiceIngest_StripsBytesPostgresCannotStore(t *testing.T) {
	repo := &fakeRepo{res: &UpsertResult{Item: &Item{ID: 1}}}
	svc := newSvc(repo, &fakeNSConfig{cfg: enabledCfg()}, &fakeXAdder{})

	if _, err := svc.Ingest(context.Background(), "ns", &IngestRequest{
		ObjectID: "o1", Content: "he\x00llo\xffworld",
	}); err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if want := "helloworld"; repo.lastContent != want {
		t.Errorf("stored content = %q, want %q", repo.lastContent, want)
	}
	// The hash must cover what was actually stored, so re-ingesting the same
	// dirty content stays an idempotent no-op upsert.
	if want := ContentHash("helloworld"); !bytes.Equal(repo.lastHash, want) {
		t.Errorf("hash = %x, want hash of sanitized content %x", repo.lastHash, want)
	}
}

// Content that is nothing but unstorable bytes collapses to empty and takes
// the existing empty-content rejection, which the stream worker acks off
// instead of redelivering.
func TestServiceIngest_ContentOfOnlyUnstorableBytesIsEmpty(t *testing.T) {
	svc := newSvc(&fakeRepo{}, &fakeNSConfig{cfg: enabledCfg()}, &fakeXAdder{})
	_, err := svc.Ingest(context.Background(), "ns", &IngestRequest{ObjectID: "o1", Content: "\x00\xff"})
	if !errors.Is(err, ErrEmptyContent) {
		t.Fatalf("expected ErrEmptyContent, got %v", err)
	}
}

// object_id is half of UNIQUE (namespace, object_id) on catalog_items and of
// the objects primary key. Stripping it would merge two distinct keys into one
// row, so an unstorable identifier is rejected instead — permanently, so the
// stream worker acks it off rather than redelivering it forever.
func TestServiceIngest_RejectsUnstorableObjectID(t *testing.T) {
	repo := &fakeRepo{res: &UpsertResult{Item: &Item{ID: 1}}}
	svc := newSvc(repo, &fakeNSConfig{cfg: enabledCfg()}, &fakeXAdder{})

	for _, id := range []string{"at://did\x00:plc/post", "at://did\xff/post"} {
		_, err := svc.Ingest(context.Background(), "ns", &IngestRequest{ObjectID: id, Content: "hello"})
		if !errors.Is(err, ErrInvalidRequest) {
			t.Errorf("object_id=%q: expected ErrInvalidRequest, got %v", id, err)
		}
	}
	if repo.called != 0 {
		t.Errorf("rejected object_id must not reach the repository, got %d calls", repo.called)
	}
}

// Two object_ids differing only in bytes PostgreSQL cannot store must not
// collapse onto one row — that would clobber one item's content and embedding
// with the other's.
func TestServiceIngest_UnstorableObjectIDDoesNotAliasACleanOne(t *testing.T) {
	repo := &fakeRepo{res: &UpsertResult{Item: &Item{ID: 1}}}
	svc := newSvc(repo, &fakeNSConfig{cfg: enabledCfg()}, &fakeXAdder{})

	if _, err := svc.Ingest(context.Background(), "ns", &IngestRequest{ObjectID: "ab", Content: "clean"}); err != nil {
		t.Fatalf("clean ingest: %v", err)
	}
	if _, err := svc.Ingest(context.Background(), "ns", &IngestRequest{ObjectID: "a\x00b", Content: "dirty"}); err == nil {
		t.Fatal("dirty object_id must not be accepted")
	}
	if repo.lastObj != "ab" || repo.lastContent != "clean" {
		t.Errorf("clean row was overwritten: object_id=%q content=%q", repo.lastObj, repo.lastContent)
	}
}

// metadata is jsonb. json.Marshal encodes a NUL as a six-character
// backslash-u escape sequence, which jsonb rejects with SQLSTATE 22P05 --
// so the map has to be cleaned before it is marshalled, not after. Keys,
// nested maps, and slice elements all reach the same column.
func TestServiceIngest_StripsUnstorableBytesFromMetadata(t *testing.T) {
	repo := &fakeRepo{res: &UpsertResult{Item: &Item{ID: 1}}}
	svc := newSvc(repo, &fakeNSConfig{cfg: enabledCfg()}, &fakeXAdder{})

	if _, err := svc.Ingest(context.Background(), "ns", &IngestRequest{
		ObjectID: "o1",
		Content:  "hello",
		Metadata: map[string]any{
			"so\x00urce": "blue\x00sky",
			"langs":      []any{"e\x00n", "vi"},
			"nested":     map[string]any{"k": "v\xffal"},
			"count":      float64(3),
		},
	}); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	want := map[string]any{
		"source": "bluesky",
		"langs":  []any{"en", "vi"},
		"nested": map[string]any{"k": "val"},
		"count":  float64(3),
	}
	if !reflect.DeepEqual(repo.lastMeta, want) {
		t.Errorf("stored metadata = %#v, want %#v", repo.lastMeta, want)
	}
}

// author_subject_id is TEXT on the objects table, written through inside the
// catalog row's own transaction. It identifies a subject, so like object_id it
// is rejected rather than stripped: quietly rewriting it would attribute the
// content to a different author than the one the caller named.
func TestServiceIngest_RejectsUnstorableAuthorSubjectID(t *testing.T) {
	writer := &fakeAuthorWriter{}
	svc := newSvc(&fakeRepo{res: &UpsertResult{Item: &Item{ID: 1}}}, &fakeNSConfig{cfg: enabledCfg()}, &fakeXAdder{})
	svc.SetAuthorWriter(writer)

	_, err := svc.Ingest(context.Background(), "ns", &IngestRequest{
		ObjectID: "o1", Content: "hello", AuthorSubjectID: "did:plc:a\x00b",
	})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest, got %v", err)
	}
	if len(writer.calls) != 0 {
		t.Errorf("rejected author must not be written through, got %v", writer.calls)
	}
}

// A data exception from the persist means the row is unstorable as sent, so it
// becomes permanent and the stream worker acks it off. This is the backstop for
// unstorable values Ingest does not model; the sanitize covers the ones it does.
func TestServiceIngest_DataExceptionOnPersistIsUnstorable(t *testing.T) {
	for _, code := range []string{"22021", "22P05", "22001"} {
		repo := &fakeRepo{err: &pgconn.PgError{Code: code, Message: "data exception"}}
		svc := newSvc(repo, &fakeNSConfig{cfg: enabledCfg()}, &fakeXAdder{})

		_, err := svc.Ingest(context.Background(), "ns", &IngestRequest{ObjectID: "o1", Content: "hello"})
		if !errors.Is(err, ErrUnstorable) {
			t.Errorf("SQLSTATE %s on persist: expected ErrUnstorable, got %v", code, err)
		}
	}
}

// Every other SQLSTATE from the persist stays transient — a connection loss or
// a serialization conflict is exactly what redelivery exists for, and acking
// those off would silently drop good content.
func TestServiceIngest_NonDataExceptionOnPersistStaysTransient(t *testing.T) {
	for _, code := range []string{"08006", "40001", "23505", "53200"} {
		repo := &fakeRepo{err: &pgconn.PgError{Code: code, Message: "not a data exception"}}
		svc := newSvc(repo, &fakeNSConfig{cfg: enabledCfg()}, &fakeXAdder{})

		_, err := svc.Ingest(context.Background(), "ns", &IngestRequest{ObjectID: "o1", Content: "hello"})
		if err == nil || errors.Is(err, ErrUnstorable) {
			t.Errorf("SQLSTATE %s on persist: expected transient, got %v", code, err)
		}
	}
}

// The classification is scoped to the persist on purpose. The same SQLSTATE
// raised by the namespace-config read is a defect in that query, not unstorable
// content, so it must stay transient — otherwise one bad config query would ack
// every item on the stream off as permanently rejected.
func TestServiceIngest_DataExceptionOnConfigReadStaysTransient(t *testing.T) {
	svc := newSvc(&fakeRepo{}, &fakeNSConfig{err: &pgconn.PgError{Code: "22021"}}, &fakeXAdder{})

	_, err := svc.Ingest(context.Background(), "ns", &IngestRequest{ObjectID: "o1", Content: "hello"})
	if err == nil || errors.Is(err, ErrUnstorable) {
		t.Fatalf("config-read data exception must stay transient, got %v", err)
	}
}

// The size cap must measure what is actually stored, not what arrived —
// otherwise content that is only oversized because of junk bytes is rejected
// for a length it will never have on disk.
func TestServiceIngest_SizeCapMeasuresSanitizedContent(t *testing.T) {
	cfg := enabledCfg()
	cfg.CatalogMaxContentBytes = 10
	repo := &fakeRepo{res: &UpsertResult{Item: &Item{ID: 1}}}
	svc := newSvc(repo, &fakeNSConfig{cfg: cfg}, &fakeXAdder{})

	// 13 bytes on the wire, 8 once the NULs are gone.
	if _, err := svc.Ingest(context.Background(), "ns", &IngestRequest{
		ObjectID: "o1", Content: strings.Repeat("x", 8) + strings.Repeat("\x00", 5),
	}); err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if want := strings.Repeat("x", 8); repo.lastContent != want {
		t.Errorf("stored content = %q, want %q", repo.lastContent, want)
	}
}

func TestServiceIngest_NamespaceNotFound(t *testing.T) {
	svc := newSvc(&fakeRepo{}, &fakeNSConfig{cfg: nil}, &fakeXAdder{})
	_, err := svc.Ingest(context.Background(), "ns", &IngestRequest{ObjectID: "o1", Content: "hi"})
	if !errors.Is(err, ErrNamespaceNotFound) {
		t.Fatalf("expected ErrNamespaceNotFound, got %v", err)
	}
}

func TestServiceIngest_NamespaceNotEnabled(t *testing.T) {
	cfg := enabledCfg()
	cfg.DenseSource = "disabled"
	svc := newSvc(&fakeRepo{}, &fakeNSConfig{cfg: cfg}, &fakeXAdder{})
	_, err := svc.Ingest(context.Background(), "ns", &IngestRequest{ObjectID: "o1", Content: "hi"})
	if !errors.Is(err, ErrNamespaceNotEnabled) {
		t.Fatalf("expected ErrNamespaceNotEnabled, got %v", err)
	}
}

func TestServiceIngest_NamespaceConfigError(t *testing.T) {
	svc := newSvc(&fakeRepo{}, &fakeNSConfig{err: errors.New("db down")}, &fakeXAdder{})
	_, err := svc.Ingest(context.Background(), "ns", &IngestRequest{ObjectID: "o1", Content: "hi"})
	if err == nil || errors.Is(err, ErrNamespaceNotFound) {
		t.Fatalf("expected wrapped DB error, got %v", err)
	}
}

func TestServiceIngest_ContentTooLarge(t *testing.T) {
	cfg := enabledCfg()
	cfg.CatalogMaxContentBytes = 10
	svc := newSvc(&fakeRepo{}, &fakeNSConfig{cfg: cfg}, &fakeXAdder{})
	_, err := svc.Ingest(context.Background(), "ns", &IngestRequest{
		ObjectID: "o1",
		Content:  strings.Repeat("x", 11),
	})
	if !errors.Is(err, ErrContentTooLarge) {
		t.Fatalf("expected ErrContentTooLarge, got %v", err)
	}
}

// Attribution no longer lands on catalog_items — it is written through to
// the objects domain so it works under every dense_source.
func TestServiceIngest_WritesAuthorThroughToObjects(t *testing.T) {
	writer := &fakeAuthorWriter{}
	svc := newSvc(&fakeRepo{res: &UpsertResult{Item: &Item{ID: 1}}}, &fakeNSConfig{cfg: enabledCfg()}, &fakeXAdder{})
	svc.SetAuthorWriter(writer)

	if _, err := svc.Ingest(context.Background(), "ns", &IngestRequest{
		ObjectID: "o1", Content: "hello", AuthorSubjectID: "u1",
	}); err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if len(writer.calls) != 1 || writer.calls[0] != "ns/o1/u1" {
		t.Errorf("author write-through: got %v", writer.calls)
	}
}

// Omitting the author must not call through at all — absence means
// "unspecified", not "clear whatever the objects endpoint set".
func TestServiceIngest_NoAuthorSkipsWriteThrough(t *testing.T) {
	writer := &fakeAuthorWriter{}
	svc := newSvc(&fakeRepo{res: &UpsertResult{Item: &Item{ID: 1}}}, &fakeNSConfig{cfg: enabledCfg()}, &fakeXAdder{})
	svc.SetAuthorWriter(writer)

	if _, err := svc.Ingest(context.Background(), "ns", &IngestRequest{
		ObjectID: "o1", Content: "hello",
	}); err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if len(writer.calls) != 0 {
		t.Errorf("expected no write-through, got %v", writer.calls)
	}
}

// Content and attribution are one request, so they are one write. Reporting
// 202 with the author silently dropped left the caller no way to learn its
// attribution never happened — the request now fails and nothing is committed.
func TestServiceIngest_AttributionFailureRollsBackTheContentWrite(t *testing.T) {
	writer := &fakeAuthorWriter{err: errors.New("objects table down")}
	repo := &fakeRepo{res: &UpsertResult{Item: &Item{ID: 1}, NeedsPublish: true}}
	pub := &fakeXAdder{}
	svc := newSvc(repo, &fakeNSConfig{cfg: enabledCfg()}, pub)
	svc.SetAuthorWriter(writer)

	_, err := svc.Ingest(context.Background(), "ns", &IngestRequest{
		ObjectID: "o1", Content: "hello", AuthorSubjectID: "u1",
	})

	if err == nil {
		t.Fatal("a dropped attribution must not be reported as success")
	}
	if !repo.rolledBack {
		t.Error("the content write must roll back with the attribution")
	}
	// Nothing was committed, so nothing may be queued for embedding.
	if len(pub.calls) != 0 {
		t.Errorf("rolled-back ingest must not publish embed work, got %v", pub.calls)
	}
}

// A re-ingest of identical content is a no-op for embedding (NeedsPublish
// false) but must still apply a new attribution — otherwise correcting an
// author would require editing the content to force a write.
func TestServiceIngest_SameContentStillUpdatesAttribution(t *testing.T) {
	writer := &fakeAuthorWriter{}
	repo := &fakeRepo{res: &UpsertResult{Item: &Item{ID: 1}, NeedsPublish: false}}
	pub := &fakeXAdder{}
	svc := newSvc(repo, &fakeNSConfig{cfg: enabledCfg()}, pub)
	svc.SetAuthorWriter(writer)

	if _, err := svc.Ingest(context.Background(), "ns", &IngestRequest{
		ObjectID: "o1", Content: "hello", AuthorSubjectID: "u2",
	}); err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if len(writer.calls) != 1 || writer.calls[0] != "ns/o1/u2" {
		t.Errorf("attribution not applied: %v", writer.calls)
	}
	if len(pub.calls) != 0 {
		t.Errorf("unchanged content must not redo embed work, got %v", pub.calls)
	}
}

func TestServiceIngest_HappyPath_PublishesToStream(t *testing.T) {
	cfg := enabledCfg()
	repo := &fakeRepo{res: &UpsertResult{
		Item: &Item{
			ID: 7, Namespace: "ns", ObjectID: "o1", State: StatePending,
		},
		NeedsPublish: true,
	}}
	pub := &fakeXAdder{}
	svc := newSvc(repo, &fakeNSConfig{cfg: cfg}, pub)

	item, err := svc.Ingest(context.Background(), "ns", &IngestRequest{
		ObjectID: "o1",
		Content:  "Hôm nay trời đẹp",
	})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if item.ID != 7 {
		t.Errorf("item.ID: got %d, want 7", item.ID)
	}
	if repo.called != 1 {
		t.Errorf("repo called %d times", repo.called)
	}
	if repo.lastNS != "ns" || repo.lastObj != "o1" {
		t.Errorf("repo args: ns=%s obj=%s", repo.lastNS, repo.lastObj)
	}
	if len(pub.calls) != 1 {
		t.Fatalf("expected 1 XAdd call, got %d", len(pub.calls))
	}
	xa := pub.calls[0]
	if xa.Stream != "catalog:embed:ns:g7" {
		t.Errorf("stream: got %q", xa.Stream)
	}
	if xa.MaxLen != 0 || xa.Approx {
		t.Errorf("embed stream must not be producer-trimmed: MaxLen=%d Approx=%v", xa.MaxLen, xa.Approx)
	}
	v, ok := xa.Values.(map[string]any)
	if !ok {
		t.Fatalf("values: got %T", xa.Values)
	}
	if v["catalog_item_id"] != int64(7) {
		t.Errorf("catalog_item_id: got %v", v["catalog_item_id"])
	}
	if v["namespace"] != "ns" {
		t.Errorf("namespace: got %v", v["namespace"])
	}
	if v["namespace_generation"] != int64(7) {
		t.Errorf("namespace_generation: got %v", v["namespace_generation"])
	}
	if v["object_id"] != "o1" {
		t.Errorf("object_id: got %v", v["object_id"])
	}
	if v["strategy_id"] != "internal-hashing-ngrams" {
		t.Errorf("strategy_id: got %v", v["strategy_id"])
	}
	if v["strategy_version"] != "v1" {
		t.Errorf("strategy_version: got %v", v["strategy_version"])
	}
	if v["enqueued_at"] != "2026-05-09T00:00:00Z" {
		t.Errorf("enqueued_at: got %v", v["enqueued_at"])
	}
}

func TestServiceIngest_IdempotentDoesNotPublish(t *testing.T) {
	cfg := enabledCfg()
	repo := &fakeRepo{res: &UpsertResult{
		Item: &Item{
			ID: 7, Namespace: "ns", ObjectID: "o1", State: StateEmbedded,
		},
		NeedsPublish: false,
	}}
	pub := &fakeXAdder{}
	svc := newSvc(repo, &fakeNSConfig{cfg: cfg}, pub)

	item, err := svc.Ingest(context.Background(), "ns", &IngestRequest{
		ObjectID: "o1",
		Content:  "hello world",
	})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if item.State != StateEmbedded {
		t.Errorf("state: got %s", item.State)
	}
	if len(pub.calls) != 0 {
		t.Errorf("expected no XAdd on idempotent re-ingest, got %d calls", len(pub.calls))
	}
}

func TestServiceIngest_RepoErrorPropagates(t *testing.T) {
	cfg := enabledCfg()
	repo := &fakeRepo{err: errors.New("db error")}
	svc := newSvc(repo, &fakeNSConfig{cfg: cfg}, &fakeXAdder{})

	_, err := svc.Ingest(context.Background(), "ns", &IngestRequest{ObjectID: "o1", Content: "hi"})
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.Is(err, ErrNamespaceNotEnabled) || errors.Is(err, ErrInvalidRequest) {
		t.Errorf("expected non-sentinel wrapped error, got %v", err)
	}
}

func TestServiceIngest_PublishFailureSurfaceErrorButDoesNotRollBack(t *testing.T) {
	// The row is already committed by the time XAdd runs. A publish failure
	// MUST be surfaced so observability sees it, but the row must not be
	// rolled back — the recovery sweep will pick it up.
	cfg := enabledCfg()
	repo := &fakeRepo{res: &UpsertResult{
		Item:         &Item{ID: 7, Namespace: "ns", ObjectID: "o1", State: StatePending},
		NeedsPublish: true,
	}}
	pub := &fakeXAdder{err: errors.New("redis down")}
	svc := newSvc(repo, &fakeNSConfig{cfg: cfg}, pub)

	item, err := svc.Ingest(context.Background(), "ns", &IngestRequest{ObjectID: "o1", Content: "hi"})
	if err == nil {
		t.Fatal("expected publish error to surface")
	}
	if item == nil || item.ID != 7 {
		t.Errorf("expected item to still be returned despite publish failure, got %+v", item)
	}
}

func TestServiceIngest_ZeroMaxContentBytesMeansNoCheck(t *testing.T) {
	// CatalogMaxContentBytes=0 means "use default at higher level"; service
	// must not enforce against zero (which would reject everything).
	cfg := enabledCfg()
	cfg.CatalogMaxContentBytes = 0
	repo := &fakeRepo{res: &UpsertResult{
		Item:         &Item{ID: 1, Namespace: "ns", ObjectID: "o1", State: StatePending},
		NeedsPublish: true,
	}}
	svc := newSvc(repo, &fakeNSConfig{cfg: cfg}, &fakeXAdder{})

	_, err := svc.Ingest(context.Background(), "ns", &IngestRequest{
		ObjectID: "o1",
		Content:  strings.Repeat("x", 1<<16),
	})
	if err != nil {
		t.Fatalf("expected no size check when limit=0, got %v", err)
	}
}

func TestStreamName(t *testing.T) {
	if got := streamName("foo"); got != "catalog:embed:foo" {
		t.Errorf("streamName(foo): %q", got)
	}
}

func TestServiceIngestBatch_PerItemResultsAndCounts(t *testing.T) {
	repo := &fakeRepo{res: &UpsertResult{Item: &Item{ID: 1, Namespace: "ns", ObjectID: "o1"}, NeedsPublish: false}}
	svc := newSvc(repo, &fakeNSConfig{cfg: enabledCfg()}, &fakeXAdder{})

	resp, err := svc.IngestBatch(context.Background(), "ns", &BatchIngestRequest{Items: []IngestRequest{
		{ObjectID: "o1", Content: "hello"},
		{ObjectID: "o2", Content: "   "}, // empty after trim → per-item rejection
		{ObjectID: "", Content: "hi"},    // missing object_id → per-item rejection
	}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Accepted != 1 || resp.Rejected != 2 || len(resp.Results) != 3 {
		t.Fatalf("counts wrong: %+v", resp)
	}
	if !resp.Results[0].Accepted || resp.Results[0].Error != "" {
		t.Fatalf("first item should be accepted: %+v", resp.Results[0])
	}
	if resp.Results[1].Error != "empty_content" || resp.Results[2].Error != "invalid_request" {
		t.Fatalf("per-item codes wrong: %+v", resp.Results)
	}
}

func TestServiceIngestBatch_NamespaceErrorAbortsWholeBatch(t *testing.T) {
	svc := newSvc(&fakeRepo{}, &fakeNSConfig{cfg: nil}, &fakeXAdder{})
	_, err := svc.IngestBatch(context.Background(), "ghost", &BatchIngestRequest{Items: []IngestRequest{
		{ObjectID: "o1", Content: "hello"},
	}})
	if !errors.Is(err, ErrNamespaceNotFound) {
		t.Fatalf("expected ErrNamespaceNotFound, got %v", err)
	}
}

func TestServiceIngestBatch_CapAndEmptyRejected(t *testing.T) {
	svc := newSvc(&fakeRepo{}, &fakeNSConfig{cfg: enabledCfg()}, &fakeXAdder{})
	if _, err := svc.IngestBatch(context.Background(), "ns", &BatchIngestRequest{}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("empty batch must be ErrInvalidRequest, got %v", err)
	}
	over := make([]IngestRequest, codohuetypes.CatalogBatchMaxItems+1)
	for i := range over {
		over[i] = IngestRequest{ObjectID: "o", Content: "c"}
	}
	if _, err := svc.IngestBatch(context.Background(), "ns", &BatchIngestRequest{Items: over}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("oversized batch must be ErrInvalidRequest, got %v", err)
	}
}

func TestServiceListObjects_MapsRows(t *testing.T) {
	repo := &fakeRepo{
		listRows:  []ObjectRow{{ObjectID: "o1", UpdatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}},
		listTotal: 7,
	}
	svc := newSvc(repo, &fakeNSConfig{cfg: enabledCfg()}, &fakeXAdder{})
	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	resp, err := svc.ListObjects(context.Background(), "ns", &since, 50, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Total != 7 || resp.Limit != 50 || resp.Offset != 10 {
		t.Fatalf("paging meta wrong: %+v", resp)
	}
	if len(resp.Items) != 1 || resp.Items[0].ObjectID != "o1" || resp.Items[0].UpdatedAt != "2026-01-02T03:04:05Z" {
		t.Fatalf("items wrong: %+v", resp.Items)
	}
	if repo.lastSince == nil || !repo.lastSince.Equal(since) || repo.lastLimit != 50 || repo.lastOffset != 10 {
		t.Fatalf("repo args wrong: since=%v limit=%d offset=%d", repo.lastSince, repo.lastLimit, repo.lastOffset)
	}
}

func TestServiceListObjects_NamespaceNotFound(t *testing.T) {
	svc := newSvc(&fakeRepo{}, &fakeNSConfig{cfg: nil}, &fakeXAdder{})
	if _, err := svc.ListObjects(context.Background(), "ghost", nil, 100, 0); !errors.Is(err, ErrNamespaceNotFound) {
		t.Fatalf("expected ErrNamespaceNotFound, got %v", err)
	}
}

func TestItemErrorCode_Mapping(t *testing.T) {
	cases := map[error]string{
		ErrEmptyContent:                  "empty_content",
		ErrContentTooLarge:               "content_too_large",
		ErrInvalidRequest:                "invalid_request",
		errors.New("something exploded"): "internal_error",
	}
	for err, want := range cases {
		if got := itemErrorCode(err); got != want {
			t.Errorf("itemErrorCode(%v) = %q, want %q", err, got, want)
		}
	}
}

func TestNewServiceAndSetters(t *testing.T) {
	svc := NewService(nil, &fakeNSConfig{}, &fakeXAdder{})
	svc.SetDefaultMaxContentBytes(1024)
	if svc.defaultMaxContentBytes != 1024 {
		t.Fatalf("default max content bytes not wired: %d", svc.defaultMaxContentBytes)
	}
	if NewHandler(svc) == nil {
		t.Fatal("NewHandler returned nil")
	}
}

// --- lifecycle fencing ----------------------------------------------------

type fakeLifecycleWriter struct {
	generation int64
	err        error
	calls      int
}

func (f *fakeLifecycleWriter) WithWriter(ctx context.Context, ns string, fn func(context.Context, *nslifecycle.NamespaceLifecycle) error) error {
	// Mirror the real WithWriter: an inherited lease is reused without a new
	// acquisition, so calls counts acquisitions only.
	if generation, ok := nslifecycle.LeaseGeneration(ctx, ns); ok {
		return fn(ctx, &nslifecycle.NamespaceLifecycle{Namespace: ns, Generation: generation, State: nslifecycle.StateActive})
	}
	f.calls++
	if f.err != nil {
		return f.err
	}
	leased := nslifecycle.ContextWithLease(ctx, ns, f.generation, nslifecycle.LockShared)
	return fn(leased, &nslifecycle.NamespaceLifecycle{Namespace: ns, Generation: f.generation, State: nslifecycle.StateActive})
}

// Catalog ingest writes a row AND publishes embed work, so it must hold the
// lease across both: a delete landing between the two would leave a stream
// entry pointing at a row that no longer exists.
func TestServiceIngest_RunsUnderLifecycleLease(t *testing.T) {
	repo := &fakeRepo{res: &UpsertResult{Item: &Item{ID: 1, Namespace: "ns", ObjectID: "o1"}, NeedsPublish: true}}
	pub := &fakeXAdder{}
	svc := newSvc(repo, &fakeNSConfig{cfg: enabledCfg()}, pub)
	lifecycle := &fakeLifecycleWriter{generation: 7}
	svc.SetLifecycleWriter(lifecycle)

	if _, err := svc.Ingest(context.Background(), "ns", &IngestRequest{ObjectID: "o1", Content: "hello"}); err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if lifecycle.calls != 1 {
		t.Errorf("expected exactly 1 lease acquisition, got %d", lifecycle.calls)
	}
	if len(pub.calls) != 1 {
		t.Fatalf("expected 1 XADD, got %d", len(pub.calls))
	}
	// The embed work lands on the generation's own stream, so an embedder
	// consuming the previous incarnation never sees it.
	if pub.calls[0].Stream != "catalog:embed:ns:g7" {
		t.Errorf("stream: got %q, want catalog:embed:ns:g7", pub.calls[0].Stream)
	}
	values, ok := pub.calls[0].Values.(map[string]any)
	if !ok {
		t.Fatalf("unexpected XADD values type %T", pub.calls[0].Values)
	}
	if values["namespace_generation"] != int64(7) {
		t.Errorf("payload generation: got %v, want 7", values["namespace_generation"])
	}
}

func TestServiceIngest_InactiveNamespaceWritesNothing(t *testing.T) {
	repo := &fakeRepo{res: &UpsertResult{Item: &Item{ID: 1, Namespace: "ns", ObjectID: "o1"}, NeedsPublish: true}}
	pub := &fakeXAdder{}
	svc := newSvc(repo, &fakeNSConfig{cfg: enabledCfg()}, pub)
	svc.SetLifecycleWriter(&fakeLifecycleWriter{err: nslifecycle.ErrNamespaceNotActive})

	_, err := svc.Ingest(context.Background(), "ns", &IngestRequest{ObjectID: "o1", Content: "hello"})
	if !errors.Is(err, nslifecycle.ErrNamespaceNotActive) {
		t.Fatalf("expected ErrNamespaceNotActive, got %v", err)
	}
	if repo.called != 0 || len(pub.calls) != 0 {
		t.Errorf("inactive namespace reached storage: repo=%d xadds=%d", repo.called, len(pub.calls))
	}
}

// The stream worker evaluates the envelope under its own lease and then calls
// Ingest; that lease must be reused rather than re-acquired.
func TestServiceIngest_ReusesHeldLease(t *testing.T) {
	repo := &fakeRepo{res: &UpsertResult{Item: &Item{ID: 1, Namespace: "ns", ObjectID: "o1"}, NeedsPublish: true}}
	svc := newSvc(repo, &fakeNSConfig{cfg: enabledCfg()}, &fakeXAdder{})
	lifecycle := &fakeLifecycleWriter{generation: 7}
	svc.SetLifecycleWriter(lifecycle)

	ctx := nslifecycle.ContextWithLease(context.Background(), "ns", 7, nslifecycle.LockShared)
	if _, err := svc.Ingest(ctx, "ns", &IngestRequest{ObjectID: "o1", Content: "hello"}); err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if lifecycle.calls != 0 {
		t.Errorf("held lease must be reused, got %d acquisitions", lifecycle.calls)
	}
	if repo.called != 1 {
		t.Errorf("expected the write to reach the repo once, got %d", repo.called)
	}
}

// Generation 1 keeps the original unqualified stream name, so upgrading moves
// nothing. A generation below 1 is not a real lifecycle value; clamping stops
// a bad caller from publishing to a stream no consumer reads.
func TestGenerationStreamName_ClampsBelowOne(t *testing.T) {
	base := generationStreamName("ns", 1)
	if base != streamName("ns") {
		t.Fatalf("generation 1 stream = %q, want the unqualified %q", base, streamName("ns"))
	}
	for _, generation := range []int64{0, -1} {
		if got := generationStreamName("ns", generation); got != base {
			t.Errorf("generation %d gave %q, want %q", generation, got, base)
		}
	}
	if g2 := generationStreamName("ns", 2); g2 == base {
		t.Error("generation 2 shares generation 1's stream; a deleted incarnation's work would be visible to the new one")
	}
}
