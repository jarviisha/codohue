package access

import (
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestPasswordHash(t *testing.T) {
	for _, password := range []string{"short", strings.Repeat("a", 73)} {
		if _, err := passwordHash(password); err == nil {
			t.Fatal("invalid password accepted")
		}
	}
	hash, err := passwordHash("a-long-test-password")
	if err != nil {
		t.Fatal(err)
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte("a-long-test-password")) != nil {
		t.Fatal("password cannot be verified")
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte("different-password")) == nil {
		t.Fatal("wrong password accepted")
	}
}
func TestServiceCapabilities(t *testing.T) {
	consumer := Actor{Role: "service", Permissions: []string{"data:read"}, Namespaces: []string{"a"}}
	for _, tc := range []struct {
		permission, namespace string
		want                  bool
	}{{"data:read", "a", true}, {"data:read", "b", false}, {"data:write", "a", false}, {"admin:read", "a", false}, {"admin:write", "", false}} {
		if got := consumer.Allows(tc.permission, tc.namespace); got != tc.want {
			t.Errorf("%s %s: %v", tc.permission, tc.namespace, got)
		}
	}
	if (Actor{Role: "admin"}).Allows("account:write", "") {
		t.Fatal("admin can manage accounts")
	}
}
func TestRandomTokenAndSpec(t *testing.T) {
	a, err := RandomToken()
	if err != nil {
		t.Fatal(err)
	}
	b, err := RandomToken()
	if err != nil {
		t.Fatal(err)
	}
	if a == b || Digest(a) == a || len(a) != 64 {
		t.Fatal("invalid random token")
	}
	if err := ValidateTokenSpec(a, []string{"data:read"}, []string{"a"}); err != nil {
		t.Fatal(err)
	}
	for _, ps := range [][]string{nil, {"owner"}, {"unknown"}} {
		if ValidateTokenSpec(a, ps, []string{"a"}) == nil {
			t.Fatal("invalid permission accepted")
		}
	}
	if ValidateTokenSpec("weak-key", []string{"data:read"}, []string{"a"}) == nil {
		t.Fatal("weak token accepted")
	}
}
