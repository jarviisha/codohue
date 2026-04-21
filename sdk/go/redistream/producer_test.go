package redistream

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/jarviisha/codohue/pkg/codohuetypes"
)

// fakeXAdder records every XAdd call and returns the configured ID and error.
type fakeXAdder struct {
	calls []redis.XAddArgs
	id    string
	err   error
}

func (f *fakeXAdder) XAdd(_ context.Context, a *redis.XAddArgs) *redis.StringCmd {
	f.calls = append(f.calls, *a)
	cmd := redis.NewStringCmd(context.Background())
	if f.err != nil {
		cmd.SetErr(f.err)
		return cmd
	}
	cmd.SetVal(f.id)
	return cmd
}

func TestProducerPublishUsesDefaultStream(t *testing.T) {
	t.Parallel()

	f := &fakeXAdder{id: "1700000000-0"}
	p := NewProducer(f)

	event := codohuetypes.EventPayload{
		Namespace: "feed",
		SubjectID: "u-1",
		ObjectID:  "o-1",
		Action:    codohuetypes.ActionLike,
		Timestamp: time.Unix(1700000000, 0).UTC(),
	}
	id, err := p.Publish(context.Background(), event)
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
	if call.Stream != codohuetypes.StreamName {
		t.Errorf("stream = %q, want %q", call.Stream, codohuetypes.StreamName)
	}
	valuesMap, ok := call.Values.(map[string]any)
	if !ok {
		t.Fatalf("Values is not a map: %T", call.Values)
	}
	raw, ok := valuesMap[codohuetypes.PayloadField].(string)
	if !ok {
		t.Fatalf("payload field missing or not string: %v", valuesMap)
	}
	var back codohuetypes.EventPayload
	if err := json.Unmarshal([]byte(raw), &back); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if back.SubjectID != "u-1" || back.Action != codohuetypes.ActionLike {
		t.Errorf("decoded payload mismatch: %+v", back)
	}
}

func TestProducerWithStreamOverride(t *testing.T) {
	t.Parallel()

	f := &fakeXAdder{id: "x"}
	p := NewProducer(f, WithStream("custom:events"))

	if _, err := p.Publish(context.Background(), codohuetypes.EventPayload{}); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if f.calls[0].Stream != "custom:events" {
		t.Errorf("stream = %q", f.calls[0].Stream)
	}
}

func TestProducerPublishPropagatesRedisError(t *testing.T) {
	t.Parallel()

	want := errors.New("redis down")
	f := &fakeXAdder{err: want}
	p := NewProducer(f)

	_, err := p.Publish(context.Background(), codohuetypes.EventPayload{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, want) {
		t.Errorf("errors.Is returned false; err = %v", err)
	}
}

func TestProducerPublishBatchEmpty(t *testing.T) {
	t.Parallel()

	f := &fakeXAdder{id: "x"}
	p := NewProducer(f)

	ids, err := p.PublishBatch(context.Background(), nil)
	if err != nil {
		t.Fatalf("PublishBatch: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("ids = %v, want empty", ids)
	}
	if len(f.calls) != 0 {
		t.Errorf("XAdd should not be called on empty batch")
	}
}

func TestProducerPublishBatchHappy(t *testing.T) {
	t.Parallel()

	f := &fakeXAdder{id: "id"}
	p := NewProducer(f)

	events := []codohuetypes.EventPayload{
		{SubjectID: "a"},
		{SubjectID: "b"},
		{SubjectID: "c"},
	}
	ids, err := p.PublishBatch(context.Background(), events)
	if err != nil {
		t.Fatalf("PublishBatch: %v", err)
	}
	if len(ids) != 3 {
		t.Errorf("ids = %v", ids)
	}
	if len(f.calls) != 3 {
		t.Errorf("XAdd calls = %d, want 3", len(f.calls))
	}
}

// stopAfter is a fakeXAdder that errors once it has recorded n calls.
type stopAfter struct {
	fakeXAdder
	limit int
}

func (s *stopAfter) XAdd(ctx context.Context, a *redis.XAddArgs) *redis.StringCmd {
	if len(s.calls) >= s.limit {
		s.err = errors.New("forced failure")
	}
	return s.fakeXAdder.XAdd(ctx, a)
}

func TestProducerPublishBatchReturnsPartialIDsOnFailure(t *testing.T) {
	t.Parallel()

	s := &stopAfter{fakeXAdder: fakeXAdder{id: "ok"}, limit: 2}
	p := NewProducer(s)

	events := []codohuetypes.EventPayload{
		{SubjectID: "a"},
		{SubjectID: "b"},
		{SubjectID: "c"}, // this one fails
	}
	ids, err := p.PublishBatch(context.Background(), events)
	if err == nil {
		t.Fatal("expected error on 3rd event")
	}
	if len(ids) != 2 {
		t.Errorf("partial ids = %v, want 2 entries", ids)
	}
}
