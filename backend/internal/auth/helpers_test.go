package auth

import (
	"reflect"
	"testing"
	"time"
)

func TestNormalizeRoles(t *testing.T) {
	if got, ok := normalizeRoles(nil); got != nil || ok {
		t.Errorf("normalizeRoles(nil) = %v %v, want nil false", got, ok)
	}
	if got, ok := normalizeRoles([]string{"bogus"}); got != nil || ok {
		t.Errorf("normalizeRoles(invalid) = %v, %v", got, ok)
	}
	if got, ok := normalizeRoles([]string{"operations_admin", "viewer", "viewer", "system_admin"}); !ok || !reflect.DeepEqual(got, []string{"operations_admin", "system_admin", "viewer"}) {
		t.Errorf("normalizeRoles = %v %v, want sorted dedup", got, ok)
	}
}

func TestManagedUserViewAndAssignable(t *testing.T) {
	admin := User{Roles: []Role{{Code: SystemAdmin}}}
	if !isAssignable(admin) {
		t.Error("system admin not assignable")
	}
	viewer := User{Roles: []Role{{Code: Viewer}}}
	if isAssignable(viewer) {
		t.Error("viewer assignable")
	}
	now := time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
	user := User{ID: 7, Username: "alice", DisplayName: "Alice", Roles: []Role{{Code: Viewer}}, Status: "active", LastLoginAt: &now, CreatedAt: now, UpdatedAt: now}
	view := managedUserView(user)
	if view.ID != 7 || view.Username != "alice" || !reflect.DeepEqual(view.Roles, []string{"viewer"}) || view.Status != "active" {
		t.Errorf("managedUserView = %+v", view)
	}
}

func TestSessionFrom(t *testing.T) {
	user := User{ID: 3, Username: "bob", DisplayName: "Bob", Roles: []Role{{Code: Viewer}}}
	now := time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
	s := sessionFrom(user, "abc", now.Add(30*time.Minute), now, "raw-refresh")
	if s.AccessToken != "abc" || s.TokenType != "Bearer" || s.AccessTokenExpiresIn != 1800 {
		t.Errorf("sessionFrom = %+v", s)
	}
	if s.User.Username != "bob" || !reflect.DeepEqual(s.User.Roles, []string{"viewer"}) {
		t.Errorf("session user = %+v", s.User)
	}
	if s.refreshToken != "raw-refresh" {
		t.Errorf("refreshToken = %q", s.refreshToken)
	}
}

func TestHasRole(t *testing.T) {
	if !hasRole([]string{"system_admin", "viewer"}, "system_admin") {
		t.Error("hasRole present = false")
	}
	if hasRole([]string{"viewer"}, "system_admin") {
		t.Error("hasRole absent = true")
	}
	if hasRole(nil, "viewer") {
		t.Error("hasRole nil = true")
	}
}

func TestModelTableNames(t *testing.T) {
	if (User{}).TableName() == "" || (Role{}).TableName() == "" || (RefreshToken{}).TableName() == "" {
		t.Error("a TableName returned empty")
	}
}
