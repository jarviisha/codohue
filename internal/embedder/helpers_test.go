package embedder

import "testing"

func TestStringField(t *testing.T) {
	m := map[string]any{"title": "hello", "count": 3}
	if got := stringField(m, "title"); got != "hello" {
		t.Errorf("present string: got %q, want hello", got)
	}
	if got := stringField(m, "missing"); got != "" {
		t.Errorf("missing key: got %q, want empty", got)
	}
	if got := stringField(m, "count"); got != "" {
		t.Errorf("non-string value: got %q, want empty", got)
	}
}

func TestStrategyCacheKey(t *testing.T) {
	noParams, err := strategyCacheKey("item2vec", "v1", nil)
	if err != nil {
		t.Fatalf("nil params: %v", err)
	}
	if noParams == "" {
		t.Error("nil params produced empty key")
	}

	withParams, err := strategyCacheKey("item2vec", "v1", map[string]any{"dim": 384})
	if err != nil {
		t.Fatalf("with params: %v", err)
	}
	if withParams == noParams {
		t.Error("params should change the cache key")
	}

	// Deterministic for identical input; sensitive to version.
	again, _ := strategyCacheKey("item2vec", "v1", map[string]any{"dim": 384})
	if again != withParams {
		t.Error("same input should yield the same key")
	}
	otherVersion, _ := strategyCacheKey("item2vec", "v2", map[string]any{"dim": 384})
	if otherVersion == withParams {
		t.Error("different version should change the key")
	}
}
