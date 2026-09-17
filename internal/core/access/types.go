package access

import (
	"context"
	"errors"
	"slices"
)

// ErrCredentials is a generic rejection without account enumeration.
var ErrCredentials = errors.New("invalid credentials")

// ErrConflict prevents silent replacement of existing identity state.
var ErrConflict = errors.New("existing credentials or account state conflict")

// ErrInvalid reports an unsupported account or token specification.
var ErrInvalid = errors.New("invalid account or token parameters")

// Actor is a verified identity. Service permissions never imply human ownership.
type Actor struct {
	Name        string   `json:"name"`
	Role        string   `json:"role"`
	Permissions []string `json:"permissions,omitempty"`
	Namespaces  []string `json:"namespaces,omitempty"`
	Version     int64    `json:"-"`
}

// Allows checks an explicit permission and namespace scope.
func (a Actor) Allows(permission, namespace string) bool {
	if a.Role == "owner" {
		return true
	}
	if a.Role == "admin" {
		return permission == "admin:read" || permission == "admin:write"
	}
	return slices.Contains(a.Permissions, permission) && (namespace == "" || slices.Contains(a.Namespaces, "*") || slices.Contains(a.Namespaces, namespace))
}

type actorKey struct{}

// WithActor attaches a verified identity to the request context.
func WithActor(ctx context.Context, actor Actor) context.Context {
	return context.WithValue(ctx, actorKey{}, actor)
}

// CurrentActor returns the authenticated actor, or the empty unauthenticated identity.
func CurrentActor(ctx context.Context) Actor {
	if a, ok := ctx.Value(actorKey{}).(Actor); ok {
		return a
	}
	return Actor{}
}
