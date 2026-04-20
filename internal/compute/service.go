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
	"github.com/qdrant/go-client/qdrant"
)

const (
	defaultLambda    = 0.05 // time decay per day
	sparseVectorName = "sparse_interactions"
	qdrantBatchSize  = 100
)

type computeRepo interface {
	GetActiveSubjects(ctx context.Context, namespace string) ([]string, error)
	GetSubjectEvents(ctx context.Context, namespace, subjectID string) ([]*RawEvent, error)
}

type idmapService interface {
	GetOrCreateSubjectID(ctx context.Context, subjectID, namespace string) (uint64, error)
	GetOrCreateObjectID(ctx context.Context, objectID, namespace string) (uint64, error)
}

// Service computes sparse vectors with time decay for each subject in a namespace.
type Service struct {
	repo     computeRepo
	idmapSvc idmapService
	qdrant   *qdrant.Client
	upsertFn func(ctx context.Context, points *qdrant.UpsertPoints) error
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
	}
}

// RecomputeNamespace runs a full vector recompute for a single namespace.
func (s *Service) RecomputeNamespace(ctx context.Context, namespace string, lambda float64) error {
	subjects, err := s.repo.GetActiveSubjects(ctx, namespace)
	if err != nil {
		return fmt.Errorf("get active subjects: %w", err)
	}

	slog.Info("recomputing namespace", "namespace", namespace, "subjects", len(subjects))

	// outer key: objectID, inner key: subjectNumericID, value: decay-weighted score
	objectAccum := make(map[string]map[uint64]float32)
	// objectMaxTime[objectID] = max occurred_at (or object_created_at when available) across all subjects
	objectMaxTime := make(map[string]int64)
	// objectCreatedAt[objectID] = object_created_at when explicitly provided by the event source
	objectCreatedAt := make(map[string]int64)

	for _, subjectID := range subjects {
		subjectVec, scores, maxTimes, createdTimes, err := s.buildVectors(ctx, namespace, subjectID, lambda)
		if err != nil {
			slog.Error("build vectors failed", "namespace", namespace, "subject_id", subjectID, "error", err)
			continue
		}

		if err := s.upsertSubjectVector(ctx, namespace, subjectVec); err != nil {
			slog.Error("upsert subject vector failed", "namespace", namespace, "subject_id", subjectID, "error", err)
		}

		if err := s.accumulateObjectCooccurrence(ctx, namespace, objectAccum, scores); err != nil {
			slog.Error("accumulate object cooccurrence failed", "namespace", namespace, "subject_id", subjectID, "error", err)
		}

		for objID := range scores {
			if t, ok := maxTimes[objID]; ok && t > objectMaxTime[objID] {
				objectMaxTime[objID] = t
			}
			if t, ok := createdTimes[objID]; ok && t > 0 {
				if existing, has := objectCreatedAt[objID]; !has || t > existing {
					objectCreatedAt[objID] = t
				}
			}
		}
	}

	if err := s.upsertObjectVectors(ctx, namespace, objectAccum, objectMaxTime, objectCreatedAt); err != nil {
		slog.Error("upsert object vectors failed", "namespace", namespace, "error", err)
	}

	metrics.BatchSubjectsProcessed.WithLabelValues(namespace).Set(float64(len(subjects)))
	slog.Info("namespace recomputed", "namespace", namespace, "subjects", len(subjects), "objects", len(objectAccum))
	return nil
}

func (s *Service) accumulateObjectCooccurrence(ctx context.Context, namespace string, objectAccum map[string]map[uint64]float32, objectScores map[string]float64) error {
	objectIDs := make(map[string]uint64, len(objectScores))
	for objectID := range objectScores {
		objNumID, err := s.idmapSvc.GetOrCreateObjectID(ctx, objectID, namespace)
		if err != nil {
			return fmt.Errorf("get object id for %q: %w", objectID, err)
		}
		objectIDs[objectID] = objNumID
	}

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

	return nil
}

