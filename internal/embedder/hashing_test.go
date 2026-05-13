package embedder

import (
	"context"
	"errors"
	"math"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jarviisha/codohue/internal/core/embedstrategy"
)

func TestHashingNgrams_FactoryRequiresDim(t *testing.T) {
	if _, err := newHashingNgrams(embedstrategy.Params{}); err == nil {
		t.Fatal("expected error when dim missing")
	}
}

func TestHashingNgrams_FactoryRejectsInvalidDim(t *testing.T) {
	cases := []embedstrategy.Params{
		{"dim": 0},
		{"dim": 100},
		{"dim": -128},
		{"dim": 1024},
		{"dim": "128"},
	}
	for _, p := range cases {
		p := p
		t.Run("", func(t *testing.T) {
			if _, err := newHashingNgrams(p); err == nil {
				t.Fatalf("expected error for invalid params %v", p)
			}
		})
	}
}

func TestHashingNgrams_FactoryAcceptsValidDims(t *testing.T) {
	for _, dim := range hashingNgramsValidDims {
		dim := dim
		s, err := newHashingNgrams(embedstrategy.Params{"dim": dim})
		if err != nil {
			t.Fatalf("dim=%d: unexpected error %v", dim, err)
		}
		if s.Dim() != dim {
			t.Fatalf("dim=%d: Dim() returned %d", dim, s.Dim())
		}
		if s.ID() != hashingNgramsID || s.Version() != hashingNgramsVersion {
			t.Fatalf("ID/Version: got %s@%s", s.ID(), s.Version())
		}
		if s.MaxInputBytes() != 0 {
			t.Errorf("MaxInputBytes: got %d, want 0", s.MaxInputBytes())
		}
	}
}

