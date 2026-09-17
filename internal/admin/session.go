package admin

import (
	"crypto/sha256"
	"crypto/subtle"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jarviisha/codohue/internal/core/access"
)

const sessionTTL = 8 * time.Hour

// SessionManager is an in-memory opaque session adapter for isolated handlers.
// Production uses IdentityStore with durable account-bound sessions.
type SessionManager struct {
	ttl    time.Duration
	now    func() time.Time
	mu     sync.Mutex
	tokens map[string]time.Time
}

// NewSessionManager creates an isolated store. The deprecated signing secret is ignored.
func NewSessionManager(_ []byte) (*SessionManager, error) {
	return &SessionManager{ttl: sessionTTL, now: time.Now, tokens: make(map[string]time.Time)}, nil
}

// Issue creates a random opaque token in the isolated store.
func (m *SessionManager) Issue() (string, time.Time, error) {
	token, err := access.RandomToken()
	if err != nil {
		return "", time.Time{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	for hash, exp := range m.tokens {
		if !exp.After(now) {
			delete(m.tokens, hash)
		}
	}
	expiry := now.Add(m.ttl)
	m.tokens[access.Digest(token)] = expiry
	return token, expiry, nil
}

// Validate checks token presence and expiry.
func (m *SessionManager) Validate(token string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	exp, ok := m.tokens[access.Digest(token)]
	return ok && exp.After(m.now())
}

// Revoke deletes an opaque token.
func (m *SessionManager) Revoke(token string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.tokens, access.Digest(token))
}

// ─── login rate limiting ─────────────────────────────────────────────────────

const (
	loginBurst       = 5
	loginRefillEvery = 10 * time.Second
)

// loginRateLimiter is a per-IP token bucket for the public login endpoint.
// Production operator login uses the shared PostgreSQL limiter.
type loginRateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*loginBucket
	now     func() time.Time
}

type loginBucket struct {
	tokens float64
	last   time.Time
}

func newLoginRateLimiter() *loginRateLimiter {
	return &loginRateLimiter{buckets: make(map[string]*loginBucket), now: time.Now}
}

// Allow atomically reserves an attempt before credential verification.
func (l *loginRateLimiter) Allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	b := l.bucketLocked(ip, l.now())
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// Blocked inspects the current budget without reserving an attempt.
func (l *loginRateLimiter) Blocked(ip string) bool {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	b := l.bucketLocked(ip, now)
	return b.tokens < 1
}

// RecordFailure consumes one token for ip after a failed login attempt.
func (l *loginRateLimiter) RecordFailure(ip string) {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	b := l.bucketLocked(ip, now)
	if b.tokens >= 1 {
		b.tokens--
	}
}

// bucketLocked returns ip's bucket with lazy refill applied. Caller holds mu.
func (l *loginRateLimiter) bucketLocked(ip string, now time.Time) *loginBucket {
	b, ok := l.buckets[ip]
	if !ok {
		b = &loginBucket{tokens: loginBurst, last: now}
		l.buckets[ip] = b
	}
	refill := now.Sub(b.last).Seconds() / loginRefillEvery.Seconds()
	b.tokens = min(loginBurst, b.tokens+refill)
	b.last = now
	// Prune buckets that have sat full for a while so the map stays bounded
	// by recent-client count.
	for k, other := range l.buckets {
		if k != ip && other.tokens >= loginBurst && now.Sub(other.last) > time.Hour {
			delete(l.buckets, k)
		}
	}
	return b
}

// clientIP extracts the caller's IP for rate-limiting. RemoteAddr is the
// authority — X-Forwarded-For is client-controlled and would let an attacker
// rotate buckets for free.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// requestIsTLS reports whether the request arrived over HTTPS, directly or
// via a terminating proxy. Drives the session cookie's Secure flag: set it
// on HTTPS deployments, omit it for plain-HTTP dev so the cookie still works.
func requestIsTLS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// constantTimeEqual compares two secrets in constant time via fixed-length
// digests. Mirrors auth.ConstantTimeEqual — the import rule forbids admin
// from importing that peer domain, so the helper lives in both with
// cross-references (same convention as the repeated stream-name literals).
func constantTimeEqual(a, b string) bool {
	ha := sha256.Sum256([]byte(a))
	hb := sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(ha[:], hb[:]) == 1
}
