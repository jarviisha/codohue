package compute

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"math/rand/v2"
	"sort"
	"time"

	"gonum.org/v1/gonum/mat"

	"github.com/jarviisha/codohue/internal/core/idmap"
	"github.com/qdrant/go-client/qdrant"
)

var qdrantUpsertDenseFn = func(ctx context.Context, client *qdrant.Client, points *qdrant.UpsertPoints) error {
	_, err := client.Upsert(ctx, points)
	if err != nil {
		return fmt.Errorf("qdrant dense upsert: %w", err)
	}
	return nil
}

const (
	denseVectorName  = "dense_interactions"
	denseBatchSize   = 500
	svdMaxMatrixSize = 10_000_000 // 10M elements; refuse SVD beyond this to avoid OOM
)

// ─────────────────────────────────────────────────────────────
// Interaction sequences
// ─────────────────────────────────────────────────────────────

// BuildInteractionSequences groups events by subject into time-ordered item sequences.
// The input must already be sorted by (subject_id, occurred_at); GetAllNamespaceEvents
// guarantees this ordering.
func BuildInteractionSequences(events []*RawEvent) []InteractionSequence {
	if len(events) == 0 {
		return nil
	}

	var seqs []InteractionSequence
	cur := InteractionSequence{SubjectID: events[0].SubjectID}

	for _, e := range events {
		if e.SubjectID != cur.SubjectID {
			if len(cur.ObjectIDs) > 0 {
				seqs = append(seqs, cur)
			}
			cur = InteractionSequence{SubjectID: e.SubjectID}
		}
		cur.ObjectIDs = append(cur.ObjectIDs, e.ObjectID)
	}
	if len(cur.ObjectIDs) > 0 {
		seqs = append(seqs, cur)
	}
	return seqs
}

// ─────────────────────────────────────────────────────────────
// Item2Vec — skip-gram with negative sampling
// ─────────────────────────────────────────────────────────────

// TrainItem2Vec trains skip-gram embeddings on interaction sequences and returns
// a map of item_id → dense vector. Items with fewer than cfg.MinCount interactions
// across all sequences are excluded from the vocabulary.
func TrainItem2Vec(sequences []InteractionSequence, cfg Item2VecConfig) map[string][]float32 {
	// Count item frequencies across all sequences.
	freq := make(map[string]int)
	for _, seq := range sequences {
		for _, item := range seq.ObjectIDs {
			freq[item]++
		}
	}

	// Build vocabulary filtered by min_count.
	vocab := make(map[string]int) // item -> index
	items := make([]string, 0, len(freq))
	counts := make([]int, 0, len(freq))
	for item, count := range freq {
		if count >= cfg.MinCount {
			vocab[item] = len(items)
			items = append(items, item)
			counts = append(counts, count)
		}
	}

	vocabSize := len(vocab)
	if vocabSize < 2 {
		slog.Warn("item2vec: vocabulary too small, skipping training", "vocab_size", vocabSize)
		return nil
	}
	slog.Info("item2vec: vocabulary built", "vocab_size", vocabSize, "min_count", cfg.MinCount)

	// Initialize embedding matrices with small random values.
	rng := rand.New(rand.NewPCG(42, 0))
	W := make([][]float32, vocabSize)  // input embeddings
	Wp := make([][]float32, vocabSize) // output embeddings
	for i := range W {
		W[i] = randomVec(rng, cfg.Dim)
		Wp[i] = make([]float32, cfg.Dim) // output starts at zero
	}

	// Pre-build noise sampler for negative sampling (unigram^0.75 distribution).
	sampler := newNoiseSampler(rng, counts)

	// Pre-filter sequences to vocabulary indices to avoid repeated map lookups.
	type filteredSeq []int
	filtered := make([]filteredSeq, 0, len(sequences))
	for _, seq := range sequences {
		fs := make(filteredSeq, 0, len(seq.ObjectIDs))
		for _, item := range seq.ObjectIDs {
			if idx, ok := vocab[item]; ok {
				fs = append(fs, idx)
			}
		}
		if len(fs) >= 2 {
			filtered = append(filtered, fs)
		}
	}

	startLR := float32(0.025)
	minLR := float32(0.0001)

	for epoch := 0; epoch < cfg.Epochs; epoch++ {
		// Shuffle sequences each epoch.
		rng.Shuffle(len(filtered), func(i, j int) { filtered[i], filtered[j] = filtered[j], filtered[i] })

		// Linearly decay learning rate over epochs.
		progress := float32(epoch) / float32(cfg.Epochs)
		lr := max32(minLR, startLR*(1-progress))

		for _, seq := range filtered {
			for pos, targetIdx := range seq {
				start := max(0, pos-cfg.Window)
				end := min(len(seq), pos+cfg.Window+1)
				for ctxPos := start; ctxPos < end; ctxPos++ {
					if ctxPos == pos {
						continue
					}
					ctxIdx := seq[ctxPos]
					sgdUpdate(W[targetIdx], Wp[ctxIdx], 1.0, lr)

					for k := 0; k < cfg.NegSamples; k++ {
						negIdx := sampler.sample()
						if negIdx == targetIdx {
							continue
						}
						sgdUpdate(W[targetIdx], Wp[negIdx], 0.0, lr)
					}
				}
			}
		}
	}
	slog.Info("item2vec: training complete", "vocab_size", vocabSize, "epochs", cfg.Epochs)

	result := make(map[string][]float32, vocabSize)
	for item, idx := range vocab {
		result[item] = W[idx]
	}
	return result
}

