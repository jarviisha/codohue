package nslifecycle

import (
	"context"
	"testing"

	"github.com/jarviisha/codohue/internal/core/namespace"
)

func TestIncarnationClampsGenerationOnce(t *testing.T) {
	for _, generation := range []int64{-3, 0, 1} {
		inc := NewIncarnation("tenant", generation)
		if inc.Generation() != 1 {
			t.Errorf("generation %d: got %d, want 1", generation, inc.Generation())
		}
		if got := inc.MustPhysicalName(KindTrending); got != "trending:tenant" {
			t.Errorf("generation %d trending key: got %q", generation, got)
		}
	}
	inc := NewIncarnation("tenant", 3)
	if inc.Generation() != 3 || inc.QdrantNamespace() != "tenant_g3" || inc.RedisNamespace() != "tenant:g3" {
		t.Errorf("generation 3: gen=%d qdrant=%q redis=%q", inc.Generation(), inc.QdrantNamespace(), inc.RedisNamespace())
	}
	var zero Incarnation
	if zero.Generation() != 1 {
		t.Errorf("zero value generation: got %d, want 1", zero.Generation())
	}
}

func TestConfigIncarnation(t *testing.T) {
	if inc := ConfigIncarnation("tenant", nil); inc.Generation() != 1 || inc.Namespace() != "tenant" {
		t.Errorf("nil config: %+v", inc)
	}
	if inc := ConfigIncarnation("tenant", &namespace.Config{Generation: 4}); inc.Generation() != 4 {
		t.Errorf("config generation: got %d, want 4", inc.Generation())
	}
	if inc := ConfigIncarnation("tenant", &namespace.Config{}); inc.Generation() != 1 {
		t.Errorf("zero config generation: got %d, want 1", inc.Generation())
	}
}

func TestLeaseIncarnation(t *testing.T) {
	if _, ok := LeaseIncarnation(context.Background(), "tenant"); ok {
		t.Fatal("no lease must not produce an incarnation")
	}
	ctx := ContextWithLease(context.Background(), "tenant", 5, LockShared)
	inc, ok := LeaseIncarnation(ctx, "tenant")
	if !ok || inc.Generation() != 5 || inc.Namespace() != "tenant" {
		t.Fatalf("leased incarnation: ok=%v %+v", ok, inc)
	}
	if _, ok := LeaseIncarnation(ctx, "other"); ok {
		t.Fatal("lease for another namespace must not produce an incarnation")
	}
}
