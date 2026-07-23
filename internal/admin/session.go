package admin

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const sessionTTL = 8 * time.Hour

// SessionManager issues, validates, and revokes admin session tokens.
//
// Tokens are HMAC-signed JWTs carrying a random jti. The signing secret is
// independent random material — never the admin API key, which would turn any
// leaked session token into an offline brute-force oracle for the key itself.
// Logout revokes the token's jti in an in-memory denylist until its natural
// expiry, so a captured cookie stops working the moment the operator logs out.
//
// The denylist is process-local: a restart forgets revocations but ALSO
// rotates the boot-generated secret (unless one is pinned via env), which
// invalidates every outstanding token anyway — strictly safer.
type SessionManager struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time

	mu      sync.Mutex
	revoked map[string]int64 // jti -> exp (unix); pruned as entries expire
}

// NewSessionManager builds a manager for the given signing secret. An empty
// secret generates fresh random material for the process lifetime — restart
// then equals logout-everyone, which cmd/admin logs at startup.
func NewSessionManager(secret []byte) (*SessionManager, error) {
	if len(secret) == 0 {
		secret = make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			return nil, fmt.Errorf("generate session secret: %w", err)
		}
	}
	return &SessionManager{
		secret:  secret,
		ttl:     sessionTTL,
		now:     time.Now,
		revoked: make(map[string]int64),
	}, nil
}

type jwtHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

type jwtClaims struct {
	Sub string `json:"sub"`
	Jti string `json:"jti"`
	Iat int64  `json:"iat"`
	Exp int64  `json:"exp"`
}

// Issue creates a signed session token and returns it with its expiry.
func (m *SessionManager) Issue() (token string, expiresAt time.Time, err error) {
	jti := make([]byte, 16)
	if _, err := rand.Read(jti); err != nil {
		return "", time.Time{}, fmt.Errorf("generate jti: %w", err)
	}

	now := m.now()
	expiresAt = now.Add(m.ttl)
	c := jwtClaims{Sub: "admin", Jti: hex.EncodeToString(jti), Iat: now.Unix(), Exp: expiresAt.Unix()}

	headerJSON, err := json.Marshal(jwtHeader{Alg: "HS256", Typ: "JWT"})
	if err != nil {
		return "", time.Time{}, fmt.Errorf("marshal jwt header: %w", err)
	}
	claimsJSON, err := json.Marshal(c)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("marshal jwt claims: %w", err)
	}

	payload := base64.RawURLEncoding.EncodeToString(headerJSON) + "." + base64.RawURLEncoding.EncodeToString(claimsJSON)
	return payload + "." + m.sign(payload), expiresAt, nil
}

// Validate reports whether the token is well-formed, correctly signed,
// unexpired, and not revoked.
func (m *SessionManager) Validate(token string) bool {
	claims, ok := m.parse(token)
	if !ok {
		return false
	}
	if m.now().Unix() >= claims.Exp {
		return false
	}

	m.mu.Lock()
	_, revoked := m.revoked[claims.Jti]
	m.mu.Unlock()
	return !revoked
}

// Revoke denylists the token's jti until its natural expiry. Invalid tokens
// are ignored — they can't authenticate anyway.
func (m *SessionManager) Revoke(token string) {
	claims, ok := m.parse(token)
	if !ok || claims.Jti == "" {
		return
	}
	now := m.now().Unix()
	m.mu.Lock()
	defer m.mu.Unlock()
	// Opportunistic prune keeps the denylist bounded by the number of
	// logouts within one TTL window.
	for jti, exp := range m.revoked {
		if exp <= now {
			delete(m.revoked, jti)
		}
	}
	if claims.Exp > now {
		m.revoked[claims.Jti] = claims.Exp
	}
}

// parse verifies the signature and decodes the claims. Signature first: no
// field of an unsigned token is trusted, including exp and jti.
func (m *SessionManager) parse(token string) (jwtClaims, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return jwtClaims{}, false
	}
	payload := parts[0] + "." + parts[1]
	if !hmac.Equal([]byte(parts[2]), []byte(m.sign(payload))) {
		return jwtClaims{}, false
	}

	claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return jwtClaims{}, false
	}
	var claims jwtClaims
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return jwtClaims{}, false
	}
	return claims, true
}

func (m *SessionManager) sign(payload string) string {
	mac := hmac.New(sha256.New, m.secret)
	mac.Write([]byte(payload)) //nolint:errcheck // hmac.Hash.Write never returns an error
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// ─── login rate limiting ─────────────────────────────────────────────────────

const (
	loginBurst       = 5
	loginRefillEvery = 10 * time.Second
)

// loginRateLimiter is a per-IP token bucket for the public login endpoint.
// Combined with the constant-time key compare it makes online guessing of
// the admin key impractical. In-memory on purpose: login is admin-plane,
// low-volume, and a restart resetting the buckets is harmless.
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

// Blocked reports whether ip has exhausted its login budget, WITHOUT
// consuming a token. Only failed logins consume the budget (RecordFailure),
// so a legitimate admin presenting the correct key is never throttled no
// matter how often they log in — the budget exists to slow key GUESSING,
// which is by definition a stream of failures.
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
