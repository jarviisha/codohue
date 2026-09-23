package compute

import (
	"context"
	"fmt"
	"log/slog"
	"math"
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
	GetSubjectEvents(ctx context.Context, namespace, subjectID string) ([]*RawEvent, error)
}

type idmapService interface {
	GetOrCreateSubjectID(ctx context.Context, subjectID, namespace string) (uint64, error)
	GetOrCreateObjectID(ctx context.Context, objectID, namespace string) (uint64, error)
	GetOrCreateObjectIDs(ctx context.Context, objectIDs []string, namespace string) (map[string]uint64, error)
}

// Service computes sparse vectors with time decay for each subject in a namespace.
type Service struct {
	repo      computeRepo
	idmapSvc  idmapService
	qdrant    *qdrant.Client
	upsertFn  func(ctx context.Context, points *qdrant.UpsertPoints) error
	cleanupFn func(ctx context.Context, collection string, keep map[uint64]struct{}) (int, error)
}

// NewService creates a new Service with the required dependencies.
func NewService(repo *Repository, idmapSvc *idmap.Service, qdrantClient *qdrant.Client) *Service {
	return &Service{
		repo:     repo,
		idmapSvc: idmapSvc,
		qdrant:   qdrantClient,
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

	// outer key: objectID, inner key: subjectNumericID, value: decay-weighted score
	objectAccum := make(map[string]map[uint64]float32)
	// objectMaxTime[objectID] = max occurred_at (or object_created_at when available) across all subjects
	objectMaxTime := make(map[string]int64)
	// objectCreatedAt[objectID] = object_created_at when explicitly provided by the event source
	objectCreatedAt := make(map[string]int64)

	// Per-subject failures are tolerated (one bad subject must not sink the
	// namespace), but a run where nothing was upserted is a failure — phase 1
	// reporting success while Qdrant holds only stale vectors is worse than
	// an honest red run.
	upserted := 0
	keepSubjects := make(map[uint64]struct{}, len(subjects))
	for _, subjectID := range subjects {
		built, err := s.buildVectors(ctx, namespace, subjectID, lambda)
		if err != nil {
			slog.Error("build vectors failed", "namespace", namespace, "subject_id", subjectID, "error", err)
			continue
		}

		if err := s.upsertSubjectVector(ctx, namespace, built.vec); err != nil {
			slog.Error("upsert subject vector failed", "namespace", namespace, "subject_id", subjectID, "error", err)
		} else {
			upserted++
			keepSubjects[built.vec.NumericID] = struct{}{}
		}

		s.accumulateObjectCooccurrence(objectAccum, built.scores, built.objectIDs)

		for objID := range built.scores {
			if t, ok := built.maxTimes[objID]; ok && t > objectMaxTime[objID] {
				objectMaxTime[objID] = t
			}
			if t, ok := built.createdTimes[objID]; ok && t > 0 {
				if existing, has := objectCreatedAt[objID]; !has || t > existing {
					objectCreatedAt[objID] = t
				}
			}
		}
	}

	if len(subjects) > 0 && upserted == 0 {
		return 0, 0, fmt.Errorf("all %d subject upserts failed", len(subjects))
	}

	keepObjects, err := s.upsertObjectVectors(ctx, namespace, objectAccum, objectMaxTime, objectCreatedAt)
	if err != nil {
		return upserted, 0, fmt.Errorf("upsert object vectors: %w", err)
	}

	// Full recompute only upserts what the window produced — sweep out the
	// points of entities that aged past it, or they keep frozen scores (and
	// keep matching searches) forever. Best-effort: a failed sweep is stale
	// data, not a failed run; the next tick retries it.
	s.cleanupCollection(ctx, collectionForContext(ctx, namespace, infraqdrant.CollectionSubjects), keepSubjects)
	s.cleanupCollection(ctx, collectionForContext(ctx, namespace, infraqdrant.CollectionObjects), keepObjects)

	metrics.BatchEntitiesProcessed.WithLabelValues(namespace).Set(float64(upserted))
	slog.Info("namespace recomputed", "namespace", namespace, "subjects", upserted, "objects", len(objectAccum))
	return upserted, len(objectAccum), nil
}

func (s *Service) cleanupCollection(ctx context.Context, collection string, keep map[uint64]struct{}) {
	if s.cleanupFn == nil {
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

// accumulateObjectCooccurrence folds one subject's scores into the namespace's
// object co-occurrence rows. objectIDs is the resolved numeric id per object,
// passed in rather than looked up here: buildVectors already resolved exactly
// this key set for the subject vector, and resolving it twice per subject was
// half of the 2N sequential queries each tick spent on id mapping.
func (s *Service) accumulateObjectCooccurrence(objectAccum map[string]map[uint64]float32, objectScores map[string]float64, objectIDs map[string]uint64) {
	for targetID := range objectScores {
		for otherID, score := range objectScores {
			if otherID == targetID {
				continue
			}
			if objectAccum[targetID] == nil {
				objectAccum[targetID] = make(map[uint64]float32)
			}
			objectAccum[targetID][objectIDs[otherID]] += float32(score)
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

// buildVectors computes one subject's sparse vector and the per-object data
// derived alongside it.
func (s *Service) buildVectors(ctx context.Context, namespace, subjectID string, lambda float64) (*subjectVectors, error) {
	events, err := s.repo.GetSubjectEvents(ctx, namespace, subjectID)
	if err != nil {
		return nil, fmt.Errorf("get events: %w", err)
	}

	objectScores := make(map[string]float64)
	objectMaxTime := make(map[string]int64)
	objectCreatedTimes := make(map[string]int64)
	now := time.Now().Unix()

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

	// One resolution per subject, shared by the subject vector and the
	// co-occurrence fold: both need exactly this key set.
	objectIDs, err := s.resolveObjectIDs(ctx, namespace, objectScores)
	if err != nil {
		return nil, err
	}

	vec, err := s.buildSubjectVector(ctx, namespace, subjectID, objectScores, objectIDs)
	if err != nil {
		return nil, fmt.Errorf("build subject vector: %w", err)
	}

	return &subjectVectors{
		vec:          vec,
		scores:       objectScores,
		maxTimes:     objectMaxTime,
		createdTimes: objectCreatedTimes,
		objectIDs:    objectIDs,
	}, nil
}

// resolveObjectIDs maps every object key to its numeric id in one round-trip.
func (s *Service) resolveObjectIDs(ctx context.Context, namespace string, objectScores map[string]float64) (map[string]uint64, error) {
	if len(objectScores) == 0 {
		return map[string]uint64{}, nil
	}
	keys := make([]string, 0, len(objectScores))
	for objectID := range objectScores {
		keys = append(keys, objectID)
	}
	objectIDs, err := s.idmapSvc.GetOrCreateObjectIDs(ctx, keys, namespace)
	if err != nil {
		return nil, fmt.Errorf("get object ids: %w", err)
	}
	return objectIDs, nil
}

func (s *Service) buildSubjectVector(ctx context.Context, namespace, subjectID string, objectScores map[string]float64, objectIDs map[string]uint64) (*SubjectVector, error) {
	subjectNumID, err := s.idmapSvc.GetOrCreateSubjectID(ctx, subjectID, namespace)
	if err != nil {
		return nil, fmt.Errorf("get subject id for %q: %w", subjectID, err)
	}

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

func (s *Service) upsertSubjectVector(ctx context.Context, namespace string, vec *SubjectVector) error {
	err := s.upsertFn(ctx, &qdrant.UpsertPoints{
		CollectionName: collectionForContext(ctx, namespace, infraqdrant.CollectionSubjects),
		Points: []*qdrant.PointStruct{
			{
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
					"updated_at": qdrant.NewValueString(time.Now().UTC().Format(time.RFC3339)),
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("upsert subject vector: %w", err)
	}
	return nil
}

func (s *Service) upsertObjectVectors(ctx context.Context, namespace string, accum map[string]map[uint64]float32, maxTimes, createdTimes map[string]int64) (map[uint64]struct{}, error) {
	collectionName := collectionForContext(ctx, namespace, infraqdrant.CollectionObjects)
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
