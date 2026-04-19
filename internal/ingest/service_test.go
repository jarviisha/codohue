package ingest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jarviisha/codohue/internal/nsconfig"
)

// fakeRepo implements eventInserter for testing.
type fakeRepo struct {
	insertErr    error
	insertCalled bool
	lastEvent    *Event
}

func (f *fakeRepo) Insert(_ context.Context, e *Event) error {
	f.insertCalled = true
	f.lastEvent = e
	return f.insertErr
}

// fakeNsConfig implements nsConfigGetter for testing.
type fakeNsConfig struct {
	cfg *nsconfig.NamespaceConfig
	err error
}

func (f *fakeNsConfig) Get(_ context.Context, _ string) (*nsconfig.NamespaceConfig, error) {
	return f.cfg, f.err
}

func newTestService(repo eventInserter, ns nsConfigGetter) *Service {
	return &Service{repo: repo, nsConfigSvc: ns}
}

func TestNewService(t *testing.T) {
	repo := &Repository{}
	nsSvc := &nsconfig.Service{}
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
			if err := svc.Process(context.Background(), tt.payload); err == nil {
				t.Error("expected validation error, got nil")
			}
		})
	}
}

func TestServiceProcess_DefaultWeight(t *testing.T) {
	repo := &fakeRepo{}
	svc := newTestService(repo, &fakeNsConfig{cfg: nil})

	if err := svc.Process(context.Background(), &EventPayload{
		Namespace: "ns", SubjectID: "u1", ObjectID: "o1",
		Action: ActionLike, Timestamp: time.Now(),
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
		cfg: &nsconfig.NamespaceConfig{
			ActionWeights: map[string]float64{"LIKE": 99.0},
		},
	})

	if err := svc.Process(context.Background(), &EventPayload{
		Namespace: "ns", SubjectID: "u1", ObjectID: "o1",
		Action: ActionLike, Timestamp: time.Now(),
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

	if err := svc.Process(context.Background(), &EventPayload{
		Namespace: "ns", SubjectID: "u1", ObjectID: "o1",
		Action: ActionView, Timestamp: time.Now(),
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if repo.lastEvent.Weight != DefaultActionWeights[ActionView] {
		t.Errorf("weight: got %.1f, want %.1f", repo.lastEvent.Weight, DefaultActionWeights[ActionView])
	}
}

func TestServiceProcess_UnknownAction(t *testing.T) {
	svc := newTestService(&fakeRepo{}, &fakeNsConfig{})

	err := svc.Process(context.Background(), &EventPayload{
		Namespace: "ns", SubjectID: "u1", ObjectID: "o1",
		Action: Action("UNKNOWN"), Timestamp: time.Now(),
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

	err := svc.Process(context.Background(), &EventPayload{
		Namespace: "ns", SubjectID: "u1", ObjectID: "o1",
		Action: ActionView, Timestamp: time.Now(),
	})
	if err == nil {
		t.Error("expected error from repo.Insert, got nil")
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
		Timestamp:       now,
		ObjectCreatedAt: &createdAt,
	}

	if err := svc.Process(context.Background(), payload); err != nil {
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
	if !e.OccurredAt.Equal(payload.Timestamp) {
		t.Errorf("OccurredAt: got %v, want %v", e.OccurredAt, payload.Timestamp)
	}
	if e.ObjectCreatedAt != payload.ObjectCreatedAt {
		t.Errorf("ObjectCreatedAt: got %v, want %v", e.ObjectCreatedAt, payload.ObjectCreatedAt)
	}
}
