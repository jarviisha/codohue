package nsconfig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jarviisha/codohue/internal/core/namespace"
)

var configurationFields = map[string][]string{
	"recommendations": {"alpha", "gamma", "max_results", "seen_items_days", "exclude_authored"},
	"signals":         {"action_weights", "lambda"},
	"trending":        {"trending_window", "trending_ttl", "lambda_trending"},
	"embeddings":      {"dense_source", "embedding_dim", "dense_distance", "catalog_strategy_id", "catalog_strategy_version", "catalog_strategy_params", "catalog_max_attempts", "catalog_max_content_bytes"},
}
var configurationGuidance = map[string]string{
	"recommendations": "Used by new recommendation calculations. Existing cached recommendations may delay visible changes.",
	"signals":         "Run a batch to rebuild sparse vectors. Action weights also affect the next trending rebuild.",
	"trending":        "Used by the next scheduled or manual batch. Cache expiry does not trigger a rebuild.",
	"embeddings":      "Used by the selected embedding producer. Existing catalog items may need re-embedding. Saving does not start a job.",
}

func configurationColumn(field string) string {
	if field == "lambda" {
		return "time_decay_factor"
	}
	return field
}

// ReadConfiguration explicitly projects the allowlist, excluding credentials.
func (r *Repository) ReadConfiguration(ctx context.Context, ns string) (*namespace.Configuration, error) {
	var columns []string
	for group, fields := range configurationFields {
		columns = append(columns, "'"+group+"_revision', c."+group+"_revision")
		for _, field := range fields {
			columns = append(columns, "'"+field+"', c."+configurationColumn(field))
		}
	}
	var raw []byte
	out := &namespace.Configuration{Namespace: ns, Groups: map[string]namespace.ConfigurationGroup{}}
	err := r.queryRowFn(ctx, `SELECT c.generation, jsonb_build_object(`+strings.Join(columns, ",")+`) FROM namespace_configs c JOIN namespace_lifecycles l USING(namespace) WHERE c.namespace=$1 AND l.state='active' AND c.generation=l.generation`, ns).Scan(&out.Generation, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, &namespace.ConfigurationError{Status: 404, Code: "not_found", Message: "Namespace is not active or does not exist"}
	}
	if err != nil {
		return nil, err
	}
	var values map[string]json.RawMessage
	if err = json.Unmarshal(raw, &values); err != nil {
		return nil, err
	}
	for group, fields := range configurationFields {
		g := namespace.ConfigurationGroup{Values: map[string]json.RawMessage{}, Locks: map[string]string{}, Guidance: configurationGuidance[group]}
		if err = json.Unmarshal(values[group+"_revision"], &g.Revision); err != nil {
			return nil, err
		}
		for _, f := range fields {
			g.Values[f] = values[f]
		}
		out.Groups[group] = g
	}
	return out, nil
}

// ChangeConfiguration locks the current row before merging and validating.
// Validation-only calls use the identical path but perform no UPDATE.
func (r *Repository) ChangeConfiguration(ctx context.Context, ns string, req *namespace.ConfigurationPatch, validateOnly bool, validate func(*namespace.Configuration, map[string]json.RawMessage) error) (*namespace.Configuration, error) {
	var result *namespace.Configuration
	err := r.withinTx(ctx, func(tx *Repository) error {
		if err := tx.execFn(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,5757))`, ns); err != nil {
			return err
		}
		var generation int64
		err := tx.queryRowFn(ctx, `SELECT generation FROM namespace_configs WHERE namespace=$1 FOR UPDATE`, ns).Scan(&generation)
		if errors.Is(err, pgx.ErrNoRows) {
			return &namespace.ConfigurationError{Status: 404, Code: "not_found", Message: "Namespace does not exist"}
		}
		if err != nil {
			return err
		}
		current, err := tx.ReadConfiguration(ctx, ns)
		if err != nil {
			return err
		}
		if current.Generation != req.Generation || current.Groups[req.Group].Revision != req.BaseRevision {
			return &namespace.ConfigurationError{Status: 409, Code: "configuration_conflict", Message: "This group changed since you started editing. Review the current values before saving.", Current: current}
		}
		merged := map[string]json.RawMessage{}
		for _, g := range current.Groups {
			for f, v := range g.Values {
				merged[f] = v
			}
		}
		for f, v := range req.Changes {
			merged[f] = v
		}
		if err := validate(current, merged); err != nil {
			return err
		}
		if !validateOnly && len(req.Changes) > 0 {
			sets := []string{}
			args := []any{ns}
			// Only allowlisted columns can enter this statement.
			for _, f := range configurationFields[req.Group] {
				if v, ok := req.Changes[f]; ok {
					args = append(args, string(v))
					sets = append(sets, fmt.Sprintf("%s=(jsonb_populate_record(NULL::namespace_configs, jsonb_build_object('%s', $%d::jsonb))).%s", configurationColumn(f), configurationColumn(f), len(args), configurationColumn(f)))
				}
			}
			if len(sets) > 0 {
				if err := tx.execFn(ctx, `UPDATE namespace_configs SET `+strings.Join(sets, ",")+`, updated_at=NOW() WHERE namespace=$1`, args...); err != nil {
					return err
				}
			}
		}
		result, err = tx.ReadConfiguration(ctx, ns)
		if err == nil {
			for group, g := range result.Groups {
				g.Locks = current.Groups[group].Locks
				result.Groups[group] = g
			}
		}
		return err
	})
	return result, err
}
