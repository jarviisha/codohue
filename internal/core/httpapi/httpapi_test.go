package httpapi

import (
	"errors"
	"strings"
	"testing"
)

func TestDecodeStrict_ValidBody(t *testing.T) {
	var out struct {
		SubjectID string `json:"subject_id"`
	}
	if err := DecodeStrict(strings.NewReader(`{"subject_id":"u1"}`), &out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.SubjectID != "u1" {
		t.Fatalf("subject_id = %q, want u1", out.SubjectID)
	}
}

func TestDecodeStrict_RejectsUnknownField(t *testing.T) {
	var out struct {
		SubjectID string `json:"subject_id"`
	}
	err := DecodeStrict(strings.NewReader(`{"subject_id":"u1","namespace":"x"}`), &out)
	if err == nil {
		t.Fatal("expected an error for an unknown field, got nil")
	}
	if !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("error = %q, want it to mention the unknown field", err)
	}
}

func TestDecodeStrict_RejectsTrailingData(t *testing.T) {
	var out struct {
		SubjectID string `json:"subject_id"`
	}
	err := DecodeStrict(strings.NewReader(`{"subject_id":"u1"}{"subject_id":"u2"}`), &out)
	if err == nil {
		t.Fatal("expected an error for trailing data, got nil")
	}
	if !strings.Contains(err.Error(), "trailing data") {
		t.Fatalf("error = %q, want it to mention trailing data", err)
	}
}

func TestDecodeStrict_RejectsMalformedJSON(t *testing.T) {
	var out struct {
		SubjectID string `json:"subject_id"`
	}
	if err := DecodeStrict(strings.NewReader(`not-json`), &out); err == nil {
		t.Fatal("expected an error for malformed JSON, got nil")
	}
}

func TestDecodeStrictMax_RejectsOversizedBody(t *testing.T) {
	var out struct {
		Value string `json:"value"`
	}
	err := DecodeStrictMax(strings.NewReader(`{"value":"123456789"}`), &out, 8)
	if !errors.Is(err, ErrBodyTooLarge) {
		t.Fatalf("expected ErrBodyTooLarge, got %v", err)
	}
}
