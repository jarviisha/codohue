package ingest

import (
	"context"
	"fmt"

	"github.com/jarviisha/codohue/internal/nsconfig"
)

type eventInserter interface {
	Insert(ctx context.Context, e *Event) error
}

type nsConfigGetter interface {
	Get(ctx context.Context, namespace string) (*nsconfig.NamespaceConfig, error)
}

// Service processes and persists behavioral events received from Redis Streams.
type Service struct {
	repo        eventInserter
	nsConfigSvc nsConfigGetter
}

// NewService creates a new Service with the given repository and namespace config service.
func NewService(repo *Repository, nsConfigSvc *nsconfig.Service) *Service {
	return &Service{repo: repo, nsConfigSvc: nsConfigSvc}
}

// Process validates the payload, resolves the action weight, and stores the event.
func (s *Service) Process(ctx context.Context, payload *EventPayload) error {
	if payload.Namespace == "" || payload.SubjectID == "" || payload.ObjectID == "" {
		return fmt.Errorf("invalid payload: namespace, subject_id, object_id are required")
	}

	weight, err := s.resolveWeight(ctx, payload.Namespace, payload.Action)
	if err != nil {
		return fmt.Errorf("resolve weight: %w", err)
	}

	event := &Event{
		Namespace:       payload.Namespace,
		SubjectID:       payload.SubjectID,
		ObjectID:        payload.ObjectID,
		Action:          payload.Action,
		Weight:          weight,
		OccurredAt:      payload.Timestamp,
		ObjectCreatedAt: payload.ObjectCreatedAt,
	}

	if err := s.repo.Insert(ctx, event); err != nil {
		return fmt.Errorf("insert event: %w", err)
	}
	return nil
}

func (s *Service) resolveWeight(ctx context.Context, namespace string, action Action) (float64, error) {
	cfg, err := s.nsConfigSvc.Get(ctx, namespace)
	if err == nil && cfg != nil {
		if w, ok := cfg.ActionWeights[string(action)]; ok {
			return w, nil
		}
	}

	if w, ok := DefaultActionWeights[action]; ok {
		return w, nil
	}
	return 0, fmt.Errorf("unknown action: %s", action)
}
