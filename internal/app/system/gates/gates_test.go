package gates_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/app/system/gates"
)

// Helper to create a request with user context
func withTestUser(r *http.Request, role string) *http.Request {
	user := &auth.SessionUser{
		ID:      "507f1f77bcf86cd799439011", // Valid ObjectID hex
		Name:    "Test User",
		LoginID: "test@example.com",
		Role:    role,
	}
	return auth.WithTestUser(r, user)
}

// Test RequireAuth

func TestRequireAuth_Authenticated(t *testing.T) {
	req := httptest.NewRequest("GET", "/protected", nil)
	req = withTestUser(req, "admin")
	rec := httptest.NewRecorder()

	result := gates.RequireAuth(rec, req, "/login")

	if !result.OK {
		t.Error("expected OK to be true for authenticated user")
	}
	if result.Role != "admin" {
		t.Errorf("Role: got %q, want %q", result.Role, "admin")
	}
	if result.Name != "Test User" {
		t.Errorf("Name: got %q, want %q", result.Name, "Test User")
	}
	if result.UserID.IsZero() {
		t.Error("expected UserID to be set")
	}
}

func TestRequireAuth_NotAuthenticated(t *testing.T) {
	req := httptest.NewRequest("GET", "/protected", nil)
	rec := httptest.NewRecorder()

	result := gates.RequireAuth(rec, req, "/login")

	if result.OK {
		t.Error("expected OK to be false for unauthenticated user")
	}
}

// Test RequireAdmin

func TestRequireAdmin_AsAdmin(t *testing.T) {
	req := httptest.NewRequest("GET", "/admin/dashboard", nil)
	req = withTestUser(req, "admin")
	rec := httptest.NewRecorder()

	result := gates.RequireAdmin(rec, req, "Admin only", "/")

	if !result.OK {
		t.Error("expected OK to be true for admin user")
	}
	if result.Role != "admin" {
		t.Errorf("Role: got %q, want %q", result.Role, "admin")
	}
}

func TestRequireAdmin_NotAuthenticated(t *testing.T) {
	req := httptest.NewRequest("GET", "/admin/dashboard", nil)
	rec := httptest.NewRecorder()

	result := gates.RequireAdmin(rec, req, "Admin only", "/")

	if result.OK {
		t.Error("expected OK to be false for unauthenticated user")
	}
}

func TestRequireAdmin_WrongRole_Leader(t *testing.T) {
	req := httptest.NewRequest("GET", "/admin/dashboard", nil)
	req = withTestUser(req, "leader")
	rec := httptest.NewRecorder()

	result := gates.RequireAdmin(rec, req, "Admin only", "/")

	if result.OK {
		t.Error("expected OK to be false for leader user")
	}
}

func TestRequireAdmin_WrongRole_Member(t *testing.T) {
	req := httptest.NewRequest("GET", "/admin/dashboard", nil)
	req = withTestUser(req, "member")
	rec := httptest.NewRecorder()

	result := gates.RequireAdmin(rec, req, "Admin only", "/")

	if result.OK {
		t.Error("expected OK to be false for member user")
	}
}

// Test RequireAdminOrAnalyst

func TestRequireAdminOrAnalyst_AsAdmin(t *testing.T) {
	req := httptest.NewRequest("GET", "/analytics", nil)
	req = withTestUser(req, "admin")
	rec := httptest.NewRecorder()

	result := gates.RequireAdminOrAnalyst(rec, req, "Admin or Analyst only", "/")

	if !result.OK {
		t.Error("expected OK to be true for admin user")
	}
	if result.Role != "admin" {
		t.Errorf("Role: got %q, want %q", result.Role, "admin")
	}
}

func TestRequireAdminOrAnalyst_AsAnalyst(t *testing.T) {
	req := httptest.NewRequest("GET", "/analytics", nil)
	req = withTestUser(req, "analyst")
	rec := httptest.NewRecorder()

	result := gates.RequireAdminOrAnalyst(rec, req, "Admin or Analyst only", "/")

	if !result.OK {
		t.Error("expected OK to be true for analyst user")
	}
	if result.Role != "analyst" {
		t.Errorf("Role: got %q, want %q", result.Role, "analyst")
	}
}

