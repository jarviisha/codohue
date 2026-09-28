package compute

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"math"
	"slices"
	"sort"
	"time"

	"github.com/jarviisha/codohue/internal/core/idmap"
	"github.com/jarviisha/codohue/internal/infra/metrics"
	infraqdrant "github.com/jarviisha/codohue/internal/infra/qdrant"
	"github.com/qdrant/go-client/qdrant"
)

const (
	defaultLambda    = 0.05 // time decay per day
	sparseVectorName = "sparse_interactions"
	qdrantBatchSize  = 100

	// maxSparseIndex bounds the numeric ids that can address a Qdrant sparse
	// vector dimension. idmap hands out uint64 ids from one BIGSERIAL, but the
	// sparse protocol indexes with uint32, so a plain cast folds two distinct
	// entities onto one dimension once ids pass 2^32 — wrong recommendations,
	// no error, nothing in the logs.
	maxSparseIndex = math.MaxUint32
)

// sparseIndex narrows a numeric id to a sparse vector dimension, reporting
// whether the id fits rather than narrowing silently. Callers skip the
// offending dimension and log: the dimension is equally unrepresentable in
// every vector, so dropping it cannot collide or corrupt, whereas propagating
// an error failed the whole subject — or, at the object site, the whole run —
// permanently, because the over-limit id never goes away.
func sparseIndex(numericID uint64) (uint32, bool) {
	if numericID > maxSparseIndex {
		return 0, false
	}
	return uint32(numericID), true
}

// sparseVectorFrom sorts entries by dimension and splits them into the
// parallel index/value slices Qdrant takes, unit-normalized. Both build sites
// need exactly this, and they must agree: if subject and object vectors stop
// being mutually unit length, their dot product stops being a cosine and the
// serving clamp silently mis-scores one side.
func sparseVectorFrom(entries []sparseEntry) (indices []uint32, values []float32) {
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].index < entries[j].index
	})
	indices = make([]uint32, 0, len(entries))
	values = make([]float32, 0, len(entries))
	for _, entry := range entries {
		indices = append(indices, entry.index)
		values = append(values, entry.value)
	}
	l2Normalize(values)
	return indices, values
}

// sparseEntry is one dimension of a sparse vector before it is split into the
// parallel slices Qdrant takes.
type sparseEntry struct {
	index uint32
	value float32
}

// l2Normalize scales values to unit Euclidean norm in place. Qdrant sparse
// search is a raw dot product (sparse vectors have no cosine mode), so without
// this the score scale is arbitrary: measured on live Bluesky data, top-1
// scores spread from 0.08 to 6094 across subjects, and an object touched by
// many subjects accumulated a co-occurrence row whose sheer magnitude — not
// its direction — dominated every search it appeared in. With both sides unit
// length the dot product is a cosine in [-1, 1]: comparable across subjects,
// and popularity expresses itself only through direction. A zero vector stays
// zero.
func l2Normalize(values []float32) {
	var sum float64
	for _, v := range values {
		sum += float64(v) * float64(v)
	}
	if sum == 0 {
		return
	}
	norm := math.Sqrt(sum)
	for i := range values {
		values[i] = float32(float64(values[i]) / norm)
	}
}

type computeRepo interface {
	GetActiveSubjects(ctx context.Context, namespace string) ([]string, error)
	GetSubjectsEvents(ctx context.Context, namespace string, subjectIDs []string) (map[string][]*RawEvent, error)
}

type idmapService interface {
	GetOrCreateSubjectID(ctx context.Context, subjectID, namespace string) (uint64, error)
	GetOrCreateSubjectIDs(ctx context.Context, subjectIDs []string, namespace string) (map[string]uint64, error)
	GetOrCreateObjectID(ctx context.Context, objectID, namespace string) (uint64, error)
	GetOrCreateObjectIDs(ctx context.Context, objectIDs []string, namespace string) (map[string]uint64, error)
}

// Service computes sparse vectors with time decay for each subject in a namespace.
type Service struct {
	repo     computeRepo
	idmapSvc idmapService
	qdrant   *qdrant.Client
	// subjectChunk is how many subjects share one events query, one id
	// resolution per entity type, and one Qdrant upsert.
	subjectChunk int
	upsertFn     func(ctx context.Context, points *qdrant.UpsertPoints) error
	cleanupFn    func(ctx context.Context, collection string, keep map[uint64]struct{}) (int, error)
}

