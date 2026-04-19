package idmap

import (
	"context"
	"fmt"

	"github.com/jarviisha/codohue/internal/infra/metrics"
)

// Service provides methods to get or create numeric IDs for subjects and objects.
type Service struct {
	repo interface {
		GetOrCreate(ctx context.Context, stringID, namespace, entityType string) (uint64, error)
	}
}

// NewService creates a new Service with the given repository.
func NewService(repo interface {
	GetOrCreate(ctx context.Context, stringID, namespace, entityType string) (uint64, error)
}) *Service {
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
