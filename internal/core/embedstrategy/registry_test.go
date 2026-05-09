package embedstrategy

import (
	"context"
	"errors"
	"sync"
	"testing"
)

type stubStrategy struct {
	id, version string
	dim         int
	maxIn       int
	embedFn     func(ctx context.Context, content string) ([]float32, error)
}

func (s *stubStrategy) ID() string            { return s.id }
func (s *stubStrategy) Version() string       { return s.version }
func (s *stubStrategy) Dim() int              { return s.dim }
func (s *stubStrategy) MaxInputBytes() int    { return s.maxIn }
func (s *stubStrategy) Embed(ctx context.Context, content string) ([]float32, error) {
	if s.embedFn != nil {
		return s.embedFn(ctx, content)
	}
	return make([]float32, s.dim), nil
}

func newStubFactory(id, version string, dim int) Factory {
	return func(_ Params) (Strategy, error) {
		return &stubStrategy{id: id, version: version, dim: dim}, nil
	}
}

func TestRegistry_RegisterAndBuild(t *testing.T) {
	r := NewRegistry()
	r.Register("test-id", "v1", newStubFactory("test-id", "v1", 64))

	if !r.Has("test-id", "v1") {
		t.Fatal("Has returned false for registered (id, version)")
	}

	s, err := r.Build("test-id", "v1", Params{})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if s.ID() != "test-id" || s.Version() != "v1" || s.Dim() != 64 {
		t.Fatalf("unexpected built strategy: %+v", s)
	}
}

func TestRegistry_BuildUnknownReturnsSentinel(t *testing.T) {
	r := NewRegistry()
	_, err := r.Build("missing", "v1", nil)
	if !errors.Is(err, ErrUnknownStrategy) {
		t.Fatalf("expected ErrUnknownStrategy, got %v", err)
	}
}

func TestRegistry_DuplicateRegistrationPanics(t *testing.T) {
	r := NewRegistry()
	r.Register("dup", "v1", newStubFactory("dup", "v1", 64))

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on duplicate registration")
		}
	}()
	r.Register("dup", "v1", newStubFactory("dup", "v1", 64))
}

func TestRegistry_EmptyIDOrVersionPanics(t *testing.T) {
	r := NewRegistry()

	cases := []struct {
		id, version string
	}{
		{"", "v1"},
		{"id", ""},
	}
	for _, c := range cases {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("expected panic for id=%q version=%q", c.id, c.version)
				}
			}()
			r.Register(c.id, c.version, newStubFactory("x", "y", 64))
		}()
	}
}

func TestRegistry_NilFactoryPanics(t *testing.T) {
	r := NewRegistry()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on nil factory")
		}
	}()
	r.Register("nil-factory", "v1", nil)
}

func TestRegistry_HasUnknownReturnsFalse(t *testing.T) {
	r := NewRegistry()
	if r.Has("absent", "v1") {
		t.Fatal("Has returned true for unregistered (id, version)")
	}
}

func TestRegistry_List_FixedDimFactory(t *testing.T) {
	r := NewRegistry()
	r.Register("alpha", "v1", newStubFactory("alpha", "v1", 64))
	r.Register("beta", "v2", newStubFactory("beta", "v2", 128))

	descs := r.List()
	if len(descs) != 2 {
		t.Fatalf("expected 2 descriptors, got %d", len(descs))
	}
	// Sorted by (id, version, dim): alpha@v1 then beta@v2.
	if descs[0].ID != "alpha" || descs[1].ID != "beta" {
		t.Fatalf("unsorted descriptors: %+v", descs)
	}
	if descs[0].Dim != 64 || descs[1].Dim != 128 {
		t.Fatalf("unexpected dims: %+v", descs)
	}
}

func TestRegistry_RegisterVariants_ListsAllVariants(t *testing.T) {
	r := NewRegistry()
	variants := []StrategyDescriptor{
		{ID: "hash", Version: "v1", Dim: 64},
		{ID: "hash", Version: "v1", Dim: 128},
		{ID: "hash", Version: "v1", Dim: 256},
	}
	r.RegisterVariants("hash", "v1", func(p Params) (Strategy, error) {
		dim := 64
		if v, ok := p["dim"].(int); ok {
			dim = v
		}
		return &stubStrategy{id: "hash", version: "v1", dim: dim}, nil
	}, variants)

	descs := r.List()
	if len(descs) != 3 {
		t.Fatalf("expected 3 descriptors, got %d", len(descs))
	}
	for i, want := range []int{64, 128, 256} {
		if descs[i].Dim != want {
			t.Fatalf("variant %d: want dim=%d got %d", i, want, descs[i].Dim)
		}
	}
}

func TestRegistry_RegisterVariants_MismatchedDescriptorPanics(t *testing.T) {
	r := NewRegistry()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on mismatched variant descriptor")
		}
	}()
	r.RegisterVariants("hash", "v1", newStubFactory("hash", "v1", 64), []StrategyDescriptor{
		{ID: "hash", Version: "v2", Dim: 64},
	})
}

func TestRegistry_List_SkipsFactoriesThatFailWithEmptyParams(t *testing.T) {
	r := NewRegistry()
	r.Register("ok", "v1", newStubFactory("ok", "v1", 64))
	r.Register("needs-creds", "v1", func(p Params) (Strategy, error) {
		if _, ok := p["api_key"]; !ok {
			return nil, errors.New("missing api_key")
		}
		return &stubStrategy{id: "needs-creds", version: "v1", dim: 1024}, nil
	})

	descs := r.List()
	if len(descs) != 1 || descs[0].ID != "ok" {
		t.Fatalf("expected only the ok descriptor, got %+v", descs)
	}
}

func TestRegistry_Concurrent_RegisterAndBuild(t *testing.T) {
	r := NewRegistry()
	const N = 32
	var wg sync.WaitGroup

	// Pre-register one entry that the readers will Build.
	r.Register("shared", "v1", newStubFactory("shared", "v1", 64))

	wg.Add(N * 2)
	for i := 0; i < N; i++ {
		go func(i int) {
			defer wg.Done()
			id := "id-" + itoa(i)
			r.Register(id, "v1", newStubFactory(id, "v1", 64))
		}(i)
		go func() {
			defer wg.Done()
			if _, err := r.Build("shared", "v1", nil); err != nil {
				t.Errorf("concurrent Build failed: %v", err)
			}
		}()
	}
	wg.Wait()

	// Every concurrently-registered entry must be visible.
	for i := 0; i < N; i++ {
		if !r.Has("id-"+itoa(i), "v1") {
			t.Errorf("concurrent registration lost: id-%d", i)
		}
	}
}

func TestDefaultRegistry_ReturnsProcessSingleton(t *testing.T) {
	if DefaultRegistry() != DefaultRegistry() {
		t.Fatal("DefaultRegistry returned different instances")
	}
}

// itoa avoids strconv to keep the test file dependency-free.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
