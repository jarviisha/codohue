package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// fakeProcessor implements eventProcessor for testing.
type fakeProcessor struct {
	processErr    error
	processCalled bool
	lastPayload   *EventPayload
}

func (f *fakeProcessor) Process(_ context.Context, p *EventPayload) error {
	f.processCalled = true
	f.lastPayload = p
	return f.processErr
}

func TestWorkerHandleMessage_MissingPayload(t *testing.T) {
	w := &Worker{}
	msg := redis.XMessage{ID: "1-0", Values: map[string]any{}}

	if err := w.handleMessage(context.Background(), msg); err == nil {
		t.Error("expected error for missing payload field, got nil")
	}
}

func TestWorkerHandleMessage_InvalidJSON(t *testing.T) {
	w := &Worker{}
	msg := redis.XMessage{ID: "1-0", Values: map[string]any{"payload": "not-valid-json"}}

	if err := w.handleMessage(context.Background(), msg); err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestWorkerHandleMessage_ValidPayload(t *testing.T) {
	proc := &fakeProcessor{}
	w := &Worker{service: proc}

	now := time.Now().UTC()
	p := EventPayload{
		Namespace: "test-ns",
		SubjectID: "user-1",
		ObjectID:  "item-1",
		Action:    ActionLike,
		Timestamp: now,
	}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	msg := redis.XMessage{ID: "1-0", Values: map[string]any{"payload": string(raw)}}

	if err := w.handleMessage(context.Background(), msg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !proc.processCalled {
		t.Fatal("expected service.Process to be called")
	}
	if proc.lastPayload.Namespace != p.Namespace {
		t.Errorf("Namespace: got %q, want %q", proc.lastPayload.Namespace, p.Namespace)
	}
	if proc.lastPayload.Action != p.Action {
		t.Errorf("Action: got %q, want %q", proc.lastPayload.Action, p.Action)
	}
}

func TestWorkerHandleMessage_ServiceError(t *testing.T) {
	proc := &fakeProcessor{processErr: errors.New("process failed")}
	w := &Worker{service: proc}

	p := EventPayload{Namespace: "ns", SubjectID: "u1", ObjectID: "o1", Action: ActionView}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	msg := redis.XMessage{ID: "1-0", Values: map[string]any{"payload": string(raw)}}

	if err := w.handleMessage(context.Background(), msg); err == nil {
		t.Error("expected error from service.Process, got nil")
	}
}

func TestNewWorker(t *testing.T) {
	svc := &Service{}
	w := NewWorker(nil, svc)
	if w == nil || w.service != svc {
		t.Fatal("expected worker to wire dependencies")
	}
}

func TestWorkerInit_AllowsBusyGroup(t *testing.T) {
	w := &Worker{
		createGroupFn: func(_ context.Context, _, _, _ string) error {
			return errors.New("BUSYGROUP Consumer Group name already exists")
		},
	}
	if err := w.Init(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWorkerInit_ReturnsCreateGroupError(t *testing.T) {
	w := &Worker{
		createGroupFn: func(_ context.Context, _, _, _ string) error {
			return errors.New("redis down")
		},
	}
	if err := w.Init(context.Background()); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestWorkerRun_StopsOnCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	w := &Worker{
		readGroupFn: func(_ context.Context, _ *redis.XReadGroupArgs) ([]redis.XStream, error) {
			t.Fatal("readGroupFn should not be called after cancellation")
			return nil, nil
		},
	}

	w.Run(ctx)
}

func TestWorkerRun_ContinuesOnReadErrorAndAcksProcessedMessages(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	proc := &fakeProcessor{}
	readCalls := 0
	acked := []string{}
	w := &Worker{
		service: proc,
		readGroupFn: func(_ context.Context, _ *redis.XReadGroupArgs) ([]redis.XStream, error) {
			readCalls++
			switch readCalls {
			case 1:
				return nil, errors.New("temporary redis error")
			case 2:
				payload, err := json.Marshal(EventPayload{Namespace: "ns", SubjectID: "u1", ObjectID: "o1", Action: ActionView})
				if err != nil {
					t.Fatalf("marshal payload: %v", err)
				}
				cancel()
				return []redis.XStream{{
					Stream: streamName,
					Messages: []redis.XMessage{{
						ID:     "1-0",
						Values: map[string]any{"payload": string(payload)},
					}},
				}}, nil
			default:
				return nil, context.Canceled
			}
		},
		ackFn: func(_ context.Context, _, _ string, ids ...string) error {
			acked = append(acked, ids...)
			return nil
		},
	}

	w.Run(ctx)

	if readCalls < 2 {
		t.Fatalf("expected at least 2 read attempts, got %d", readCalls)
	}
	if !proc.processCalled {
		t.Fatal("expected payload to be processed")
	}
	if len(acked) != 1 || acked[0] != "1-0" {
		t.Fatalf("unexpected acked ids: %v", acked)
	}
}
