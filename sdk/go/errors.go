package codohue

import (
	"errors"
	"fmt"
)

const codeServiceDegraded = "service_degraded"

// Sentinel errors — match with errors.Is on a returned error.
var (
	ErrUnauthorized = errors.New("codohue: unauthorized")
	ErrBadRequest   = errors.New("codohue: bad request")
	ErrNotFound     = errors.New("codohue: not found")
	ErrDimMismatch  = errors.New("codohue: embedding dimension mismatch")
	// ErrDegraded is returned by Healthz when the server responds 503 with a
	// parseable body. The returned *HealthStatus is still populated with the
	// per-component details so callers can inspect which dependency is down.
	ErrDegraded = errors.New("codohue: service degraded")
)

// APIError is returned when the server responds with a non-2xx status.
// Status is the HTTP status code; Code and Message come from the server's
// codohuetypes.ErrorDetail envelope.
type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("codohue: %d %s: %s", e.Status, e.Code, e.Message)
}

// Is allows APIError to match against the sentinel errors in this package via
// errors.Is. A 401 matches ErrUnauthorized, 404 matches ErrNotFound, the
// "embedding_dimension_mismatch" server code matches ErrDimMismatch, and any
// other 4xx matches ErrBadRequest.
func (e *APIError) Is(target error) bool {
	switch {
	case errors.Is(target, ErrUnauthorized):
		return e.Status == 401
	case errors.Is(target, ErrNotFound):
		return e.Status == 404
	case errors.Is(target, ErrDimMismatch):
		return e.Code == "embedding_dimension_mismatch"
	case errors.Is(target, ErrBadRequest):
		return e.Status >= 400 && e.Status < 500
	case errors.Is(target, ErrDegraded):
		return e.Code == codeServiceDegraded
	}
	return false
}
