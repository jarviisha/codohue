package ingest

import (
	"context"
	"errors"
	"fmt"

	"github.com/jarviisha/codohue/internal/core/namespace"
	"github.com/jarviisha/codohue/internal/infra/metrics"
)

var (
	// ErrInvalidPayload indicates that the inbound event payload is missing required fields or is otherwise malformed.
	ErrInvalidPayload = errors.New("invalid payload")
	// ErrUnknownAction indicates that the event action cannot be resolved to a configured or default weight.
	ErrUnknownAction = errors.New("unknown action")
)

type eventInserter interface {
	Insert(ctx context.Context, e *Event) error
}

type nsConfigGetter interface {
	Get(ctx context.Context, namespace string) (*namespace.Config, error)
}

// Service processes and persists behavioral events received from Redis Streams.
type Service struct {
	repo          eventInserter
	nsConfigSvc   nsConfigGetter
	tailPublisher EventTailPublisher
}

// NewService creates a new Service with the given repository and namespace config service.
func NewService(repo *Repository, nsConfigSvc nsConfigGetter) *Service {
	return &Service{repo: repo, nsConfigSvc: nsConfigSvc}
}

// SetTailPublisher wires the live-tail publisher. Optional: a nil publisher
// (the default) disables the tail fan-out without affecting ingest.
func (s *Service) SetTailPublisher(p EventTailPublisher) {
	s.tailPublisher = p
}

// Process validates the payload, resolves the action weight, and stores the
// event. On success it returns the generated event id, increments the ingest
// counter, and publishes the event to the live tail. Failures increment
// codohue_ingest_errors_total with a reason label.
func (s *Service) Process(ctx context.Context, payload *EventPayload) (int64, error) {
	if payload.Namespace == "" || payload.SubjectID == "" || payload.ObjectID == "" {
		metrics.IngestErrorsTotal.WithLabelValues(payload.Namespace, "invalid_payload").Inc()
		return 0, fmt.Errorf("%w: namespace, subject_id, object_id are required", ErrInvalidPayload)
	}

	weight, err := s.resolveWeight(ctx, payload.Namespace, payload.Action)
	if err != nil {
		metrics.IngestErrorsTotal.WithLabelValues(payload.Namespace, "unknown_action").Inc()
		return 0, fmt.Errorf("resolve weight: %w", err)
	}

	event := &Event{
		Namespace:       payload.Namespace,
		SubjectID:       payload.SubjectID,
		ObjectID:        payload.ObjectID,
		Action:          payload.Action,
		Weight:          weight,
		OccurredAt:      payload.OccurredAt,
		ObjectCreatedAt: payload.ObjectCreatedAt,
	}

	if err := s.repo.Insert(ctx, event); err != nil {
		metrics.IngestErrorsTotal.WithLabelValues(payload.Namespace, "insert").Inc()
		return 0, fmt.Errorf("insert event: %w", err)
	}

	metrics.EventsIngestedTotal.WithLabelValues(event.Namespace, string(event.Action)).Inc()
	if s.tailPublisher != nil {
		s.tailPublisher.Publish(EventTailMessage{
			ID:         event.ID,
			Namespace:  event.Namespace,
			SubjectID:  event.SubjectID,
			ObjectID:   event.ObjectID,
			Action:     string(event.Action),
			Weight:     event.Weight,
			OccurredAt: event.OccurredAt,
		})
	}
	return event.ID, nil
}

func (s *Service) resolveWeight(ctx context.Context, ns string, action Action) (float64, error) {
	cfg, err := s.nsConfigSvc.Get(ctx, ns)
	if err == nil && cfg != nil {
		if w, ok := cfg.ActionWeights[string(action)]; ok {
			return w, nil
		}
	}

	if w, ok := DefaultActionWeights[action]; ok {
		return w, nil
	}
	return 0, fmt.Errorf("%w: %s", ErrUnknownAction, action)
}
