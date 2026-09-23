package nsconfig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jarviisha/codohue/internal/core/namespace"
	"github.com/jarviisha/codohue/internal/core/nslifecycle"
)

type configurationRepository interface {
	ReadConfiguration(context.Context, string) (*namespace.Configuration, error)
	ChangeConfiguration(context.Context, string, *namespace.ConfigurationPatch, bool, func(*namespace.Configuration, map[string]json.RawMessage) error) (*namespace.Configuration, error)
}

// fieldAt returns the offset at which message names exactly this field, or -1.
// Whole-identifier matching stops "lambda" claiming a "lambda_trending" error;
// callers take the earliest match so an echoed value ("got \"dense_source\"")
// never outranks the field the message actually complains about.
func fieldAt(message, field string) int {
	if field == "" {
		return -1
	}
	identifier := func(b byte) bool {
		return b == '_' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
	}
	for at := 0; at <= len(message)-len(field); {
		i := strings.Index(message[at:], field)
		if i < 0 {
			return -1
		}
		i += at
		end := i + len(field)
		if (i == 0 || !identifier(message[i-1])) && (end == len(message) || !identifier(message[end])) {
			return i
		}
		at = i + 1
	}
	return -1
}

func configurationInvalid(group, field, message string) error {
	return &namespace.ConfigurationError{Status: 422, Code: "invalid_configuration", Message: message, Fields: map[string]string{group + "." + field: message}}
}

// ReadConfiguration returns stored values and authoritative collection locks.
func (s *Service) ReadConfiguration(ctx context.Context, ns string) (*namespace.Configuration, error) {
	repo, ok := s.repo.(configurationRepository)
	if !ok {
		return nil, fmt.Errorf("configuration repository unavailable")
	}
	out, err := repo.ReadConfiguration(ctx, ns)
	if err != nil {
		return nil, err
	}
	if err := s.configurationLocks(ctx, ns, out); err != nil {
		return nil, err
	}

	return out, nil
}

func (s *Service) configurationLocks(ctx context.Context, ns string, out *namespace.Configuration) error {
	if s.denseCollections != nil {
		exists, err := s.denseCollections.DenseCollectionsExist(ctx, ns, out.Generation)
		if err != nil {
			return err
		}
		if exists {
			g := out.Groups["embeddings"]
			for _, f := range []string{"embedding_dim", "dense_distance"} {
				g.Locks[f] = "Dense collections already exist. Changing their shape requires a separate migration."
			}
			out.Groups["embeddings"] = g
		}
	}
	return nil
}

// PatchConfiguration preserves lifecycle fencing without implicitly activating a namespace.
func (s *Service) PatchConfiguration(ctx context.Context, ns string, req *namespace.ConfigurationPatch, validateOnly bool) (*namespace.Configuration, error) {
	if err := validateConfigurationPatch(req); err != nil {
		return nil, err
	}
	repo, ok := s.repo.(configurationRepository)
	if !ok {
		return nil, fmt.Errorf("configuration repository unavailable")
	}
	var result *namespace.Configuration
	change := func(leased context.Context, _ *nslifecycle.NamespaceLifecycle) error {
		var err error
		result, err = repo.ChangeConfiguration(leased, ns, req, validateOnly, func(current *namespace.Configuration, values map[string]json.RawMessage) error {
			if err := s.configurationLocks(leased, ns, current); err != nil {
				return err
			}
			return s.validateConfiguration(leased, ns, req, current, values)
		})
		return err
	}
	// Return a predictable 404 before attempting to lease a deleted namespace.
	if _, err := repo.ReadConfiguration(ctx, ns); err != nil {
		return nil, err
	}
	var err error
	if s.lifecycle != nil && nslifecycle.RequireNamespaceLease(ctx, ns) != nil {
		err = s.lifecycle.WithWriter(ctx, ns, change)
	} else {
		err = change(ctx, nil)
	}
	// A conflict snapshot is read before validation runs, so it carries no locks.
	// Clients adopt it as canonical, and an unlocked field would invite a save the
	// server then rejects.
	var conflict *namespace.ConfigurationError
	if errors.As(err, &conflict) && conflict.Current != nil {
		if lockErr := s.configurationLocks(ctx, ns, conflict.Current); lockErr != nil {
			return nil, lockErr
		}
	}
	if errors.Is(err, nslifecycle.ErrNamespaceNotFound) || errors.Is(err, nslifecycle.ErrNamespaceNotActive) {
		return nil, &namespace.ConfigurationError{Status: 404, Code: "not_found", Message: "Namespace is not active or does not exist"}
	}
	if errors.Is(err, nslifecycle.ErrSystemResetting) {
		return nil, &namespace.ConfigurationError{Status: 409, Code: "system_resetting", Message: "System reset is in progress; retry after it completes"}
	}
	return result, err
}

