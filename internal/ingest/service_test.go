package ingest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jarviisha/codohue/internal/core/namespace"
)

// fakeRepo implements eventInserter for testing.
type fakeRepo struct {
	insertErr    error
	insertCalled bool
	insertID     int64
	lastEvent    *Event
}

func (f *fakeRepo) Insert(_ context.Context, e *Event) error {
	f.insertCalled = true
	f.lastEvent = e
	if f.insertErr == nil {
		e.ID = f.insertID
	}
	return f.insertErr
}

// fakeTailPublisher records the last message handed to the tail.
type fakeTailPublisher struct {
	called bool
	last   EventTailMessage
}

func (f *fakeTailPublisher) Publish(msg EventTailMessage) {
	f.called = true
	f.last = msg
}

// fakeNsConfig implements nsConfigGetter for testing.
type fakeNsConfig struct {
	cfg *namespace.Config
	err error
}

func (f *fakeNsConfig) Get(_ context.Context, _ string) (*namespace.Config, error) {
	return f.cfg, f.err
}

func newTestService(repo eventInserter, ns nsConfigGetter) *Service {
	return &Service{repo: repo, nsConfigSvc: ns}
}

func TestNewService(t *testing.T) {
	repo := &Repository{}
	nsSvc := &fakeNsConfig{}
	svc := NewService(repo, nsSvc)
	if svc == nil || svc.repo != repo || svc.nsConfigSvc != nsSvc {
		t.Fatal("expected NewService to wire dependencies")
	}
}

func TestServiceProcess_Validation(t *testing.T) {
	svc := &Service{}

	cases := []struct {
		name    string
		payload *EventPayload
	}{
		{"missing namespace", &EventPayload{SubjectID: "u1", ObjectID: "o1", Action: ActionView}},
		{"missing subject_id", &EventPayload{Namespace: "ns", ObjectID: "o1", Action: ActionView}},
		{"missing object_id", &EventPayload{Namespace: "ns", SubjectID: "u1", Action: ActionView}},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := svc.Process(context.Background(), tt.payload); err == nil {
				t.Error("expected validation error, got nil")
			}
		})
	}
}

