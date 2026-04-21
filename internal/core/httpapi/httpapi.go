package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/jarviisha/codohue/pkg/codohuetypes"
)

// ErrorDetail is the machine-readable error payload returned by API handlers.
// Re-exported from codohuetypes so SDK clients parse the same struct.
type ErrorDetail = codohuetypes.ErrorDetail

// ErrorResponse wraps ErrorDetail in a stable top-level object for clients.
// Re-exported from codohuetypes so SDK clients parse the same struct.
type ErrorResponse = codohuetypes.ErrorResponse

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