func validateConfigurationPatch(req *namespace.ConfigurationPatch) error {
	if req == nil {
		return configurationInvalid("configuration", "request", "Request is required")
	}
	fields, ok := configurationFields[req.Group]
	if !ok {
		return configurationInvalid(req.Group, "group", "Unknown configuration group")
	}
	if req.Generation < 1 || req.BaseRevision < 1 {
		return configurationInvalid(req.Group, "base_revision", "Positive generation and base_revision are required")
	}
	for f, v := range req.Changes {
		allowed := false
		for _, candidate := range fields {
			if candidate == f {
				allowed = true
			}
		}
		if !allowed {
			return configurationInvalid(req.Group, f, "Field does not belong to this group")
		}
		if strings.TrimSpace(string(v)) == "null" && f != "catalog_max_attempts" && f != "catalog_max_content_bytes" {
			return configurationInvalid(req.Group, f, "This field cannot inherit a system default")
		}
		// Decode each field independently so type errors identify its exact path.
		var target any
		switch f {
		case "exclude_authored":
			target = new(bool)
		case "dense_source", "dense_distance", "catalog_strategy_id", "catalog_strategy_version":
			target = new(string)
		case "action_weights":
			target = new(map[string]float64)
		case "catalog_strategy_params":
			target = new(map[string]any)
		case "alpha", "gamma", "lambda", "lambda_trending":
			target = new(float64)
		default:
			target = new(int32)
		}
		if err := json.Unmarshal(v, target); err != nil {
			return configurationInvalid(req.Group, f, "Invalid value type")
		}
	}
	return nil
}

func (s *Service) validateConfiguration(ctx context.Context, ns string, patch *namespace.ConfigurationPatch, current *namespace.Configuration, values map[string]json.RawMessage) error {
	raw, _ := json.Marshal(values) //nolint:errcheck // values came from JSON and re-marshal cleanly
	var req UpsertRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return configurationInvalid(patch.Group, "values", "Invalid configuration values")
	}
	if err := validateUpsert(&req); err != nil {
		field, first := "values", len(err.Error())
		for _, f := range configurationFields[patch.Group] {
			if at := fieldAt(err.Error(), f); at >= 0 && at < first {
				field, first = f, at
			}
		}
		return configurationInvalid(patch.Group, field, err.Error())
	}
	// Empty means "unset" and is tolerated on write, matching validateUpsert.
	// Rejecting it here would wedge every group on a namespace already storing it.
	if req.DenseDistance != nil && *req.DenseDistance != "" && !denseDistances[*req.DenseDistance] {
		return configurationInvalid("embeddings", "dense_distance", "Select cosine or dot")
	}
	for action := range req.ActionWeights {
		if strings.TrimSpace(action) == "" || strings.TrimSpace(action) != action {
			return configurationInvalid("signals", "action_weights", "Action names must be non-empty and have no surrounding whitespace")
		}
	}
	for _, f := range []string{"catalog_max_attempts", "catalog_max_content_bytes"} {
		if string(values[f]) != "null" {
			var n int
			if err := json.Unmarshal(values[f], &n); err != nil || n < 1 {
				return configurationInvalid("embeddings", f, "Override must be a positive integer; use null to inherit")
			}
		}
	}
	if req.DenseSource != nil && *req.DenseSource == "catalog" {
		if err := s.validateCatalogStrategy(ctx, ns, &UpdateCatalogRequest{Enabled: true, StrategyID: *req.CatalogStrategyID, StrategyVersion: *req.CatalogStrategyVersion, Params: req.CatalogStrategyParams}, req.EmbeddingDim); err != nil {
			return configurationInvalid("embeddings", "catalog_strategy_id", err.Error())
		}
	}
	if s.denseCollections != nil && patch.Group == "embeddings" {
		for _, f := range []string{"embedding_dim", "dense_distance"} {
			if string(values[f]) != string(current.Groups["embeddings"].Values[f]) {
				exists, err := s.denseCollections.DenseCollectionsExist(ctx, ns, current.Generation)
				if err != nil {
					return err
				}
				if exists {
					return configurationInvalid("embeddings", f, "Dense collections exist; a separate collection migration is required")
				}
			}
		}
	}
	return nil
}
