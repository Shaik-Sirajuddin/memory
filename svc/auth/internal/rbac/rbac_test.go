package rbac

import (
	"testing"

	"example.com/m/v2/internal/domain"
)

func TestAllowed(t *testing.T) {
	m := NewMatcher()
	scopes := []domain.Scope{{Root: "team", Path: "/store/*", Scope: "read"}}
	if !m.Allowed("/store/get", scopes, "team", "GET") {
		t.Fatalf("expected allowed")
	}
	if m.Allowed("/store/get", scopes, "user", "GET") {
		t.Fatalf("expected denied for root mismatch")
	}
}