// buildVectors computes the sparse vector for a subject and returns the decay-weighted scores,
// the max occurred_at timestamp per object, and the explicit object_created_at per object when available.
func (s *Service) buildVectors(ctx context.Context, namespace, subjectID string, lambda float64) (vec *SubjectVector, scores map[string]float64, maxTimes, createdTimes map[string]int64, err error) {
	events, err := s.repo.GetSubjectEvents(ctx, namespace, subjectID)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("get events: %w", err)
	}

	objectScores := make(map[string]float64)
	objectMaxTime := make(map[string]int64)
	objectCreatedTimes := make(map[string]int64)
	now := time.Now().Unix()

	for _, e := range events {
		daysSince := float64(now-e.OccurredAt) / 86400.0
		freshness := math.Exp(-lambda * daysSince)
		objectScores[e.ObjectID] += e.Weight * freshness

		if e.OccurredAt > objectMaxTime[e.ObjectID] {
			objectMaxTime[e.ObjectID] = e.OccurredAt
		}
		if e.ObjectCreatedAt != nil && *e.ObjectCreatedAt > objectCreatedTimes[e.ObjectID] {
			objectCreatedTimes[e.ObjectID] = *e.ObjectCreatedAt
		}
	}

	subjectVec, err := s.buildSubjectVector(ctx, namespace, subjectID, objectScores)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("build subject vector: %w", err)
	}

	return subjectVec, objectScores, objectMaxTime, objectCreatedTimes, nil
}

func (s *Service) buildSubjectVector(ctx context.Context, namespace, subjectID string, objectScores map[string]float64) (*SubjectVector, error) {
	subjectNumID, err := s.idmapSvc.GetOrCreateSubjectID(ctx, subjectID, namespace)
	if err != nil {
		return nil, fmt.Errorf("get subject id for %q: %w", subjectID, err)
	}

	type sparseEntry struct {
		index uint32
		value float32
	}
	entries := make([]sparseEntry, 0, len(objectScores))
	for objectID, score := range objectScores {
		objNumID, err := s.idmapSvc.GetOrCreateObjectID(ctx, objectID, namespace)
		if err != nil {
			return nil, fmt.Errorf("get object id for %q: %w", objectID, err)
		}
		entries = append(entries, sparseEntry{index: uint32(objNumID), value: float32(score)})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].index < entries[j].index
	})

	indices := make([]uint32, 0, len(entries))
	values := make([]float32, 0, len(entries))
	for _, entry := range entries {
		indices = append(indices, entry.index)
		values = append(values, entry.value)
	}

	return &SubjectVector{
		SubjectID: subjectID,
		NumericID: subjectNumID,
		Indices:   indices,
		Values:    values,
	}, nil
}

func (s *Service) upsertSubjectVector(ctx context.Context, namespace string, vec *SubjectVector) error {
	err := s.upsertFn(ctx, &qdrant.UpsertPoints{
		CollectionName: namespace + "_subjects",
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

func (s *Service) upsertObjectVectors(ctx context.Context, namespace string, accum map[string]map[uint64]float32, maxTimes, createdTimes map[string]int64) error {
	collectionName := namespace + "_objects"
	var batch []*qdrant.PointStruct

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
		objNumID, err := s.idmapSvc.GetOrCreateObjectID(ctx, objectID, namespace)
		if err != nil {
			return fmt.Errorf("get object id %q: %w", objectID, err)
		}

		type sparseEntry struct {
			index uint32
			value float32
		}
		entries := make([]sparseEntry, 0, len(subjectScores))
		for subjNumID, score := range subjectScores {
			entries = append(entries, sparseEntry{index: uint32(subjNumID), value: score})
		}
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].index < entries[j].index
		})

		indices := make([]uint32, 0, len(entries))
		values := make([]float32, 0, len(entries))
		for _, entry := range entries {
			indices = append(indices, entry.index)
			values = append(values, entry.value)
		}

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
				return err
			}
		}
	}
	return flush()
}
