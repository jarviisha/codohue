package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/jarviisha/codohue/internal/core/namespace"
)

func TestRejectPausedNamespace(t *testing.T) {
	cases := []struct {
		name string
		cfg  *namespace.Config
		err  error
		want int
	}{
		{"active", &namespace.Config{}, nil, http.StatusOK},
		{"paused", &namespace.Config{Paused: true}, nil, http.StatusConflict},
		{"missing passes to handler", nil, nil, http.StatusOK},
		{"lookup failure", nil, errors.New("db down"), http.StatusServiceUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := chi.NewRouter()
			r.Group(func(r chi.Router) {
				r.Use(rejectPausedNamespace(func(context.Context, string) (*namespace.Config, error) {
					return tc.cfg, tc.err
				}))
				r.Get("/v1/namespaces/{ns}/trending", func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusOK)
				})
			})
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/v1/namespaces/ns/trending", http.NoBody))
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tc.want, rec.Body)
			}
		})
	}
}
