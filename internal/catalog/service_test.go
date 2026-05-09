package catalog

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/jarviisha/codohue/internal/core/namespace"
)

// fakeRepo records calls and returns canned values.
type fakeRepo struct {
	res     *UpsertResult
	err     error
	called  int
	lastNS  string
	lastObj string
	lastHash []byte
}

func (f *fakeRepo) Upsert(_ context.Context, ns, obj, _ string, hash []byte, _ map[string]any) (*UpsertResult, error) {
	f.called++
	f.lastNS = ns
	f.lastObj = obj
	f.lastHash = hash
	return f.res, f.err
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
		CatalogEnabled:         true,
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

func TestServiceIngest_NamespaceNotFound(t *testing.T) {
	svc := newSvc(&fakeRepo{}, &fakeNSConfig{cfg: nil}, &fakeXAdder{})
	_, err := svc.Ingest(context.Background(), "ns", &IngestRequest{ObjectID: "o1", Content: "hi"})
	if !errors.Is(err, ErrNamespaceNotFound) {
		t.Fatalf("expected ErrNamespaceNotFound, got %v", err)
	}
}

func TestServiceIngest_NamespaceNotEnabled(t *testing.T) {
	cfg := enabledCfg()
	cfg.CatalogEnabled = false
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

func TestServiceIngest_HappyPath_PublishesToStream(t *testing.T) {
	cfg := enabledCfg()
	repo := &fakeRepo{res: &UpsertResult{
		Item: &CatalogItem{
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
	if xa.Stream != "catalog:embed:ns" {
		t.Errorf("stream: got %q", xa.Stream)
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
		Item: &CatalogItem{
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
		Item:         &CatalogItem{ID: 7, Namespace: "ns", ObjectID: "o1", State: StatePending},
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
		Item:         &CatalogItem{ID: 1, Namespace: "ns", ObjectID: "o1", State: StatePending},
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
