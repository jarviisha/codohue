//go:build e2e

package e2e

import (
	"net/http"
	"testing"
	"time"
)

func TestUpsertNamespace_RequiresAdminKey(t *testing.T) {
	resp := doRequest(t, http.MethodPut,
		baseURL+"/v1/config/namespaces/"+testNS, "",
		map[string]any{"action_weights": map[string]float64{"click": 1.0}})
	assertStatus(t, resp, http.StatusUnauthorized)
	resp.Body.Close()
}

func TestUpsertNamespace_WrongKey(t *testing.T) {
	resp := doRequest(t, http.MethodPut,
		baseURL+"/v1/config/namespaces/"+testNS, "totally-wrong-key",
		map[string]any{"action_weights": map[string]float64{"click": 1.0}})
	assertStatus(t, resp, http.StatusUnauthorized)
	resp.Body.Close()
}

func TestUpsertNamespace_UpdateReturnsNoAPIKey(t *testing.T) {
	// testNS was already created in TestMain; updating must not return an api_key.
	resp := doRequest(t, http.MethodPut,
		baseURL+"/v1/config/namespaces/"+testNS, adminKey,
		map[string]any{
			"action_weights": map[string]float64{"click": 1.0, "like": 3.0},
			"max_results":    25,
			"dense_strategy": "byoe",
			"embedding_dim":  4,
			"alpha":          0.7,
		})

	var body struct {
		Namespace string    `json:"namespace"`
		UpdatedAt time.Time `json:"updated_at"`
		APIKey    string    `json:"api_key"`
	}
	decodeJSON(t, resp, &body)

	if body.Namespace != testNS {
		t.Errorf("namespace = %q, want %q", body.Namespace, testNS)
	}
	if body.UpdatedAt.IsZero() {
		t.Error("updated_at is zero")
	}
	if body.APIKey != "" {
		t.Errorf("api_key = %q on update, want empty (key only returned on first create)", body.APIKey)
	}
}
