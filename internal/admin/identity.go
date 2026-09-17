package admin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jarviisha/codohue/internal/core/access"
	"github.com/jarviisha/codohue/internal/core/httpapi"
	"github.com/jarviisha/codohue/pkg/codohuetypes"
)

// IdentityStore is the shared security state needed by the admin HTTP boundary.
type IdentityStore interface {
	AllowAttempt(context.Context, string) (bool, error)
	RefundAttempt(context.Context, string) error
	Login(context.Context, string, string, string) (access.Actor, error)
	IssueSession(context.Context, access.Actor) (string, time.Time, error)
	Session(context.Context, string) (access.Actor, error)
	RevokeSession(context.Context, string) error
	ServiceToken(context.Context, string) (access.Actor, error)
	Audit(context.Context, string, string, string) error
	SetAccount(context.Context, string, string, string, string, bool, bool) error
	Accounts(context.Context) ([]codohuetypes.OperatorAccount, error)
	ProvisionToken(context.Context, string, string, string, []string, []string) error
	RevokeToken(context.Context, string, string) error
}

// IdentityOptions configures the browser trust boundary independently of credentials.
type IdentityOptions struct {
	SecureCookies  bool
	TrustedProxies []netip.Prefix
	AllowedOrigin  string
}

// SetIdentity wires the shared identity store and trusted browser configuration.
func (h *Handler) SetIdentity(store IdentityStore, opts IdentityOptions) {
	h.identity = store
	h.identityOptions = opts
	h.sessionCheckInterval = 15 * time.Second
}
func (h *Handler) trustedProxy(r *http.Request) bool {
	addr, err := netip.ParseAddr(clientIP(r))
	if err != nil {
		return false
	}
	for _, prefix := range h.identityOptions.TrustedProxies {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}
func (h *Handler) remoteIP(r *http.Request) string {
	if h.trustedProxy(r) {
		// Walk from the trusted edge toward the caller; never trust a client-supplied leftmost entry.
		chain := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
		for i := len(chain) - 1; i >= 0; i-- {
			addr, err := netip.ParseAddr(strings.TrimSpace(chain[i]))
			if err != nil {
				break
			}
			trusted := false
			for _, p := range h.identityOptions.TrustedProxies {
				trusted = trusted || p.Contains(addr)
			}
			if !trusted {
				return addr.String()
			}
		}
	}
	return clientIP(r)
}
func (h *Handler) secureCookie(r *http.Request) bool {
	return h.identityOptions.SecureCookies || r.TLS != nil || (h.trustedProxy(r) && strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"))
}
func (h *Handler) csrfOK(r *http.Request) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
		return true
	}
	if r.Header.Get("X-Codohue-CSRF") != "1" {
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	} // Non-browser clients must still supply the custom header.
	if h.identityOptions.AllowedOrigin != "" && origin == h.identityOptions.AllowedOrigin {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	scheme := "http"
	if h.secureCookie(r) {
		scheme = "https"
	}
	return u.Scheme == scheme && strings.EqualFold(u.Host, r.Host)
}
func identityError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, access.ErrThrottled):
		w.Header().Set("Retry-After", "60")
		httpapi.WriteError(w, 429, "rate_limited", "too many login attempts")
	case errors.Is(err, access.ErrCredentials):
		httpapi.WriteError(w, 401, "unauthorized", "invalid credentials")
	case errors.Is(err, access.ErrInvalid):
		httpapi.WriteError(w, 400, "invalid_request", err.Error())
	case errors.Is(err, access.ErrConflict):
		httpapi.WriteError(w, 409, "conflict", err.Error())
	default:
		httpapi.WriteError(w, 503, "identity_unavailable", "identity store unavailable")
	}
}
func (h *Handler) createOperatorSession(w http.ResponseWriter, r *http.Request) {
	if !h.csrfOK(r) {
		httpapi.WriteError(w, 403, "csrf", "same-origin CSRF header required")
		return
	}
	var req codohuetypes.OperatorSessionRequest
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := httpapi.DecodeStrict(r.Body, &req); err != nil {
		httpapi.WriteError(w, 400, "invalid_request", "username and password required")
		return
	}
	a, err := h.identity.Login(r.Context(), h.remoteIP(r), req.Username, req.Password)
	if err != nil {
		identityError(w, err)
		return
	}
	if err = h.identity.Audit(r.Context(), "operator:"+a.Name, "session.create", a.Name); err != nil {
		identityError(w, err)
		return
	}
	token, expiry, err := h.identity.IssueSession(r.Context(), a)
	if err != nil {
		identityError(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: token, Path: "/api", HttpOnly: true, Secure: h.secureCookie(r), MaxAge: int(time.Until(expiry).Seconds()), SameSite: http.SameSiteStrictMode})
	httpapi.WriteJSON(w, 201, codohuetypes.OperatorSessionResponse{ExpiresAt: expiry, Actor: codohuetypes.OperatorIdentity{Name: a.Name, Role: a.Role}})
}