// NewService creates a new Service with the required dependencies.
func NewService(repo *Repository, idmapSvc *idmap.Service, qdrantClient *qdrant.Client) *Service {
	return &Service{
		repo:         repo,
		idmapSvc:     idmapSvc,
		qdrant:       qdrantClient,
		subjectChunk: qdrantBatchSize,
		upsertFn: func(ctx context.Context, points *qdrant.UpsertPoints) error {
			_, err := qdrantClient.Upsert(ctx, points)
			if err != nil {
				return fmt.Errorf("qdrant upsert: %w", err)
			}
			return nil
		},
		cleanupFn: func(ctx context.Context, collection string, keep map[uint64]struct{}) (int, error) {
			return CleanupStalePoints(ctx, qdrantClient, collection, keep)
		},
	}
}

// RecomputeNamespace runs a full vector recompute for a single namespace.
// It returns the number of subjects and distinct objects processed.
func (s *Service) RecomputeNamespace(ctx context.Context, namespace string, lambda float64) (subjectCount, objectCount int, err error) {
	subjects, err := s.repo.GetActiveSubjects(ctx, namespace)
	if err != nil {
		return 0, 0, fmt.Errorf("get active subjects: %w", err)
	}

	slog.Info("recomputing namespace", "namespace", namespace, "subjects", len(subjects))

	// rows keeps each subject's (object id, score) pairs so the quadratic
	// co-occurrence fold can run in bounded partitions after the loop.
	rows := make([][]coEntry, 0, len(subjects))
	objectKeys := make(map[uint64]string)
	// objectMaxTime[objectID] = max occurred_at (or object_created_at when available) across all subjects
	objectMaxTime := make(map[string]int64)
	// objectCreatedAt[objectID] = object_created_at when explicitly provided by the event source
	objectCreatedAt := make(map[string]int64)

	// Subjects are processed in chunks so each chunk costs a fixed handful of
	// round-trips (events, subject ids, object ids, one upsert) instead of
	// three per subject — the N+1 that made phase 1 84% of batch time.
	// Chunk and per-subject failures are tolerated (one bad subject must not
	// sink the namespace), but a run where nothing was upserted is a failure —
	// phase 1 reporting success while Qdrant holds only stale vectors is worse
	// than an honest red run.
	upserted := 0
	keepSubjects := make(map[uint64]struct{}, len(subjects))
	for start := 0; start < len(subjects); start += s.subjectChunk {
		chunk := subjects[start:min(start+s.subjectChunk, len(subjects))]
		built, err := s.buildChunk(ctx, namespace, chunk, lambda)
		if err != nil {
			slog.Error("build subject chunk failed", "namespace", namespace, "subjects", len(chunk), "first_subject_id", chunk[0], "error", err)
			continue
		}

		vecs := make([]*SubjectVector, 0, len(built))
		for _, b := range built {
			vecs = append(vecs, b.vec)
		}
		if err := s.upsertSubjectVectors(ctx, namespace, vecs); err != nil {
			slog.Error("upsert subject vectors failed", "namespace", namespace, "subjects", len(vecs), "first_subject_id", chunk[0], "error", err)
		} else {
			upserted += len(vecs)
			for _, vec := range vecs {
				keepSubjects[vec.NumericID] = struct{}{}
			}
		}

		for _, b := range built {
			row := make([]coEntry, 0, len(b.scores))
			for objID, score := range b.scores {
				id := b.objectIDs[objID]
				objectKeys[id] = objID
				row = append(row, coEntry{id: id, score: float32(score)})

				if t, ok := b.maxTimes[objID]; ok && t > objectMaxTime[objID] {
					objectMaxTime[objID] = t
				}
				if t, ok := b.createdTimes[objID]; ok && t > 0 {
					if existing, has := objectCreatedAt[objID]; !has || t > existing {
						objectCreatedAt[objID] = t
					}
				}
			}
			rows = append(rows, row)
		}
	}

	if len(subjects) > 0 && upserted == 0 {
		return 0, 0, fmt.Errorf("all %d subject upserts failed", len(subjects))
	}

	// The full object×object matrix grows with Σk² over subjects and OOM-killed
	// cron at bluesky scale, so rows are built one target partition at a time
	// and each partition is upserted before the next is accumulated.
	passes := cooccurrencePasses(rows)
	if passes > 1 {
		slog.Info("partitioning object co-occurrence", "namespace", namespace, "passes", passes)
	}
	keepObjects := make(map[uint64]struct{}, len(objectKeys))
	objects := 0
	for pass := range passes {
		accum := make(map[string]map[uint64]float32)
		for _, row := range rows {
			accumulateObjectCooccurrence(accum, row, objectKeys, passes, pass)
		}
		kept, err := s.upsertObjectVectors(ctx, namespace, accum, objectMaxTime, objectCreatedAt)
		if err != nil {
			return upserted, 0, fmt.Errorf("upsert object vectors: %w", err)
		}
		maps.Copy(keepObjects, kept)
		objects += len(accum)
	}

	// Full recompute only upserts what the window produced — sweep out the
	// points of entities that aged past it, or they keep frozen scores (and
	// keep matching searches) forever. Best-effort: a failed sweep is stale
	// data, not a failed run; the next tick retries it.
	s.cleanupCollection(ctx, namespace, infraqdrant.CollectionSubjects, keepSubjects)
	s.cleanupCollection(ctx, namespace, infraqdrant.CollectionObjects, keepObjects)

	metrics.BatchEntitiesProcessed.WithLabelValues(namespace).Set(float64(upserted))
	slog.Info("namespace recomputed", "namespace", namespace, "subjects", upserted, "objects", objects)
	return upserted, objects, nil
}

