package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/jarviisha/codohue/pkg/codohuetypes"
)

const defaultMaxJSONBodyBytes int64 = 8 << 20

// ErrBodyTooLarge indicates that a JSON request exceeded the hard decoder cap.
var ErrBodyTooLarge = errors.New("request body too large")

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
	return DecodeStrictMax(r, v, defaultMaxJSONBodyBytes)
}

// DecodeStrictMax is DecodeStrict with an explicit byte cap. The decoder reads
// at most maxBytes+1, so an authenticated request cannot force an unbounded
// string, slice, or map allocation before business validation runs.
func DecodeStrictMax(r io.Reader, v any, maxBytes int64) error {
	if maxBytes <= 0 {
		return ErrBodyTooLarge
	}
	limited := &io.LimitedReader{R: r, N: maxBytes + 1}
	dec := json.NewDecoder(limited)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		if limited.N == 0 {
			return ErrBodyTooLarge
		}
		return fmt.Errorf("decode request body: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err == nil {
		return errors.New("unexpected trailing data after JSON body")
	} else if !errors.Is(err, io.EOF) {
		if limited.N == 0 {
			return ErrBodyTooLarge
		}
		return fmt.Errorf("decode trailing request data: %w", err)
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
