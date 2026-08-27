package nslifecycle

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type fakeLock struct{ release func() }

func (l fakeLock) Release(context.Context) error {
	if l.release != nil {
		l.release()
	}
	return nil
}

type recordingLocker struct {
	events []string
	errAt  string
}

func (l *recordingLocker) Acquire(_ context.Context, key string, mode LockMode) (Lock, error) {
	event := "lock:" + key + ":" + string(mode)
	l.events = append(l.events, event)
	if l.errAt == event {
		return nil, errors.New("lock failed")
	}
	return fakeLock{release: func() { l.events = append(l.events, "unlock:"+key) }}, nil
}

type serviceStore struct {
	*fakeStore
	events *[]string
}

func (s *serviceStore) GetSystem(ctx context.Context) (*SystemLifecycle, error) {
	*s.events = append(*s.events, "read:system")
	return s.fakeStore.GetSystem(ctx)
}
func (s *serviceStore) GetNamespace(ctx context.Context, ns string) (*NamespaceLifecycle, error) {
	*s.events = append(*s.events, "read:"+ns)
	return s.fakeStore.GetNamespace(ctx, ns)
}

func TestWriterLeaseOrderingPostLockRereadAndContext(t *testing.T) {
	locker := &recordingLocker{}
	events := &locker.events
	store := &serviceStore{fakeStore: &fakeStore{
		system:    &SystemLifecycle{State: SystemActive},
		lifecycle: &NamespaceLifecycle{Namespace: "tenant", Generation: 3, State: StateActive},
	}, events: events}
	svc := NewService(store, locker)
	err := svc.WithWriter(context.Background(), "tenant", func(ctx context.Context, lifecycle *NamespaceLifecycle) error {
		*events = append(*events, "mutate")
		if err := RequireNamespaceLease(ctx, "tenant"); err != nil {
			return err
		}
		// The lease must carry the generation the post-lock reread produced, or
		// a fenced write could name a generation the lock does not cover.
		if got, ok := LeaseGeneration(ctx, "tenant"); !ok || got != lifecycle.Generation {
			t.Fatalf("lease generation = %d (ok=%v), want %d", got, ok, lifecycle.Generation)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"lock:global:shared", "lock:namespace:tenant:shared", "read:system", "read:tenant", "mutate", "unlock:namespace:tenant", "unlock:global"}
	if !reflect.DeepEqual(locker.events, want) {
		t.Fatalf("events = %#v, want %#v", locker.events, want)
	}
}

func TestWriterLeaseRejectsInactiveAndResetting(t *testing.T) {
	for name, tc := range map[string]struct {
		system SystemState
		state  NamespaceState
		want   error
	}{
		"resetting": {SystemResetting, StateActive, ErrSystemResetting},
		"deleting":  {SystemActive, StateDeleting, ErrNamespaceNotActive},
	} {
		t.Run(name, func(t *testing.T) {
			store := &fakeStore{system: &SystemLifecycle{State: tc.system}, lifecycle: &NamespaceLifecycle{Namespace: "tenant", Generation: 1, State: tc.state}}
			svc := NewService(store, &recordingLocker{})
			err := svc.WithWriter(context.Background(), "tenant", func(context.Context, *NamespaceLifecycle) error { t.Fatal("mutation ran"); return nil })
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestNestedWriterLeaseDoesNotReacquire(t *testing.T) {
	locker := &recordingLocker{}
	store := &fakeStore{system: &SystemLifecycle{State: SystemActive}, lifecycle: &NamespaceLifecycle{Namespace: "tenant", Generation: 1, State: StateActive}}
	svc := NewService(store, locker)
	err := svc.WithWriter(context.Background(), "tenant", func(ctx context.Context, lifecycle *NamespaceLifecycle) error {
		return svc.WithWriter(ctx, "tenant", func(nested context.Context, nestedLifecycle *NamespaceLifecycle) error {
			if err := RequireNamespaceLease(nested, "tenant"); err != nil {
				return err
			}
			// The inherited lease must still name the current generation —
			// reusing the outer lock is only safe if it covers the same one.
			if got, ok := LeaseGeneration(nested, "tenant"); !ok || got != nestedLifecycle.Generation {
				t.Fatalf("nested lease generation = %d (ok=%v), want %d", got, ok, nestedLifecycle.Generation)
			}
			return nil
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(locker.events) != 4 {
		t.Fatalf("nested acquisition events = %v", locker.events)
	}
}
