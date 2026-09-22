package nsconfig

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jarviisha/codohue/internal/core/namespace"
	"testing"
)

func TestConfigurationPatchParsing(t *testing.T) {
	for _, tc := range []struct {
		name, group, field, value string
		valid                     bool
	}{
		{"number", "trending", "trending_ttl", "3600", true},
		{"null inheritance", "embeddings", "catalog_max_attempts", "null", true},
		{"null forbidden", "trending", "trending_ttl", "null", false},
		{"wrong group", "signals", "alpha", "0.5", false},
		{"unknown", "trending", "secret", "1", false},
		{"wrong type", "trending", "trending_ttl", `"3600"`, false},
		{"fractional integer", "trending", "trending_ttl", "1.5", false},
		{"weights replacement", "signals", "action_weights", `{}`, true},
		{"group", "wrong", "alpha", "1", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateConfigurationPatch(&namespace.ConfigurationPatch{Group: tc.group, Generation: 1, BaseRevision: 1, Changes: map[string]json.RawMessage{tc.field: json.RawMessage(tc.value)}})
			if (err == nil) != tc.valid {
				t.Fatalf("err=%v", err)
			}
		})
	}
}
func TestConfigurationRejectsBlankAction(t *testing.T) {
	s := NewService(nil)
	vals := map[string]json.RawMessage{"action_weights": json.RawMessage(`{" ":1}`), "catalog_max_attempts": json.RawMessage(`null`), "catalog_max_content_bytes": json.RawMessage(`null`)}
	err := s.validateConfiguration(context.Background(), "x", &namespace.ConfigurationPatch{Group: "signals"}, &namespace.Configuration{}, vals)
	var invalid *namespace.ConfigurationError
	if !errors.As(err, &invalid) || invalid.Fields["signals.action_weights"] == "" {
		t.Fatalf("%v", err)
	}
}

// A "lambda_trending" complaint must not be blamed on the signals group's
// shorter "lambda", which is a substring of it.
func TestConfigurationAttributesWholeFieldNames(t *testing.T) {
	s := NewService(nil)
	vals := map[string]json.RawMessage{
		"lambda_trending":           json.RawMessage(`-1`),
		"catalog_max_attempts":      json.RawMessage(`null`),
		"catalog_max_content_bytes": json.RawMessage(`null`),
	}
	err := s.validateConfiguration(context.Background(), "x", &namespace.ConfigurationPatch{Group: "signals"}, &namespace.Configuration{}, vals)
	var invalid *namespace.ConfigurationError
	if !errors.As(err, &invalid) {
		t.Fatalf("want validation error, got %v", err)
	}
	if invalid.Fields["signals.lambda"] != "" {
		t.Fatalf("lambda_trending error misattributed to signals.lambda: %v", invalid.Fields)
	}
}

// An echoed value must not be mistaken for the field being complained about:
// "dense_distance must be one of cosine|dot, got \"dense_source\"".
func TestConfigurationIgnoresFieldNamesInsideValues(t *testing.T) {
	s := NewService(nil)
	vals := map[string]json.RawMessage{"dense_distance": json.RawMessage(`"dense_source"`)}
	err := s.validateConfiguration(context.Background(), "x", &namespace.ConfigurationPatch{Group: "embeddings"}, &namespace.Configuration{}, vals)
	var invalid *namespace.ConfigurationError
	if !errors.As(err, &invalid) {
		t.Fatalf("want validation error, got %v", err)
	}
	if invalid.Fields["embeddings.dense_source"] != "" {
		t.Fatalf("echoed value misattributed to dense_source: %v", invalid.Fields)
	}
}
