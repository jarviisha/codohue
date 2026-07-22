package idmap

import (
	"context"
	"fmt"

	"github.com/jarviisha/codohue/internal/infra/metrics"
)

// idmapRepo is the repository surface the Service needs; *Repository
// implements it, tests fake it.
type idmapRepo interface {
	GetOrCreate(ctx context.Context, stringID, namespace, entityType string) (uint64, error)
	Lookup(ctx context.Context, stringID, namespace, entityType string) (uint64, bool, error)
	GetOrCreateBatch(ctx context.Context, stringIDs []string, namespace, entityType string) (map[string]uint64, error)
}

// Service provides methods to get or create numeric IDs for subjects and objects.
type Service struct {
	repo idmapRepo
}

// NewService creates a new Service with the given repository.
func NewService(repo idmapRepo) *Service {
	return &Service{repo: repo}
}

// GetOrCreateSubjectID returns the numeric ID for the given subjectID, creating it if absent.
func (s *Service) GetOrCreateSubjectID(ctx context.Context, subjectID, namespace string) (uint64, error) {
	id, err := s.repo.GetOrCreate(ctx, subjectID, namespace, "subject")
	if err != nil {
		metrics.IDMappingErrors.WithLabelValues("subject").Inc()
		return id, fmt.Errorf("get or create subject id: %w", err)
	}
	return id, nil
}

// GetOrCreateObjectID returns the numeric ID for the given objectID, creating it if absent.
func (s *Service) GetOrCreateObjectID(ctx context.Context, objectID, namespace string) (uint64, error) {
	id, err := s.repo.GetOrCreate(ctx, objectID, namespace, "object")
	if err != nil {
		metrics.IDMappingErrors.WithLabelValues("object").Inc()
		return id, fmt.Errorf("get or create object id: %w", err)
	}
	return id, nil
}

// LookupObjectID returns the numeric id for objectID without creating one.
func (s *Service) LookupObjectID(ctx context.Context, objectID, namespace string) (numericID uint64, found bool, err error) {
	id, found, err := s.repo.Lookup(ctx, objectID, namespace, "object")
	if err != nil {
		metrics.IDMappingErrors.WithLabelValues("object").Inc()
		return 0, false, fmt.Errorf("lookup object id: %w", err)
	}
	return id, found, nil
}

// GetOrCreateObjectIDs resolves many object ids in a single round-trip.
func (s *Service) GetOrCreateObjectIDs(ctx context.Context, objectIDs []string, namespace string) (map[string]uint64, error) {
	ids, err := s.repo.GetOrCreateBatch(ctx, objectIDs, namespace, "object")
	if err != nil {
		metrics.IDMappingErrors.WithLabelValues("object").Inc()
		return nil, fmt.Errorf("batch get or create object ids: %w", err)
	}
	return ids, nil
}