func TestServiceProcess_DefaultWeight(t *testing.T) {
	repo := &fakeRepo{}
	svc := newTestService(repo, &fakeNsConfig{cfg: nil})

	if _, err := svc.Process(context.Background(), &EventPayload{
		Namespace: "ns", SubjectID: "u1", ObjectID: "o1",
		Action: ActionLike, OccurredAt: time.Now(),
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !repo.insertCalled {
		t.Fatal("expected repo.Insert to be called")
	}
	if repo.lastEvent.Weight != DefaultActionWeights[ActionLike] {
		t.Errorf("weight: got %.1f, want %.1f", repo.lastEvent.Weight, DefaultActionWeights[ActionLike])
	}
}

func TestServiceProcess_CustomNamespaceWeight(t *testing.T) {
	repo := &fakeRepo{}
	svc := newTestService(repo, &fakeNsConfig{
		cfg: &namespace.Config{
			ActionWeights: map[string]float64{"LIKE": 99.0},
		},
	})

	if _, err := svc.Process(context.Background(), &EventPayload{
		Namespace: "ns", SubjectID: "u1", ObjectID: "o1",
		Action: ActionLike, OccurredAt: time.Now(),
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if repo.lastEvent.Weight != 99.0 {
		t.Errorf("weight: got %.1f, want 99.0", repo.lastEvent.Weight)
	}
}

func TestServiceProcess_NsConfigError_FallsBackToDefault(t *testing.T) {
	repo := &fakeRepo{}
	svc := newTestService(repo, &fakeNsConfig{err: errors.New("db error")})

	if _, err := svc.Process(context.Background(), &EventPayload{
		Namespace: "ns", SubjectID: "u1", ObjectID: "o1",
		Action: ActionView, OccurredAt: time.Now(),
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if repo.lastEvent.Weight != DefaultActionWeights[ActionView] {
		t.Errorf("weight: got %.1f, want %.1f", repo.lastEvent.Weight, DefaultActionWeights[ActionView])
	}
}

func TestServiceProcess_UnknownAction(t *testing.T) {
	svc := newTestService(&fakeRepo{}, &fakeNsConfig{})

	_, err := svc.Process(context.Background(), &EventPayload{
		Namespace: "ns", SubjectID: "u1", ObjectID: "o1",
		Action: Action("UNKNOWN"), OccurredAt: time.Now(),
	})
	if err == nil {
		t.Error("expected error for unknown action, got nil")
	}
}

func TestServiceProcess_InsertError(t *testing.T) {
	svc := newTestService(
		&fakeRepo{insertErr: errors.New("db error")},
		&fakeNsConfig{},
	)

	_, err := svc.Process(context.Background(), &EventPayload{
		Namespace: "ns", SubjectID: "u1", ObjectID: "o1",
		Action: ActionView, OccurredAt: time.Now(),
	})
	if err == nil {
		t.Error("expected error from repo.Insert, got nil")
	}
}

func TestServiceProcess_ReturnsIDAndPublishesTail(t *testing.T) {
	repo := &fakeRepo{insertID: 4242}
	pub := &fakeTailPublisher{}
	svc := newTestService(repo, &fakeNsConfig{})
	svc.SetTailPublisher(pub)

	now := time.Now().UTC().Truncate(time.Second)
	id, err := svc.Process(context.Background(), &EventPayload{
		Namespace: "ns", SubjectID: "u1", ObjectID: "o1",
		Action: ActionLike, OccurredAt: now,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 4242 {
		t.Errorf("returned id: got %d, want 4242", id)
	}
	if !pub.called {
		t.Fatal("expected tail publisher to be called")
	}
	if pub.last.ID != 4242 || pub.last.SubjectID != "u1" || pub.last.Action != "LIKE" {
		t.Errorf("tail message mismatch: %+v", pub.last)
	}
}

func TestServiceProcess_NoTailOnError(t *testing.T) {
	pub := &fakeTailPublisher{}
	svc := newTestService(&fakeRepo{insertErr: errors.New("db error")}, &fakeNsConfig{})
	svc.SetTailPublisher(pub)

	if _, err := svc.Process(context.Background(), &EventPayload{
		Namespace: "ns", SubjectID: "u1", ObjectID: "o1",
		Action: ActionView, OccurredAt: time.Now(),
	}); err == nil {
		t.Fatal("expected insert error")
	}
	if pub.called {
		t.Error("tail publisher must not fire when ingest fails")
	}
}

func TestServiceProcess_EventFields(t *testing.T) {
	repo := &fakeRepo{}
	svc := newTestService(repo, &fakeNsConfig{})

	now := time.Now().UTC().Truncate(time.Second)
	createdAt := now.Add(-24 * time.Hour)
	payload := &EventPayload{
		Namespace:       "ns",
		SubjectID:       "u1",
		ObjectID:        "o1",
		Action:          ActionShare,
		OccurredAt:      now,
		ObjectCreatedAt: &createdAt,
	}

	if _, err := svc.Process(context.Background(), payload); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	e := repo.lastEvent
	if e.Namespace != payload.Namespace {
		t.Errorf("Namespace: got %q, want %q", e.Namespace, payload.Namespace)
	}
	if e.SubjectID != payload.SubjectID {
		t.Errorf("SubjectID: got %q, want %q", e.SubjectID, payload.SubjectID)
	}
	if e.ObjectID != payload.ObjectID {
		t.Errorf("ObjectID: got %q, want %q", e.ObjectID, payload.ObjectID)
	}
	if e.Action != payload.Action {
		t.Errorf("Action: got %q, want %q", e.Action, payload.Action)
	}
	if !e.OccurredAt.Equal(payload.OccurredAt) {
		t.Errorf("OccurredAt: got %v, want %v", e.OccurredAt, payload.OccurredAt)
	}
	if e.ObjectCreatedAt != payload.ObjectCreatedAt {
		t.Errorf("ObjectCreatedAt: got %v, want %v", e.ObjectCreatedAt, payload.ObjectCreatedAt)
	}
}

func TestProcess_OmittedOccurredAtDefaultsToNow(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(nil, &fakeNsConfig{})
	svc.repo = repo

	before := time.Now().UTC().Add(-time.Second)
	_, err := svc.Process(context.Background(), &EventPayload{
		Namespace: "ns", SubjectID: "u1", ObjectID: "o1", Action: ActionView,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := repo.lastEvent.OccurredAt
	if got.Before(before) || got.After(time.Now().UTC().Add(time.Second)) {
		t.Fatalf("omitted occurred_at must default to now, got %s — year-0001 events were 202'd but invisible to every window", got)
	}
}

func TestProcess_FutureOccurredAtRejected(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(nil, &fakeNsConfig{})
	svc.repo = repo

	_, err := svc.Process(context.Background(), &EventPayload{
		Namespace: "ns", SubjectID: "u1", ObjectID: "o1", Action: ActionView,
		OccurredAt: time.Now().Add(time.Hour),
	})
	if !errors.Is(err, ErrInvalidPayload) {
		t.Fatalf("a future occurred_at must be rejected (it exponentiates into +Inf in the decay math), got %v", err)
	}
	if repo.insertCalled {
		t.Fatal("rejected event must not be stored")
	}
}

func TestProcess_SlightClockSkewTolerated(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(nil, &fakeNsConfig{})
	svc.repo = repo

	if _, err := svc.Process(context.Background(), &EventPayload{
		Namespace: "ns", SubjectID: "u1", ObjectID: "o1", Action: ActionView,
		OccurredAt: time.Now().Add(30 * time.Second),
	}); err != nil {
		t.Fatalf("ordinary clock skew must pass: %v", err)
	}
}