func TestRequireAdminOrAnalyst_NotAuthenticated(t *testing.T) {
	req := httptest.NewRequest("GET", "/analytics", nil)
	rec := httptest.NewRecorder()

	result := gates.RequireAdminOrAnalyst(rec, req, "Admin or Analyst only", "/")

	if result.OK {
		t.Error("expected OK to be false for unauthenticated user")
	}
}

func TestRequireAdminOrAnalyst_WrongRole_Leader(t *testing.T) {
	req := httptest.NewRequest("GET", "/analytics", nil)
	req = withTestUser(req, "leader")
	rec := httptest.NewRecorder()

	result := gates.RequireAdminOrAnalyst(rec, req, "Admin or Analyst only", "/")

	if result.OK {
		t.Error("expected OK to be false for leader user")
	}
}

func TestRequireAdminOrAnalyst_WrongRole_Member(t *testing.T) {
	req := httptest.NewRequest("GET", "/analytics", nil)
	req = withTestUser(req, "member")
	rec := httptest.NewRecorder()

	result := gates.RequireAdminOrAnalyst(rec, req, "Admin or Analyst only", "/")

	if result.OK {
		t.Error("expected OK to be false for member user")
	}
}

// Test RequireAdminOrLeader

func TestRequireAdminOrLeader_AsAdmin(t *testing.T) {
	req := httptest.NewRequest("GET", "/group/manage", nil)
	req = withTestUser(req, "admin")
	rec := httptest.NewRecorder()

	result := gates.RequireAdminOrLeader(rec, req, "Admin or Leader only", "/")

	if !result.OK {
		t.Error("expected OK to be true for admin user")
	}
	if result.Role != "admin" {
		t.Errorf("Role: got %q, want %q", result.Role, "admin")
	}
}

func TestRequireAdminOrLeader_AsLeader(t *testing.T) {
	req := httptest.NewRequest("GET", "/group/manage", nil)
	req = withTestUser(req, "leader")
	rec := httptest.NewRecorder()

	result := gates.RequireAdminOrLeader(rec, req, "Admin or Leader only", "/")

	if !result.OK {
		t.Error("expected OK to be true for leader user")
	}
	if result.Role != "leader" {
		t.Errorf("Role: got %q, want %q", result.Role, "leader")
	}
}

func TestRequireAdminOrLeader_NotAuthenticated(t *testing.T) {
	req := httptest.NewRequest("GET", "/group/manage", nil)
	rec := httptest.NewRecorder()

	result := gates.RequireAdminOrLeader(rec, req, "Admin or Leader only", "/")

	if result.OK {
		t.Error("expected OK to be false for unauthenticated user")
	}
}

func TestRequireAdminOrLeader_WrongRole_Analyst(t *testing.T) {
	req := httptest.NewRequest("GET", "/group/manage", nil)
	req = withTestUser(req, "analyst")
	rec := httptest.NewRecorder()

	result := gates.RequireAdminOrLeader(rec, req, "Admin or Leader only", "/")

	if result.OK {
		t.Error("expected OK to be false for analyst user")
	}
}

func TestRequireAdminOrLeader_WrongRole_Member(t *testing.T) {
	req := httptest.NewRequest("GET", "/group/manage", nil)
	req = withTestUser(req, "member")
	rec := httptest.NewRecorder()

	result := gates.RequireAdminOrLeader(rec, req, "Admin or Leader only", "/")

	if result.OK {
		t.Error("expected OK to be false for member user")
	}
}

// Test RequireAnyRole

func TestRequireAnyRole_FirstRoleAllowed(t *testing.T) {
	req := httptest.NewRequest("GET", "/content", nil)
	req = withTestUser(req, "admin")
	rec := httptest.NewRecorder()

	result := gates.RequireAnyRole(rec, req, "Access denied", "/", "admin", "leader", "coordinator")

	if !result.OK {
		t.Error("expected OK to be true for admin user")
	}
	if result.Role != "admin" {
		t.Errorf("Role: got %q, want %q", result.Role, "admin")
	}
}

