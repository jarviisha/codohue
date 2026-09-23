//go:build e2e

package e2e

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/jarviisha/codohue/internal/core/namespace"
)

func TestAdmin_GroupConfiguration(t *testing.T) {
	cookie := adminLogin(t)
	path := "/api/admin/v1/namespaces/e2e_group_configuration"
	create := adminRequest(t, http.MethodPut, path, cookie, map[string]any{})
	create.Body.Close()
	if create.StatusCode != 200 && create.StatusCode != 201 {
		t.Fatalf("create=%d", create.StatusCode)
	}
	path += "/configuration"
	read := adminRequest(t, http.MethodGet, path, cookie, nil)
	var config namespace.Configuration
	err := json.NewDecoder(read.Body).Decode(&config)
	read.Body.Close()
	if err != nil || read.StatusCode != 200 {
		t.Fatalf("read=%d err=%v", read.StatusCode, err)
	}
	var previousTTL int
	if err := json.Unmarshal(config.Groups["trending"].Values["trending_ttl"], &previousTTL); err != nil {
		t.Fatal(err)
	}
	nextTTL := previousTTL + 1
	request := map[string]any{"group": "trending", "generation": config.Generation, "base_revision": config.Groups["trending"].Revision, "changes": map[string]any{"trending_ttl": nextTTL}}
	unauth := adminRequest(t, http.MethodPatch, path, nil, request)
	unauth.Body.Close()
	if unauth.StatusCode != 401 {
		t.Fatalf("unauth=%d", unauth.StatusCode)
	}
	validation := adminRequest(t, http.MethodPost, path+"/validation", cookie, request)
	validation.Body.Close()
	if validation.StatusCode != 200 {
		t.Fatalf("validate=%d", validation.StatusCode)
	}
	save := adminRequest(t, http.MethodPatch, path, cookie, request)
	var saved namespace.Configuration
	err = json.NewDecoder(save.Body).Decode(&saved)
	save.Body.Close()
	if err != nil || save.StatusCode != 200 || string(saved.Groups["trending"].Values["trending_ttl"]) != strconv.Itoa(nextTTL) {
		t.Fatalf("save=%d err=%v", save.StatusCode, err)
	}
	conflict := adminRequest(t, http.MethodPatch, path, cookie, request)
	conflict.Body.Close()
	if conflict.StatusCode != 409 {
		t.Fatalf("conflict=%d", conflict.StatusCode)
	}
	request["group"] = "recommendations"
	request["base_revision"] = config.Groups["recommendations"].Revision
	request["changes"] = map[string]any{"max_results": 37}
	other := adminRequest(t, http.MethodPatch, path, cookie, request)
	other.Body.Close()
	if other.StatusCode != 200 {
		t.Fatalf("independent group=%d", other.StatusCode)
	}
	request["changes"] = map[string]any{"trending_ttl": 999}
	invalid := adminRequest(t, http.MethodPatch, path, cookie, request)
	invalid.Body.Close()
	if invalid.StatusCode != 422 {
		t.Fatalf("wrong group=%d", invalid.StatusCode)
	}
}
