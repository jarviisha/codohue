//go:build e2e

package e2e

import (
	"net/http"
	"testing"
)

func TestDataPlaneNamespaceMutationRouteRemoved_NoToken(t *testing.T) {
	resp := doRequest(t, http.MethodPut,
		baseURL+"/v1/config/namespaces/"+testNS, "",
		map[string]any{"action_weights": map[string]float64{"click": 1.0}})
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

func TestDataPlaneNamespaceMutationRouteRemoved_WrongKey(t *testing.T) {
	resp := doRequest(t, http.MethodPut,
		baseURL+"/v1/config/namespaces/"+testNS, "totally-wrong-key",
		map[string]any{"action_weights": map[string]float64{"click": 1.0}})
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

func TestDataPlaneNamespaceMutationRouteRemoved_AdminKey(t *testing.T) {
	resp := doRequest(t, http.MethodPut,
		baseURL+"/v1/config/namespaces/"+testNS, adminKey,
		map[string]any{
			"action_weights": map[string]float64{"click": 1.0, "like": 3.0},
			"max_results":    25,
			"dense_strategy": "byoe",
			"embedding_dim":  4,
			"alpha":          0.7,
		})
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}