func (s *Service) cleanupCollection(ctx context.Context, namespace string, kind infraqdrant.CollectionKind, keep map[uint64]struct{}) {
	if s.cleanupFn == nil {
		return
	}
	collection, err := collectionForContext(ctx, namespace, kind)
	if err != nil {
		slog.Warn("stale point cleanup skipped", "namespace", namespace, "error", err)
		return
	}
	n, err := s.cleanupFn(ctx, collection, keep)
	if err != nil {
		slog.Warn("stale point cleanup failed", "collection", collection, "error", err)
		return
	}
	if n > 0 {
		slog.Info("stale points removed", "collection", collection, "removed", n)
	}
}

// coEntry is one object a subject touched: its numeric id and the subject's
// decay-weighted score for it.
type coEntry struct {
	id    uint64
	score float32
}

// cooccurrenceBudget caps the co-occurrence contributions accumulated in one
// pass. Contributions bound distinct map entries from above, so one pass holds
// at most this many entries (a few hundred MiB) whatever the namespace size.
const cooccurrenceBudget = 10_000_000

// cooccurrencePasses returns how many target partitions keep each pass within
// cooccurrenceBudget.
func cooccurrencePasses(rows [][]coEntry) uint64 {
	var pairs uint64
	for _, row := range rows {
		if k := uint64(len(row)); k > 1 {
			pairs += k * (k - 1)
		}
	}
	return max(1, (pairs+cooccurrenceBudget-1)/cooccurrenceBudget)
}

// accumulateObjectCooccurrence folds one subject's row into the co-occurrence
// rows of the targets that fall in this pass (id % passes == pass). Across all
// passes every target is folded exactly once, so the union equals the
// unpartitioned matrix.
func accumulateObjectCooccurrence(accum map[string]map[uint64]float32, row []coEntry, keys map[uint64]string, passes, pass uint64) {
	for _, target := range row {
		if target.id%passes != pass {
			continue
		}
		key := keys[target.id]
		for _, other := range row {
			if other.id == target.id {
				continue
			}
			if accum[key] == nil {
				accum[key] = make(map[uint64]float32)
			}
			accum[key][other.id] += other.score
		}
	}
}

// subjectVectors is everything one subject's event window produced: the sparse
// vector itself, plus the per-object data the namespace-level accumulators
// fold together afterwards. These five travel together to every caller, so
// they are one value rather than a six-result signature.
type subjectVectors struct {
	vec *SubjectVector
	// scores is the decay-weighted score per object.
	scores map[string]float64
	// maxTimes is the latest occurred_at per object.
	maxTimes map[string]int64
	// createdTimes is the explicit object_created_at per object, when the
	// event source supplied one.
	createdTimes map[string]int64
	// objectIDs is the numeric id per object, resolved once so callers need
	// not resolve it again.
	objectIDs map[string]uint64
}

