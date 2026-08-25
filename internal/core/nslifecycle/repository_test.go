package nslifecycle

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeStore struct {
	lifecycle *NamespaceLifecycle
	system    *SystemLifecycle
	errorText string
}

func (f *fakeStore) Activate(_ context.Context, namespace string) (*NamespaceLifecycle, error) {
	if f.lifecycle == nil {
		f.lifecycle = &NamespaceLifecycle{Namespace: namespace, Generation: 1, State: StateActive}
		return cloneLifecycle(f.lifecycle), nil
	}
	if f.lifecycle.State == StateDeleting {
		return nil, ErrNamespaceNotActive
	}
	if f.lifecycle.State == StateDeleted {
		f.lifecycle.Generation++
		f.lifecycle.State = StateActive
		f.lifecycle.LegacyMessagesAllowed = false
	}
	return cloneLifecycle(f.lifecycle), nil
}

func (f *fakeStore) GetNamespace(_ context.Context, _ string) (*NamespaceLifecycle, error) {
	return cloneLifecycle(f.lifecycle), nil
}
func (f *fakeStore) StartDelete(_ context.Context, _ string) (*NamespaceLifecycle, error) {
	f.lifecycle.State = StateDeleting
	return cloneLifecycle(f.lifecycle), nil
}
func (f *fakeStore) CompleteDelete(_ context.Context, _ string, _ int64) error {
	f.lifecycle.State = StateDeleted
	return nil
}
func (f *fakeStore) RecordNamespaceError(_ context.Context, _ string, _ int64, message string) error {
	f.errorText = message
	f.lifecycle.LastError = message
	return nil
}
func (f *fakeStore) GetSystem(context.Context) (*SystemLifecycle, error) {
	if f.system == nil {
		f.system = &SystemLifecycle{State: SystemActive}
	}
	copy := *f.system
	return &copy, nil
}
func (f *fakeStore) StartReset(context.Context) error    { f.system.State = SystemResetting; return nil }
func (f *fakeStore) CompleteReset(context.Context) error { f.system.State = SystemActive; return nil }
func (f *fakeStore) RecordResetError(_ context.Context, message string) error {
	f.system.LastError = message
	return nil
}
func (f *fakeStore) DisableLegacy(_ context.Context, _ string, at time.Time) (bool, error) {
	if f.system.LegacyEnvelopesDisabledAt != nil {
		return false, nil
	}
	f.system.LegacyEnvelopesDisabledAt = &at
	if f.lifecycle != nil {
		f.lifecycle.LegacyMessagesAllowed = false
	}
	return true, nil
}

func cloneLifecycle(in *NamespaceLifecycle) *NamespaceLifecycle {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func TestLifecyclePersistenceTransitionsAndGeneration(t *testing.T) {
	store := &fakeStore{system: &SystemLifecycle{State: SystemActive}}
	first, err := store.Activate(context.Background(), "tenant")
	if err != nil || first.Generation != 1 || first.State != StateActive {
		t.Fatalf("first activation = %+v, %v", first, err)
	}
	if _, err := store.StartDelete(context.Background(), "tenant"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Activate(context.Background(), "tenant"); !errors.Is(err, ErrNamespaceNotActive) {
		t.Fatalf("activate while deleting error = %v", err)
	}
	if err := store.RecordNamespaceError(context.Background(), "tenant", 1, "qdrant down"); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteDelete(context.Background(), "tenant", 1); err != nil {
		t.Fatal(err)
	}
	second, err := store.Activate(context.Background(), "tenant")
	if err != nil || second.Generation != 2 || second.State != StateActive || second.LegacyMessagesAllowed {
		t.Fatalf("recreation = %+v, %v", second, err)
	}
}

func TestSystemResetAndLegacyClosureAreDurableAndIdempotent(t *testing.T) {
	store := &fakeStore{system: &SystemLifecycle{State: SystemActive}, lifecycle: &NamespaceLifecycle{Namespace: "tenant", Generation: 1, State: StateActive, LegacyMessagesAllowed: true}}
	if err := store.StartReset(context.Background()); err != nil {
		t.Fatal(err)
	}
	state, _ := store.GetSystem(context.Background())
	if state.State != SystemResetting {
		t.Fatalf("state = %q", state.State)
	}
	if err := store.RecordResetError(context.Background(), "redis down"); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteReset(context.Background()); err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC()
	changed, err := store.DisableLegacy(context.Background(), "deploy-42", at)
	if err != nil || !changed {
		t.Fatalf("first closure changed=%v err=%v", changed, err)
	}
	changed, err = store.DisableLegacy(context.Background(), "deploy-42", at.Add(time.Hour))
	if err != nil || changed {
		t.Fatalf("second closure changed=%v err=%v", changed, err)
	}
	if store.lifecycle.LegacyMessagesAllowed {
		t.Fatal("legacy allowance reopened")
	}
}
