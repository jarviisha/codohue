//go:build e2e

package e2e

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/jarviisha/codohue/internal/config"
)

func TestAdmin_RuntimeSelfReports(t *testing.T) {
	cookie := adminLogin(t)
	unauthorized := adminRequest(t, http.MethodGet, "/api/admin/v1/runtime", nil, nil)
	unauthorized.Body.Close()
	if unauthorized.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthorized status=%d", unauthorized.StatusCode)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		response := adminRequest(t, http.MethodGet, "/api/admin/v1/runtime", cookie, nil)
		var body struct {
			Processes     []config.RuntimeSnapshot `json:"processes"`
			ExpirySeconds int                      `json:"expiry_seconds"`
		}
		err := json.NewDecoder(response.Body).Decode(&body)
		response.Body.Close()
		if err != nil || response.StatusCode != 200 {
			t.Fatalf("status=%d err=%v", response.StatusCode, err)
		}
		seen := map[string]bool{}
		for _, report := range body.Processes {
			seen[report.Process] = true
		}
		if seen["admin"] && seen["api"] && body.ExpirySeconds == 120 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("runtime reports missing: %+v", body)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