func TestRequireAnyRole_MiddleRoleAllowed(t *testing.T) {
	req := httptest.NewRequest("GET", "/content", nil)
	req = withTestUser(req, "leader")
	rec := httptest.NewRecorder()

	result := gates.RequireAnyRole(rec, req, "Access denied", "/", "admin", "leader", "coordinator")

	if !result.OK {
		t.Error("expected OK to be true for leader user")
	}
	if result.Role != "leader" {
		t.Errorf("Role: got %q, want %q", result.Role, "leader")
	}
}

func TestRequireAnyRole_LastRoleAllowed(t *testing.T) {
	req := httptest.NewRequest("GET", "/content", nil)
	req = withTestUser(req, "coordinator")
	rec := httptest.NewRecorder()

	result := gates.RequireAnyRole(rec, req, "Access denied", "/", "admin", "leader", "coordinator")

	if !result.OK {
		t.Error("expected OK to be true for coordinator user")
	}
	if result.Role != "coordinator" {
		t.Errorf("Role: got %q, want %q", result.Role, "coordinator")
	}
}

func TestRequireAnyRole_NotAuthenticated(t *testing.T) {
	req := httptest.NewRequest("GET", "/content", nil)
	rec := httptest.NewRecorder()

	result := gates.RequireAnyRole(rec, req, "Access denied", "/", "admin", "leader")

	if result.OK {
		t.Error("expected OK to be false for unauthenticated user")
	}
}

func TestRequireAnyRole_RoleNotAllowed(t *testing.T) {
	req := httptest.NewRequest("GET", "/content", nil)
	req = withTestUser(req, "member")
	rec := httptest.NewRecorder()

	result := gates.RequireAnyRole(rec, req, "Access denied", "/", "admin", "leader")

	if result.OK {
		t.Error("expected OK to be false for member user when only admin/leader allowed")
	}
}

func TestRequireAnyRole_SingleRoleAllowed(t *testing.T) {
	req := httptest.NewRequest("GET", "/admin-only", nil)
	req = withTestUser(req, "admin")
	rec := httptest.NewRecorder()

	result := gates.RequireAnyRole(rec, req, "Admin only", "/", "admin")

	if !result.OK {
		t.Error("expected OK to be true for admin user with single role allowed")
	}
}

func TestRequireAnyRole_SingleRoleNotAllowed(t *testing.T) {
	req := httptest.NewRequest("GET", "/admin-only", nil)
	req = withTestUser(req, "leader")
	rec := httptest.NewRecorder()

	result := gates.RequireAnyRole(rec, req, "Admin only", "/", "admin")

	if result.OK {
		t.Error("expected OK to be false for leader user with only admin allowed")
	}
}

// Test role case normalization (authz.UserCtx lowercases roles)

func TestRequireAdmin_CaseInsensitive(t *testing.T) {
	// Note: authz.UserCtx normalizes role to lowercase
	req := httptest.NewRequest("GET", "/admin/dashboard", nil)
	// Even if stored as "Admin", authz.UserCtx returns "admin"
	req = withTestUser(req, "admin")
	rec := httptest.NewRecorder()

	result := gates.RequireAdmin(rec, req, "Admin only", "/")

	if !result.OK {
		t.Error("expected OK to be true for admin user")
	}
}

// Test that Result contains correct user info

func TestRequireAuth_ReturnsCorrectUserInfo(t *testing.T) {
	req := httptest.NewRequest("GET", "/protected", nil)
	user := &auth.SessionUser{
		ID:      "507f1f77bcf86cd799439011",
		Name:    "John Smith",
		LoginID: "jsmith@example.com",
		Role:    "leader",
	}
	req = auth.WithTestUser(req, user)
	rec := httptest.NewRecorder()

	result := gates.RequireAuth(rec, req, "/login")

	if !result.OK {
		t.Fatal("expected OK to be true")
	}
	if result.Name != "John Smith" {
		t.Errorf("Name: got %q, want %q", result.Name, "John Smith")
	}
	if result.Role != "leader" {
		t.Errorf("Role: got %q, want %q", result.Role, "leader")
	}
	if result.UserID.Hex() != "507f1f77bcf86cd799439011" {
		t.Errorf("UserID: got %q, want %q", result.UserID.Hex(), "507f1f77bcf86cd799439011")
	}
}
