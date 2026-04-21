package codohue

import (
	"errors"
	"testing"
)

func TestAPIErrorIsSentinel(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		err    *APIError
		target error
		want   bool
	}{
		{"401 is unauthorized", &APIError{Status: 401}, ErrUnauthorized, true},
		{"404 is not found", &APIError{Status: 404}, ErrNotFound, true},
		{"400 is bad request", &APIError{Status: 400}, ErrBadRequest, true},
		{"499 is bad request", &APIError{Status: 499}, ErrBadRequest, true},
		{"500 is not bad request", &APIError{Status: 500}, ErrBadRequest, false},
		{"dim mismatch by code", &APIError{Status: 400, Code: "embedding_dimension_mismatch"}, ErrDimMismatch, true},
		{"different code not dim mismatch", &APIError{Status: 400, Code: "invalid_request"}, ErrDimMismatch, false},
		{"503 with degraded code matches ErrDegraded", &APIError{Status: 503, Code: codeServiceDegraded}, ErrDegraded, true},
		{"503 without degraded code does not match ErrDegraded", &APIError{Status: 503, Code: "unknown"}, ErrDegraded, false},
		{"501 not unauthorized", &APIError{Status: 501}, ErrUnauthorized, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := errors.Is(tc.err, tc.target); got != tc.want {
				t.Errorf("errors.Is = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAPIErrorMessage(t *testing.T) {
	t.Parallel()

	e := &APIError{Status: 400, Code: "bad", Message: "boom"}
	want := "codohue: 400 bad: boom"
	if got := e.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}
