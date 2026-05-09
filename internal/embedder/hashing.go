package embedder

import (
	"context"
	"fmt"
	"math"

	"github.com/cespare/xxhash/v2"

	"github.com/jarviisha/codohue/internal/core/embedstrategy"
)

// hashingNgramsID is the immutable strategy identifier for V1.
// Bumping the version (e.g. to "v2") requires a namespace-wide re-embed
// because vectors produced under different versions are not comparable.
const (
	hashingNgramsID      = "internal-hashing-ngrams"
	hashingNgramsVersion = "v1"
)

// hashingNgramsValidDims is the set of output dimensions V1 supports.
// Powers of two; the choice is exposed to operators via the strategy
// descriptor list so the admin UI can present the dim that matches the
// namespace's embedding_dim.
var hashingNgramsValidDims = []int{64, 128, 256, 512}

// hashingNgrams is the V1 deterministic, training-free embedding strategy.
//
// Algorithm (per research.md R1):
//
//  1. Tokenize content into word + character-n-gram features.
//  2. For each feature:
//     - hash := xxhash64(feature_bytes)
//     - slot := hash mod dim
//     - sign := -1 if (hash >> 63) & 1 else +1   (Weinberger sign trick)
//     - vec[slot] += sign
//
//  3. L2-normalise vec.
//
// Properties:
//   - Bitwise-deterministic: identical input always produces identical output.
//   - Training-free: no corpus statistics, no shipped artefacts.
//   - Language-agnostic: tokenizer drives all language behaviour.
type hashingNgrams struct {
	dim int
}

func (h *hashingNgrams) ID() string         { return hashingNgramsID }
func (h *hashingNgrams) Version() string    { return hashingNgramsVersion }
func (h *hashingNgrams) Dim() int           { return h.dim }
func (h *hashingNgrams) MaxInputBytes() int { return 0 } // namespace-level cap applies

// Embed produces an L2-normalised dense vector. Returns ErrZeroNorm when
// the content has no surviving features (e.g. punctuation-only) or the
// signed-sum vector cancels exactly to zero (a rare but possible outcome
// when balanced positive/negative collisions sum to zero).
func (h *hashingNgrams) Embed(ctx context.Context, content string) ([]float32, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	features := Tokenize(content)
	if len(features) == 0 {
		return nil, embedstrategy.ErrZeroNorm
	}

	vec := make([]float32, h.dim)
	dim := uint64(h.dim)
	for _, f := range features {
		hash := xxhash.Sum64(f)
		slot := hash % dim
		sign := float32(1)
		if (hash>>63)&1 == 1 {
			sign = -1
		}
		vec[slot] += sign
	}

	var sumSq float32
	for _, v := range vec {
		sumSq += v * v
	}
	if sumSq == 0 {
		return nil, embedstrategy.ErrZeroNorm
	}
	invNorm := float32(1.0 / math.Sqrt(float64(sumSq)))
	for i := range vec {
		vec[i] *= invNorm
	}

	return vec, nil
}

// newHashingNgrams is the Factory for the V1 strategy. Operator-supplied
// Params MUST include "dim" with a value matching one of the registered
// variants (see hashingNgramsValidDims) — otherwise enabling catalog mode
// for a namespace fails fast with a clear error.
func newHashingNgrams(p embedstrategy.Params) (embedstrategy.Strategy, error) {
	raw, ok := p["dim"]
	if !ok {
		return nil, fmt.Errorf("%s: 'dim' param is required (one of %v)", hashingNgramsID, hashingNgramsValidDims)
	}
	var dim int
	switch v := raw.(type) {
	case int:
		dim = v
	case int32:
		dim = int(v)
	case int64:
		dim = int(v)
	case float32:
		dim = int(v)
	case float64:
		dim = int(v)
	default:
		return nil, fmt.Errorf("%s: 'dim' must be a number, got %T", hashingNgramsID, raw)
	}
	if !validHashingDim(dim) {
		return nil, fmt.Errorf("%s: 'dim' must be one of %v, got %d", hashingNgramsID, hashingNgramsValidDims, dim)
	}
	return &hashingNgrams{dim: dim}, nil
}

func validHashingDim(d int) bool {
	for _, v := range hashingNgramsValidDims {
		if d == v {
			return true
		}
	}
	return false
}
