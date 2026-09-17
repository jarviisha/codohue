package codohuetypes

import "time"

// OperatorIdentity identifies a human console session without credential material.
type OperatorIdentity struct {
	Name string `json:"name"`
	Role string `json:"role"`
}

// OperatorAccount is the owner-visible account lifecycle state.
type OperatorAccount struct {
	Username string `json:"username"`
	Role     string `json:"role"`
	Disabled bool   `json:"disabled"`
}

// OperatorSessionRequest starts a human session; machine tokens are not accepted.
type OperatorSessionRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// OperatorSessionResponse is returned with the opaque HttpOnly session cookie.
type OperatorSessionResponse struct {
	ExpiresAt time.Time        `json:"expires_at"`
	Actor     OperatorIdentity `json:"actor"`
}

// OperatorAccountRequest creates or updates an account. Empty password preserves an existing hash.
type OperatorAccountRequest struct {
	Password string `json:"password,omitempty"`
	Role     string `json:"role"`
	Disabled bool   `json:"disabled"`
}

// ServiceTokenRequest provisions an immutable pre-generated token.
type ServiceTokenRequest struct {
	Token       string   `json:"token"`
	Permissions []string `json:"permissions"`
	Namespaces  []string `json:"namespaces"`
}