// ─────────────────────────────────────────────────────────────
// SVD — truncated matrix factorization
// ─────────────────────────────────────────────────────────────

// SVDEmbeddings builds a decay-weighted user×item interaction matrix,
// applies truncated SVD, and returns item_id → dense vector (the item latent factors).
// Returns an error if the matrix size exceeds svdMaxMatrixSize to prevent OOM.
func SVDEmbeddings(events []*RawEvent, embeddingDim int) (map[string][]float32, error) {
	// Build index maps for subjects and objects.
	subjectIndex := make(map[string]int)
	objectIndex := make(map[string]int)
	for _, e := range events {
		if _, ok := subjectIndex[e.SubjectID]; !ok {
			subjectIndex[e.SubjectID] = len(subjectIndex)
		}
		if _, ok := objectIndex[e.ObjectID]; !ok {
			objectIndex[e.ObjectID] = len(objectIndex)
		}
	}

	nSubjects := len(subjectIndex)
	nObjects := len(objectIndex)

	if nSubjects == 0 || nObjects == 0 {
		return nil, nil
	}
	if nSubjects*nObjects > svdMaxMatrixSize {
		return nil, fmt.Errorf("SVD matrix too large (%d×%d = %d elements > %d limit); use item2vec or reduce namespace size",
			nSubjects, nObjects, nSubjects*nObjects, svdMaxMatrixSize)
	}

	// Build dense interaction matrix with time-decay weights.
	now := time.Now().Unix()
	data := make([]float64, nSubjects*nObjects)
	for _, e := range events {
		si := subjectIndex[e.SubjectID]
		oi := objectIndex[e.ObjectID]
		daysSince := float64(now-e.OccurredAt) / 86400.0
		score := e.Weight * math.Exp(-defaultLambda*daysSince)
		data[si*nObjects+oi] += score
	}

	R := mat.NewDense(nSubjects, nObjects, data)

	// Truncated SVD: R ≈ U Σ V^T
	var svd mat.SVD
	if ok := svd.Factorize(R, mat.SVDThin); !ok {
		return nil, fmt.Errorf("SVD factorization failed")
	}

	// V has shape (nObjects × k) where k = min(nSubjects, nObjects).
	// Row i of V is the latent factor for item i.
	var V mat.Dense
	svd.VTo(&V)

	_, vCols := V.Dims()
	rank := min(embeddingDim, vCols)

	// Scale item vectors by sqrt of singular values for better quality.
	singVals := svd.Values(nil)

	result := make(map[string][]float32, nObjects)
	// Build reverse index: object index -> string ID.
	invObjectIndex := make([]string, nObjects)
	for id, idx := range objectIndex {
		invObjectIndex[idx] = id
	}
	for oi, itemID := range invObjectIndex {
		vec := make([]float32, rank)
		for d := range rank {
			vec[d] = float32(V.At(oi, d) * math.Sqrt(singVals[d]))
		}
		result[itemID] = vec
	}

	slog.Info("svd: embeddings computed", "objects", nObjects, "rank", rank)
	return result, nil
}

// ─────────────────────────────────────────────────────────────
// Qdrant upsert
// ─────────────────────────────────────────────────────────────

// UpsertItemDenseVectors upserts item dense vectors into {ns}_objects_dense.
func UpsertItemDenseVectors(ctx context.Context, qdrantClient *qdrant.Client, idmapSvc *idmap.Service, namespace, strategy string, itemVecs map[string][]float32) error {
	return upsertDenseVectors(ctx, qdrantClient, idmapSvc, namespace+"_objects_dense", namespace, "object", strategy, itemVecs)
}

