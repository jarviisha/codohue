package objects

import (
	"context"
	"fmt"
	"strings"
)

type objectsRepository interface {
	Upsert(ctx context.Context, namespace, objectID, authorSubjectID string) (*Object, error)
	Get(ctx context.Context, namespace, objectID string) (*Object, error)
	Delete(ctx context.Context, namespace, objectID string) error
}

// Service owns the per-object metadata rules.
type Service struct {
	repo objectsRepository
}

// NewService creates a new Service.
func NewService(repo objectsRepository) *Service {
	return &Service{repo: repo}
}

// Upsert stores metadata for an object. Unlike catalog ingest this path is
// open to every namespace regardless of dense_source — attribution is not an
// embedding concern, which is the whole reason the objects table exists.
func (s *Service) Upsert(ctx context.Context, namespace, objectID string, req *UpsertRequest) (*Object, error) {
	if namespace == "" || objectID == "" {
		return nil, fmt.Errorf("%w: namespace and object id are required", ErrInvalidRequest)
	}
	if req == nil {
		return nil, fmt.Errorf("%w: request body is required", ErrInvalidRequest)
	}

	obj, err := s.repo.Upsert(ctx, namespace, objectID, strings.TrimSpace(req.AuthorSubjectID))
	if err != nil {
		return nil, fmt.Errorf("persist object metadata: %w", err)
	}
	return obj, nil
}

// SetAuthor is the write-through used by the catalog ingest path, which
// accepts author_subject_id on its own request body. Wired through an
// interface in cmd/api so internal/catalog never imports this package.
func (s *Service) SetAuthor(ctx context.Context, namespace, objectID, authorSubjectID string) error {
	author := strings.TrimSpace(authorSubjectID)
	if author == "" {
		// Catalog ingest omitting the author must not wipe an attribution set
		// through the objects endpoint — absence here means "unspecified",
		// not "clear it".
		return nil
	}
	if _, err := s.repo.Upsert(ctx, namespace, objectID, author); err != nil {
		return fmt.Errorf("set object author: %w", err)
	}
	return nil
}

// Get returns metadata for one object, or nil when it has none.
func (s *Service) Get(ctx context.Context, namespace, objectID string) (*Object, error) {
	return s.repo.Get(ctx, namespace, objectID)
}

// Delete removes an object's metadata.
func (s *Service) Delete(ctx context.Context, namespace, objectID string) error {
	return s.repo.Delete(ctx, namespace, objectID)
}
