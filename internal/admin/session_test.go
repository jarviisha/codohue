package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSessionManager_IssueAndValidate(t *testing.T) {
	sm, err := NewSessionManager(nil)
	if err != nil {
		t.Fatalf("new session manager: %v", err)
	}

	token, expiresAt, err := sm.Issue()
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if !sm.Validate(token) {
		t.Fatal("freshly issued token must validate")
	}
	if time.Until(expiresAt) <= 0 {
		t.Fatal("expiry must be in the future")
	}
}

func TestSessionManager_RejectsForeignSecret(t *testing.T) {
	// A token signed under one secret must not validate under another — this
	// is what makes a leaked token useless as an offline oracle for the API
	// key: the signing material is independent random bytes.
	a, _ := NewSessionManager([]byte("secret-a"))
	b, _ := NewSessionManager([]byte("secret-b"))

	token, _, err := a.Issue()
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if b.Validate(token) {
		t.Fatal("token signed under a different secret must be rejected")
	}
}

func TestSessionManager_RevokeInvalidatesToken(t *testing.T) {
	sm, _ := NewSessionManager(nil)
	token, _, err := sm.Issue()
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	sm.Revoke(token)

	if sm.Validate(token) {
		t.Fatal("a revoked token must stop validating — logout has to mean logout")
	}

	// Other sessions stay valid.
	other, _, err := sm.Issue()
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if !sm.Validate(other) {
		t.Fatal("revocation must be per-token, not global")
	}
}

func TestSessionManager_ExpiredTokenRejected(t *testing.T) {
	sm, _ := NewSessionManager(nil)
	now := time.Now()
	sm.now = func() time.Time { return now }

	token, _, err := sm.Issue()
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	now = now.Add(sessionTTL + time.Minute)
	if sm.Validate(token) {
		t.Fatal("expired token must be rejected")
	}
}

func TestSessionManager_TamperedClaimsRejected(t *testing.T) {
	sm, _ := NewSessionManager(nil)
	token, _, err := sm.Issue()
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	// Flip one byte of the claims segment; the signature must catch it.
	tampered := []byte(token)
	mid := len(tampered) / 2
	tampered[mid] ^= 0x01
	if sm.Validate(string(tampered)) {
		t.Fatal("tampered token must be rejected")
	}
}

func TestLoginRateLimiter_BlocksAfterFailureBurst(t *testing.T) {
	l := newLoginRateLimiter()
	now := time.Now()
	l.now = func() time.Time { return now }

	// Only failures consume the budget.
	for i := 0; i < loginBurst; i++ {
		if l.Blocked("10.0.0.1") {
			t.Fatalf("failure %d within burst must not be blocked yet", i)
		}
		l.RecordFailure("10.0.0.1")
	}
	if !l.Blocked("10.0.0.1") {
		t.Fatal("past the burst of failures must be blocked")
	}
	// A different IP has its own bucket.
	if l.Blocked("10.0.0.2") {
		t.Fatal("other IPs must not share the drained bucket")
	}
	// Refill restores capacity over time.
	now = now.Add(loginRefillEvery * 2)
	if l.Blocked("10.0.0.1") {
		t.Fatal("bucket must refill after the refill interval")
	}
}

func TestRequestIsTLS(t *testing.T) {
	plain := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "http://x/", http.NoBody)
	if requestIsTLS(plain) {
		t.Error("plain http must not count as TLS")
	}
	forwarded := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "http://x/", http.NoBody)
	forwarded.Header.Set("X-Forwarded-Proto", "https")
	if !requestIsTLS(forwarded) {
		t.Error("proxy-terminated TLS must count")
	}
}