// UpsertSubjectDenseVectors upserts subject dense vectors into {ns}_subjects_dense.
func UpsertSubjectDenseVectors(ctx context.Context, qdrantClient *qdrant.Client, idmapSvc *idmap.Service, namespace, strategy string, subjectVecs map[string][]float32) error {
	return upsertDenseVectors(ctx, qdrantClient, idmapSvc, namespace+"_subjects_dense", namespace, "subject", strategy, subjectVecs)
}

func upsertDenseVectors(ctx context.Context, qdrantClient *qdrant.Client, idmapSvc *idmap.Service, collection, namespace, entityType, strategy string, vecs map[string][]float32) error {
	// Sort entity IDs for deterministic batching.
	ids := make([]string, 0, len(vecs))
	for id := range vecs {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	updatedAt := time.Now().UTC().Format(time.RFC3339)
	idKey := entityType + "_id" // "object_id" or "subject_id"

	var batch []*qdrant.PointStruct

	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		err := qdrantUpsertDenseFn(ctx, qdrantClient, &qdrant.UpsertPoints{
			CollectionName: collection,
			Points:         batch,
		})
		batch = batch[:0]
		if err != nil {
			return fmt.Errorf("flush dense batch to qdrant: %w", err)
		}
		return nil
	}

	for _, entityID := range ids {
		vec := vecs[entityID]

		var numID uint64
		var err error
		if entityType == "object" {
			numID, err = idmapSvc.GetOrCreateObjectID(ctx, entityID, namespace)
		} else {
			numID, err = idmapSvc.GetOrCreateSubjectID(ctx, entityID, namespace)
		}
		if err != nil {
			slog.Error("upsert dense: get numeric id failed", "entity", entityID, "error", err)
			continue
		}

		batch = append(batch, &qdrant.PointStruct{
			Id: qdrant.NewIDNum(numID),
			Vectors: &qdrant.Vectors{
				VectorsOptions: &qdrant.Vectors_Vectors{
					Vectors: &qdrant.NamedVectors{
						Vectors: map[string]*qdrant.Vector{
							denseVectorName: qdrant.NewVectorDense(vec),
						},
					},
				},
			},
			Payload: map[string]*qdrant.Value{
				idKey:        qdrant.NewValueString(entityID),
				"strategy":   qdrant.NewValueString(strategy),
				"updated_at": qdrant.NewValueString(updatedAt),
			},
		})

		if len(batch) >= denseBatchSize {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	return flush()
}

// ─────────────────────────────────────────────────────────────
// Internal helpers
// ─────────────────────────────────────────────────────────────

// noiseSampler samples item indices from a unigram^0.75 distribution using
// the inverse CDF method.
type noiseSampler struct {
	cumProbs []float64
	rng      *rand.Rand
}

func newNoiseSampler(rng *rand.Rand, counts []int) *noiseSampler {
	probs := make([]float64, len(counts))
	total := 0.0
	for i, c := range counts {
		p := math.Pow(float64(c), 0.75)
		probs[i] = p
		total += p
	}
	cum := make([]float64, len(counts))
	running := 0.0
	for i, p := range probs {
		running += p / total
		cum[i] = running
	}
	return &noiseSampler{cumProbs: cum, rng: rng}
}

func (s *noiseSampler) sample() int {
	r := s.rng.Float64()
	lo, hi := 0, len(s.cumProbs)-1
	for lo < hi {
		mid := (lo + hi) / 2
		if s.cumProbs[mid] < r {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo
}

// sgdUpdate performs one negative-sampling SGD step for a (target, output) pair.
// label is 1.0 for positive pairs and 0.0 for negative pairs.
func sgdUpdate(target, output []float32, label, lr float32) {
	dot := float32(0)
	for d := range target {
		dot += target[d] * output[d]
	}
	sig := sigmoid32(dot)
	grad := lr * (label - sig)
	for d := range target {
		t := target[d]
		target[d] += grad * output[d]
		output[d] += grad * t
	}
}

func sigmoid32(x float32) float32 {
	return float32(1.0 / (1.0 + math.Exp(-float64(x))))
}

func randomVec(rng *rand.Rand, dim int) []float32 {
	v := make([]float32, dim)
	for i := range v {
		v[i] = (rng.Float32() - 0.5) / float32(dim)
	}
	return v
}

func max32(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}