func TestHashingNgrams_FactoryAcceptsFloat64Dim(t *testing.T) {
	// JSON unmarshal yields float64 for numbers; the factory must accept it.
	s, err := newHashingNgrams(embedstrategy.Params{"dim": float64(128)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Dim() != 128 {
		t.Errorf("Dim: got %d, want 128", s.Dim())
	}
}

func TestHashingNgrams_EmbedProducesCorrectDim(t *testing.T) {
	s, err := newHashingNgrams(embedstrategy.Params{"dim": 128})
	if err != nil {
		t.Fatal(err)
	}
	vec, err := s.Embed(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vec) != 128 {
		t.Fatalf("vec length: got %d, want 128", len(vec))
	}
}

func TestHashingNgrams_EmbedDeterministic(t *testing.T) {
	s, err := newHashingNgrams(embedstrategy.Params{"dim": 128})
	if err != nil {
		t.Fatal(err)
	}
	content := "Hôm nay trời đẹp quá, ai cũng muốn ra biển! #weekend"
	a, err := s.Embed(context.Background(), content)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Embed(context.Background(), content)
	if err != nil {
		t.Fatal(err)
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("non-deterministic at position %d: %v vs %v", i, a[i], b[i])
		}
	}
}

func TestHashingNgrams_EmbedL2Normalised(t *testing.T) {
	s, err := newHashingNgrams(embedstrategy.Params{"dim": 256})
	if err != nil {
		t.Fatal(err)
	}
	vec, err := s.Embed(context.Background(), "the quick brown fox jumps over the lazy dog")
	if err != nil {
		t.Fatal(err)
	}
	var sumSq float64
	for _, v := range vec {
		sumSq += float64(v) * float64(v)
	}
	got := math.Sqrt(sumSq)
	// Allow small float error; require within 1e-5 of unit length.
	if math.Abs(got-1.0) > 1e-5 {
		t.Fatalf("vec is not unit-norm: got %v", got)
	}
}

func TestHashingNgrams_EmbedEmptyContentReturnsZeroNorm(t *testing.T) {
	s, err := newHashingNgrams(embedstrategy.Params{"dim": 128})
	if err != nil {
		t.Fatal(err)
	}
	for _, content := range []string{"", "   ", "!!! ???", "..."} {
		_, err := s.Embed(context.Background(), content)
		if !errors.Is(err, embedstrategy.ErrZeroNorm) {
			t.Errorf("content=%q: expected ErrZeroNorm, got %v", content, err)
		}
	}
}

func TestHashingNgrams_EmbedDifferentDimsProduceDifferentLengths(t *testing.T) {
	for _, dim := range hashingNgramsValidDims {
		dim := dim
		s, err := newHashingNgrams(embedstrategy.Params{"dim": dim})
		if err != nil {
			t.Fatal(err)
		}
		vec, err := s.Embed(context.Background(), "the quick brown fox")
		if err != nil {
			t.Fatalf("dim=%d: %v", dim, err)
		}
		if len(vec) != dim {
			t.Errorf("dim=%d: vec length %d", dim, len(vec))
		}
	}
}

func TestHashingNgrams_EmbedRespectsContextCancellation(t *testing.T) {
	s, err := newHashingNgrams(embedstrategy.Params{"dim": 128})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Embed(ctx, "hello world"); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestHashingNgrams_EmbedDifferentContentProducesDifferentVectors(t *testing.T) {
	// Sanity: two clearly-different inputs should produce vectors with
	// cosine similarity strictly less than 1.0. (We don't assert how
	// much less; we only assert that the function actually distinguishes.)
	s, err := newHashingNgrams(embedstrategy.Params{"dim": 256})
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.Embed(context.Background(), "the quick brown fox jumps over the lazy dog")
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Embed(context.Background(), "Hôm nay trời đẹp quá")
	if err != nil {
		t.Fatal(err)
	}
	var dot float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
	}
	if dot > 0.99 {
		t.Errorf("expected meaningfully different vectors, got cosine=%v", dot)
	}
}

func TestHashingNgrams_EmbedSimilarContentIsCloser(t *testing.T) {
	// Two inputs sharing tokens/n-grams should be more similar than two
	// disjoint inputs. This is a weak quality smoke test; it does not
	// pin down absolute similarity values.
	s, err := newHashingNgrams(embedstrategy.Params{"dim": 256})
	if err != nil {
		t.Fatal(err)
	}
	embed := func(content string) []float32 {
		v, err := s.Embed(context.Background(), content)
		if err != nil {
			t.Fatalf("embed %q: %v", content, err)
		}
		return v
	}
	cosine := func(x, y []float32) float64 {
		var d float64
		for i := range x {
			d += float64(x[i]) * float64(y[i])
		}
		return d
	}

	a := embed("the quick brown fox jumps over the lazy dog")
	aSimilar := embed("the quick brown fox jumped over the lazy dogs")
	bDifferent := embed("Hôm nay trời đẹp quá ai cũng muốn ra biển")

	if cosine(a, aSimilar) <= cosine(a, bDifferent) {
		t.Errorf("expected similar English inputs to be closer than English vs Vietnamese; got %v vs %v",
			cosine(a, aSimilar), cosine(a, bDifferent))
	}
}

// BenchmarkHashingEmbed_1KiB measures end-to-end embed latency on a 1 KiB
// input. The plan's Performance Goals require p95 ≤ 5 ms (T066). This
// benchmark also asserts the budget so a regression in the tokenizer or
// hashing path will surface as a failing benchmark rather than only a
// slower number on the dashboard.
func BenchmarkHashingEmbed_1KiB(b *testing.B) {
	const targetBytes = 1024
	// A representative input: short Vietnamese sentences interleaved with a
	// hashtag. Repeated until just over 1 KiB.
	sample := "Hôm nay trời đẹp quá, ai cũng muốn ra biển! #weekend "
	var sb strings.Builder
	for sb.Len() < targetBytes {
		sb.WriteString(sample)
	}
	content := sb.String()[:targetBytes]

	s, err := newHashingNgrams(embedstrategy.Params{"dim": 128})
	if err != nil {
		b.Fatalf("factory: %v", err)
	}

	// Warm-up to amortise first-run jitter (allocator, branch predictor).
	for i := 0; i < 50; i++ {
		if _, err := s.Embed(context.Background(), content); err != nil {
			b.Fatalf("warm-up embed: %v", err)
		}
	}

	const sampleCount = 1000
	durations := make([]time.Duration, 0, sampleCount)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		start := time.Now()
		if _, err := s.Embed(context.Background(), content); err != nil {
			b.Fatalf("embed: %v", err)
		}
		if i < sampleCount {
			durations = append(durations, time.Since(start))
		}
	}

	if b.N < sampleCount {
		// `go test -bench` may run with a small N when sampling for the
		// final estimate. Skip the p95 guard in that case so the benchmark
		// still reports a number; the guarded path triggers under
		// `-benchtime=1000x` (or higher) which feeds enough iterations.
		return
	}

	b.StopTimer()
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	p95 := durations[(len(durations)*95)/100]
	const budget = 5 * time.Millisecond
	if p95 > budget {
		b.Fatalf("p95 latency over budget: got %s, want <= %s (n=%d)", p95, budget, len(durations))
	}
	b.ReportMetric(float64(p95)/float64(time.Millisecond), "p95-ms")
}
