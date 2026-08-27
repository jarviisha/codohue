package redistream

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jarviisha/codohue/pkg/codohuetypes"
)

func TestCatalogProducerPublishUsesDefaultStream(t *testing.T) {
	t.Parallel()

	f := &fakeXAdder{id: "1700000000-0"}
	p := NewCatalogProducer(f)

	item := codohuetypes.CatalogStreamItem{
		Namespace:       "feed",
		ObjectID:        "post-1",
		Content:         "hello world",
		AuthorSubjectID: "u-1",
	}
	id, err := p.Publish(context.Background(), item)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if id != "1700000000-0" {
		t.Errorf("id = %q", id)
	}
	if len(f.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(f.calls))
	}
	call := f.calls[0]
	if call.Stream != codohuetypes.CatalogStreamName {
		t.Errorf("stream = %q, want %q", call.Stream, codohuetypes.CatalogStreamName)
	}
	if call.MaxLen != 0 {
		t.Errorf("MaxLen = %d — the catalog stream must not be producer-trimmed", call.MaxLen)
	}

	valuesMap, ok := call.Values.(map[string]any)
	if !ok {
		t.Fatalf("values type = %T", call.Values)
	}
	raw, ok := valuesMap[codohuetypes.PayloadField].(string)
	if !ok {
		t.Fatalf("payload field missing: %#v", valuesMap)
	}
	var decoded codohuetypes.CatalogStreamItem
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatalf("payload not valid JSON: %v", err)
	}
	if decoded.Namespace != item.Namespace || decoded.ObjectID != item.ObjectID ||
		decoded.Content != item.Content || decoded.AuthorSubjectID != item.AuthorSubjectID {
		t.Errorf("round-trip mismatch: %+v vs %+v", decoded, item)
	}
}

func TestCatalogProducerStreamOverride(t *testing.T) {
	t.Parallel()

	f := &fakeXAdder{id: "1-0"}
	p := NewCatalogProducer(f, WithCatalogStream("custom:catalog"))
	if _, err := p.Publish(context.Background(), codohuetypes.CatalogStreamItem{Namespace: "n", ObjectID: "o", Content: "c"}); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if f.calls[0].Stream != "custom:catalog" {
		t.Errorf("stream = %q", f.calls[0].Stream)
	}
}

func TestCatalogProducerDefaultGenerationStampsLegacyPayload(t *testing.T) {
	f := &fakeXAdder{id: "1-0"}
	p := NewCatalogProducer(f, WithCatalogNamespaceGeneration(4))
	if _, err := p.Publish(context.Background(), codohuetypes.CatalogStreamItem{Namespace: "feed"}); err != nil {
		t.Fatal(err)
	}
	raw := f.calls[0].Values.(map[string]any)[codohuetypes.PayloadField].(string)
	var decoded codohuetypes.CatalogStreamItem
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.NamespaceGeneration != 4 {
		t.Fatalf("generation = %d", decoded.NamespaceGeneration)
	}
}

func TestCatalogProducerPublishBatchStopsOnError(t *testing.T) {
	t.Parallel()

	f := &fakeXAdder{err: errors.New("redis down")}
	p := NewCatalogProducer(f)
	ids, err := p.PublishBatch(context.Background(), []codohuetypes.CatalogStreamItem{
		{Namespace: "n", ObjectID: "o1", Content: "c"},
		{Namespace: "n", ObjectID: "o2", Content: "c"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if len(ids) != 0 {
		t.Errorf("ids = %v, want none", ids)
	}
	if len(f.calls) != 1 {
		t.Errorf("calls = %d — batch must stop at the first failure", len(f.calls))
	}
}