// buildChunk builds the sparse vectors of a chunk of subjects with one events
// query and one batch id resolution per entity type. A subject that cannot be
// built is logged and left out; a failed round-trip fails the whole chunk.
func (s *Service) buildChunk(ctx context.Context, namespace string, chunk []string, lambda float64) ([]*subjectVectors, error) {
	events, err := s.repo.GetSubjectsEvents(ctx, namespace, chunk)
	if err != nil {
		return nil, fmt.Errorf("get events: %w", err)
	}
	subjectIDs, err := s.idmapSvc.GetOrCreateSubjectIDs(ctx, chunk, namespace)
	if err != nil {
		return nil, fmt.Errorf("get subject ids: %w", err)
	}

	now := time.Now().Unix()
	scored := make([]*subjectVectors, len(chunk))
	objectSet := make(map[string]struct{})
	for i, subjectID := range chunk {
		scored[i] = scoreEvents(events[subjectID], lambda, now)
		for objID := range scored[i].scores {
			objectSet[objID] = struct{}{}
		}
	}

	// One resolution per chunk, shared by the subject vectors and the
	// co-occurrence fold: both need exactly this key set.
	objectIDs, err := s.resolveObjectIDs(ctx, namespace, slices.Collect(maps.Keys(objectSet)))
	if err != nil {
		return nil, err
	}

	built := make([]*subjectVectors, 0, len(chunk))
	for i, subjectID := range chunk {
		subjectNumID, ok := subjectIDs[subjectID]
		if !ok {
			slog.Error("build vectors failed", "namespace", namespace, "subject_id", subjectID, "error", "no numeric id resolved")
			continue
		}
		vec, err := buildSubjectVector(namespace, subjectID, subjectNumID, scored[i].scores, objectIDs)
		if err != nil {
			slog.Error("build vectors failed", "namespace", namespace, "subject_id", subjectID, "error", err)
			continue
		}
		scored[i].vec = vec
		scored[i].objectIDs = objectIDs
		built = append(built, scored[i])
	}
	return built, nil
}

// scoreEvents folds one subject's events into decay-weighted per-object
// scores plus the object timestamps the namespace accumulators need.
func scoreEvents(events []*RawEvent, lambda float64, now int64) *subjectVectors {
	objectScores := make(map[string]float64)
	objectMaxTime := make(map[string]int64)
	objectCreatedTimes := make(map[string]int64)

	for _, e := range events {
		// Clamp: a future-dated event (clock skew that slipped past ingest)
		// must cap at freshness 1.0, not exponentiate into +Inf.
		daysSince := max(float64(now-e.OccurredAt)/86400.0, 0)
		freshness := math.Exp(-lambda * daysSince)
		objectScores[e.ObjectID] += e.Weight * freshness

		if e.OccurredAt > objectMaxTime[e.ObjectID] {
			objectMaxTime[e.ObjectID] = e.OccurredAt
		}
		if e.ObjectCreatedAt != nil && *e.ObjectCreatedAt > objectCreatedTimes[e.ObjectID] {
			objectCreatedTimes[e.ObjectID] = *e.ObjectCreatedAt
		}
	}

	return &subjectVectors{
		scores:       objectScores,
		maxTimes:     objectMaxTime,
		createdTimes: objectCreatedTimes,
	}
}

// resolveObjectIDs maps every object key to its numeric id in one round-trip.
func (s *Service) resolveObjectIDs(ctx context.Context, namespace string, keys []string) (map[string]uint64, error) {
	if len(keys) == 0 {
		return map[string]uint64{}, nil
	}
	objectIDs, err := s.idmapSvc.GetOrCreateObjectIDs(ctx, keys, namespace)
	if err != nil {
		return nil, fmt.Errorf("get object ids: %w", err)
	}
	return objectIDs, nil
}

func buildSubjectVector(namespace, subjectID string, subjectNumID uint64, objectScores map[string]float64, objectIDs map[string]uint64) (*SubjectVector, error) {
	entries := make([]sparseEntry, 0, len(objectScores))
	for objectID, score := range objectScores {
		objNumID, ok := objectIDs[objectID]
		if !ok {
			return nil, fmt.Errorf("no numeric id resolved for object %q", objectID)
		}
		index, fits := sparseIndex(objNumID)
		if !fits {
			slog.Warn("skipping unrepresentable sparse dimension", "namespace", namespace, "subject_id", subjectID, "object_id", objectID, "numeric_id", objNumID)
			metrics.SparseDimensionsSkippedTotal.WithLabelValues(namespace, "subject").Inc()
			continue
		}
		entries = append(entries, sparseEntry{index: index, value: float32(score)})
	}

	// Partial truncation degrades; total truncation must not pass as health.
	// An empty vector is legitimate for a subject with no interactions, but a
	// subject that had scores and kept none would be upserted empty and
	// counted toward upserted++ — sparse search then returns nothing, requests
	// fall to fallback_popular, and the run still reports success.
	if len(objectScores) > 0 && len(entries) == 0 {
		return nil, fmt.Errorf("every dimension exceeds the uint32 sparse index space (%d objects)", len(objectScores))
	}

	indices, values := sparseVectorFrom(entries)

	return &SubjectVector{
		SubjectID: subjectID,
		NumericID: subjectNumID,
		Indices:   indices,
		Values:    values,
	}, nil
}

