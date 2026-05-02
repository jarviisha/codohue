package admin

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jarviisha/codohue/internal/core/httpapi"
)

const sessionCookieName = "codohue_admin_session"

type jwtHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

type jwtClaims struct {
	Sub string `json:"sub"`
	Iat int64  `json:"iat"`
	Exp int64  `json:"exp"`
}

func createSessionToken(apiKey string) (string, error) {
	h := jwtHeader{Alg: "HS256", Typ: "JWT"}
	c := jwtClaims{
		Sub: "admin",
		Iat: time.Now().Unix(),
		Exp: time.Now().Add(8 * time.Hour).Unix(),
	}

	headerJSON, err := json.Marshal(h)
	if err != nil {
		return "", fmt.Errorf("marshal jwt header: %w", err)
	}
	claimsJSON, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("marshal jwt claims: %w", err)
	}

	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)
	claimsB64 := base64.RawURLEncoding.EncodeToString(claimsJSON)
	payload := headerB64 + "." + claimsB64

	mac := hmac.New(sha256.New, []byte(apiKey))
	mac.Write([]byte(payload)) //nolint:errcheck // hmac.Hash.Write never returns an error
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return payload + "." + sig, nil
}

func validateSessionToken(token, apiKey string) bool {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return false
	}

	payload := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, []byte(apiKey))
	mac.Write([]byte(payload)) //nolint:errcheck // hmac.Hash.Write never returns an error
	expectedSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(parts[2]), []byte(expectedSig)) {
		return false
	}

	claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	var claims jwtClaims
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return false
	}
	return time.Now().Unix() < claims.Exp
}

// RequireSession is middleware that validates the session cookie on every request.
// If the session is missing or invalid, it returns 401.
func RequireSession(apiKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(sessionCookieName)
			if err != nil || !validateSessionToken(cookie.Value, apiKey) {
				httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "invalid or missing session")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
