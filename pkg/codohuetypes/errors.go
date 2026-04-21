package codohuetypes

// ErrorDetail is the machine-readable error payload returned by API handlers.
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ErrorResponse wraps ErrorDetail in a stable top-level object for clients.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}
