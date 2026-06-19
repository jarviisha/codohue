package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/jarviisha/codohue/pkg/codohuetypes"
)

// ErrorDetail is the machine-readable error payload returned by API handlers.
// Re-exported from codohuetypes so SDK clients parse the same struct.
type ErrorDetail = codohuetypes.ErrorDetail

// ErrorResponse wraps ErrorDetail in a stable top-level object for clients.
// Re-exported from codohuetypes so SDK clients parse the same struct.
type ErrorResponse = codohuetypes.ErrorResponse

// DecodeStrict reads exactly one JSON value from r into v, rejecting unknown
// fields and any trailing data after the value. It locks the request contract:
// a client typo (e.g. "subjectId" instead of "subject_id") or a stray extra
// field fails loudly with an error instead of being silently dropped. The
// caller maps the returned error to its own 400 envelope and metrics.
func DecodeStrict(r io.Reader, v any) error {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("decode request body: %w", err)
	}
	// A well-formed request body is a single JSON value; reject anything that
	// follows it (e.g. a second concatenated object).
	if dec.More() {
		return errors.New("unexpected trailing data after JSON body")
	}
	return nil
}

// WriteJSON writes a JSON response with the given status code.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck // ResponseWriter write errors are not actionable here.
}

// WriteError writes a stable JSON error response for clients.
func WriteError(w http.ResponseWriter, status int, code, message string) {
	WriteJSON(w, status, ErrorResponse{
		Error: ErrorDetail{
			Code:    code,
			Message: message,
		},
	})
}