// RequireIdentity enforces distinct session and service-token capabilities.
func (h *Handler) RequireIdentity(legacySessions *SessionManager, legacyKey string) func(http.Handler) http.Handler {
	if h.identity == nil {
		return RequireSessionOrBearer(legacySessions, legacyKey)
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var a access.Actor
			var err error
			var sessionToken, serviceToken string
			bearer := r.Header.Get("Authorization")
			if bearer != "" {
				token, ok := strings.CutPrefix(bearer, "Bearer ")
				if !ok {
					identityError(w, access.ErrCredentials)
					return
				}
				a, err = h.identity.ServiceToken(r.Context(), token)
				if err == nil {
					serviceToken = token
				}
				if errors.Is(err, access.ErrCredentials) && legacyKey != "" {
					allowed, limitErr := h.identity.AllowAttempt(r.Context(), h.remoteIP(r))
					if limitErr != nil {
						identityError(w, limitErr)
						return
					}
					if !allowed {
						identityError(w, access.ErrThrottled)
						return
					}
				}
				if errors.Is(err, access.ErrCredentials) && legacyKey != "" && constantTimeEqual(token, legacyKey) {
					if refundErr := h.identity.RefundAttempt(r.Context(), h.remoteIP(r)); refundErr != nil {
						identityError(w, refundErr)
						return
					}
					a = access.Actor{Name: "legacy", Role: "service", Permissions: []string{"admin:read", "admin:write", "namespace:provision"}, Namespaces: []string{"*"}}
					err = nil
				}
			} else {
				cookie, e := r.Cookie(sessionCookieName)
				if e != nil {
					identityError(w, access.ErrCredentials)
					return
				}
				sessionToken = cookie.Value
				a, err = h.identity.Session(r.Context(), sessionToken)
				if err == nil && !h.csrfOK(r) {
					httpapi.WriteError(w, 403, "csrf", "same-origin CSRF header required")
					return
				}
			}
			if err != nil {
				identityError(w, err)
				return
			}
			ns := chi.URLParam(r, "ns")
			permission := "admin:read"
			if r.Method != "GET" && r.Method != "HEAD" {
				permission = "admin:write"
			}
			sensitive := strings.HasPrefix(r.URL.Path, "/api/admin/v1/accounts") || strings.HasPrefix(r.URL.Path, "/api/admin/v1/service-tokens") || r.URL.Path == "/api/admin/v1/reset"
			sessionRoute := strings.HasPrefix(r.URL.Path, "/api/v1/auth/sessions/")
			allowed := a.Allows(permission, ns)
			if a.Role == "service" && ns == "" {
				allowed = allowed && containsWildcard(a.Namespaces)
			}
			if r.Method == "PUT" && ns != "" && r.URL.Path == "/api/admin/v1/namespaces/"+ns {
				allowed = allowed || a.Allows("namespace:provision", ns)
			}
			if sensitive {
				allowed = a.Role == "owner"
			}
			if sessionRoute {
				allowed = bearer == ""
			}
			if !allowed {
				httpapi.WriteError(w, 403, "forbidden", "credential lacks permission")
				return
			}
			if r.Method != "GET" && r.Method != "HEAD" {
				// Record an authorized attempt before executing it; do not include bodies, query strings or headers.
				actor := a.Role + ":" + a.Name
				if err := h.identity.Audit(r.Context(), actor, r.Method+" "+chi.RouteContext(r.Context()).RoutePattern(), r.URL.Path); err != nil {
					identityError(w, err)
					return
				}
			}
			ctx := access.WithActor(r.Context(), a)
			if (sessionToken != "" || serviceToken != "") && strings.HasSuffix(r.URL.Path, "/stream") {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				defer cancel()
				token := sessionToken
				if serviceToken != "" {
					token = serviceToken
				}
				go h.watchCredential(ctx, cancel, token, serviceToken != "")
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
func containsWildcard(namespaces []string) bool {
	for _, ns := range namespaces {
		if ns == "*" {
			return true
		}
	}
	return false
}

// GetCurrentSession returns the individual operator identity.
func (h *Handler) GetCurrentSession(w http.ResponseWriter, r *http.Request) {
	a := access.CurrentActor(r.Context())
	httpapi.WriteJSON(w, 200, codohuetypes.OperatorIdentity{Name: a.Name, Role: a.Role})
}

// ListAccounts returns all operator accounts to an authorized owner.
func (h *Handler) ListAccounts(w http.ResponseWriter, r *http.Request) {
	if h.identity == nil {
		httpapi.WriteError(w, 503, "unavailable", "accounts unavailable")
		return
	}
	items, err := h.identity.Accounts(r.Context())
	if err != nil {
		identityError(w, err)
		return
	}
	httpapi.WriteJSON(w, 200, items)
}

// PutAccount creates or updates an operator and revokes its sessions.
func (h *Handler) PutAccount(w http.ResponseWriter, r *http.Request) {
	if h.identity == nil {
		httpapi.WriteError(w, 503, "unavailable", "accounts unavailable")
		return
	}
	var req codohuetypes.OperatorAccountRequest
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := httpapi.DecodeStrict(r.Body, &req); err != nil {
		httpapi.WriteError(w, 400, "invalid_request", "invalid account")
		return
	}
	if err := h.identity.SetAccount(r.Context(), "operator:"+access.CurrentActor(r.Context()).Name, chi.URLParam(r, "username"), req.Password, req.Role, req.Disabled, false); err != nil {
		identityError(w, err)
		return
	}
	w.WriteHeader(204)
}

// PutServiceToken provisions an immutable machine credential.
func (h *Handler) PutServiceToken(w http.ResponseWriter, r *http.Request) {
	if h.identity == nil {
		httpapi.WriteError(w, 503, "unavailable", "tokens unavailable")
		return
	}
	var req codohuetypes.ServiceTokenRequest
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	if err := httpapi.DecodeStrict(r.Body, &req); err != nil {
		httpapi.WriteError(w, 400, "invalid_request", "invalid token specification")
		return
	}
	if err := h.identity.ProvisionToken(r.Context(), "operator:"+access.CurrentActor(r.Context()).Name, chi.URLParam(r, "name"), req.Token, req.Permissions, req.Namespaces); err != nil {
		identityError(w, err)
		return
	}
	w.WriteHeader(204)
}

// DeleteServiceToken revokes a named machine credential.
func (h *Handler) DeleteServiceToken(w http.ResponseWriter, r *http.Request) {
	if h.identity == nil {
		httpapi.WriteError(w, 503, "unavailable", "tokens unavailable")
		return
	}
	if err := h.identity.RevokeToken(r.Context(), "operator:"+access.CurrentActor(r.Context()).Name, chi.URLParam(r, "name")); err != nil {
		identityError(w, err)
		return
	}
	w.WriteHeader(204)
}

// ParseTrustedProxies rejects invalid CIDRs rather than silently trusting forwarded headers.
func ParseTrustedProxies(raw string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, s := range strings.Split(raw, ",") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		p, err := netip.ParsePrefix(s)
		if err != nil {
			return nil, fmt.Errorf("parse trusted proxy CIDR: %w", err)
		}
		out = append(out, p)
	}
	return out, nil
}

// watchCredential bounds access through an already-open stream after revocation.
func (h *Handler) watchCredential(ctx context.Context, cancel context.CancelFunc, token string, service bool) {
	ticker := time.NewTicker(h.sessionCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			lookup := h.identity.Session
			if service {
				lookup = h.identity.ServiceToken
			}
			if _, err := lookup(ctx, token); err != nil {
				cancel()
				return
			}
		}
	}
}