// upsertSubjectVectors writes a chunk of subject vectors in one request.
func (s *Service) upsertSubjectVectors(ctx context.Context, namespace string, vecs []*SubjectVector) error {
	if len(vecs) == 0 {
		return nil
	}
	collection, err := collectionForContext(ctx, namespace, infraqdrant.CollectionSubjects)
	if err != nil {
		return err
	}
	updatedAt := qdrant.NewValueString(time.Now().UTC().Format(time.RFC3339))
	points := make([]*qdrant.PointStruct, 0, len(vecs))
	for _, vec := range vecs {
		points = append(points, &qdrant.PointStruct{
			Id: qdrant.NewIDNum(vec.NumericID),
			Vectors: &qdrant.Vectors{
				VectorsOptions: &qdrant.Vectors_Vectors{
					Vectors: &qdrant.NamedVectors{
						Vectors: map[string]*qdrant.Vector{
							sparseVectorName: qdrant.NewVectorSparse(vec.Indices, vec.Values),
						},
					},
				},
			},
			Payload: map[string]*qdrant.Value{
				"subject_id": qdrant.NewValueString(vec.SubjectID),
				"updated_at": updatedAt,
			},
		})
	}
	if err := s.upsertFn(ctx, &qdrant.UpsertPoints{CollectionName: collection, Points: points}); err != nil {
		return fmt.Errorf("upsert subject vectors: %w", err)
	}
	return nil
}

func (s *Service) upsertObjectVectors(ctx context.Context, namespace string, accum map[string]map[uint64]float32, maxTimes, createdTimes map[string]int64) (map[uint64]struct{}, error) {
	collectionName, err := collectionForContext(ctx, namespace, infraqdrant.CollectionObjects)
	if err != nil {
		return nil, err
	}
	upsertedIDs := make(map[uint64]struct{}, len(accum))
	var batch []*qdrant.PointStruct

	// One resolution for the whole accumulated set: these keys were already
	// minted while building the subject vectors, so this is a bulk read.
	objectKeys := make([]string, 0, len(accum))
	for objectID := range accum {
		objectKeys = append(objectKeys, objectID)
	}
	objectIDs := make(map[string]uint64, len(accum))
	if len(objectKeys) > 0 {
		resolved, err := s.idmapSvc.GetOrCreateObjectIDs(ctx, objectKeys, namespace)
		if err != nil {
			return upsertedIDs, fmt.Errorf("get object ids: %w", err)
		}
		objectIDs = resolved
	}

	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		err := s.upsertFn(ctx, &qdrant.UpsertPoints{
			CollectionName: collectionName,
			Points:         batch,
		})
		batch = batch[:0]
		if err != nil {
			return fmt.Errorf("flush object batch to qdrant: %w", err)
		}
		return nil
	}

	for objectID, subjectScores := range accum {
		objNumID, ok := objectIDs[objectID]
		if !ok {
			return upsertedIDs, fmt.Errorf("no numeric id resolved for object %q", objectID)
		}
		upsertedIDs[objNumID] = struct{}{}

		entries := make([]sparseEntry, 0, len(subjectScores))
		for coNumID, score := range subjectScores {
			index, fits := sparseIndex(coNumID)
			if !fits {
				slog.Warn("skipping unrepresentable sparse dimension", "namespace", namespace, "object_id", objectID, "numeric_id", coNumID)
				metrics.SparseDimensionsSkippedTotal.WithLabelValues(namespace, "object").Inc()
				continue
			}
			entries = append(entries, sparseEntry{index: index, value: score})
		}
		indices, values := sparseVectorFrom(entries)

		// Prefer explicit object_created_at from the event payload; fall back to max occurred_at.
		createdAt := time.Now().UTC().Format(time.RFC3339)
		if t, ok := createdTimes[objectID]; ok && t > 0 {
			createdAt = time.Unix(t, 0).UTC().Format(time.RFC3339)
		} else if t, ok := maxTimes[objectID]; ok && t > 0 {
			createdAt = time.Unix(t, 0).UTC().Format(time.RFC3339)
		}

		batch = append(batch, &qdrant.PointStruct{
			Id: qdrant.NewIDNum(objNumID),
			Vectors: &qdrant.Vectors{
				VectorsOptions: &qdrant.Vectors_Vectors{
					Vectors: &qdrant.NamedVectors{
						Vectors: map[string]*qdrant.Vector{
							sparseVectorName: qdrant.NewVectorSparse(indices, values),
						},
					},
				},
			},
			Payload: map[string]*qdrant.Value{
				"object_id":  qdrant.NewValueString(objectID),
				"created_at": qdrant.NewValueString(createdAt),
			},
		})

		if len(batch) >= qdrantBatchSize {
			if err := flush(); err != nil {
				return upsertedIDs, err
			}
		}
	}
	return upsertedIDs, flush()
}
