package authz_test

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import (
	"net/http/httptest"
	"testing"

	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// testUserID returns a valid ObjectID hex string for tests.
func testUserID() string {
	return primitive.NewObjectID().Hex()
}

func TestIsSuperAdmin_True(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)
	req = auth.WithTestUser(req, &auth.SessionUser{
		ID:           testUserID(),
		Role:         "superadmin",
		IsSuperAdmin: true,
	})

	if !authz.IsSuperAdmin(req) {
		t.Error("expected IsSuperAdmin to return true for superadmin user")
	}
}

func TestIsSuperAdmin_False_Admin(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)
	req = auth.WithTestUser(req, &auth.SessionUser{
		ID:           testUserID(),
		Role:         "admin",
		IsSuperAdmin: false,
	})

	if authz.IsSuperAdmin(req) {
		t.Error("expected IsSuperAdmin to return false for admin user")
	}
}

func TestIsSuperAdmin_False_NoUser(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)

	if authz.IsSuperAdmin(req) {
		t.Error("expected IsSuperAdmin to return false when no user")
	}
}

func TestIsAdmin_True_ForAdmin(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)
	req = auth.WithTestUser(req, &auth.SessionUser{
		ID:   testUserID(),
		Role: "admin",
	})

	if !authz.IsAdmin(req) {
		t.Error("expected IsAdmin to return true for admin user")
	}
}

func TestIsAdmin_True_ForSuperAdmin(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)
	req = auth.WithTestUser(req, &auth.SessionUser{
		ID:           testUserID(),
		Role:         "superadmin",
		IsSuperAdmin: true,
	})

	if !authz.IsAdmin(req) {
		t.Error("expected IsAdmin to return true for superadmin user")
	}
}

func TestIsAdmin_False_ForMember(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)
	req = auth.WithTestUser(req, &auth.SessionUser{
		ID:   testUserID(),
		Role: "member",
	})

	if authz.IsAdmin(req) {
		t.Error("expected IsAdmin to return false for member user")
	}
}

func TestIsAdminOnly_True_ForAdmin(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)
	req = auth.WithTestUser(req, &auth.SessionUser{
		ID:   testUserID(),
		Role: "admin",
	})

	if !authz.IsAdminOnly(req) {
		t.Error("expected IsAdminOnly to return true for admin user")
	}
}

func TestIsAdminOnly_False_ForSuperAdmin(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)
	req = auth.WithTestUser(req, &auth.SessionUser{
		ID:           testUserID(),
		Role:         "superadmin",
		IsSuperAdmin: true,
	})

	if authz.IsAdminOnly(req) {
		t.Error("expected IsAdminOnly to return false for superadmin user")
	}
}

func TestCanManageResources_True_ForAdmin(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)
	req = auth.WithTestUser(req, &auth.SessionUser{
		ID:   testUserID(),
		Role: "admin",
	})

	if !authz.CanManageResources(req) {
		t.Error("expected CanManageResources to return true for admin")
	}
}

func TestCanManageResources_True_ForSuperAdmin(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)
	req = auth.WithTestUser(req, &auth.SessionUser{
		ID:           testUserID(),
		Role:         "superadmin",
		IsSuperAdmin: true,
	})

	if !authz.CanManageResources(req) {
		t.Error("expected CanManageResources to return true for superadmin")
	}
}

func TestCanManageResources_True_ForCoordinator_WithPermission(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)
	req = auth.WithTestUser(req, &auth.SessionUser{
		ID:                 testUserID(),
		Role:               "coordinator",
		CanManageResources: true,
	})

	if !authz.CanManageResources(req) {
		t.Error("expected CanManageResources to return true for coordinator with permission")
	}
}

func TestCanManageResources_False_ForCoordinator_NoPermission(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)
	req = auth.WithTestUser(req, &auth.SessionUser{
		ID:                 testUserID(),
		Role:               "coordinator",
		CanManageResources: false,
	})

	if authz.CanManageResources(req) {
		t.Error("expected CanManageResources to return false for coordinator without permission")
	}
}

func TestCanManageResources_False_ForMember(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)
	req = auth.WithTestUser(req, &auth.SessionUser{
		ID:   testUserID(),
		Role: "member",
	})

	if authz.CanManageResources(req) {
		t.Error("expected CanManageResources to return false for member")
	}
}

func TestCanManageMaterials_True_ForSuperAdmin(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)
	req = auth.WithTestUser(req, &auth.SessionUser{
		ID:           testUserID(),
		Role:         "superadmin",
		IsSuperAdmin: true,
	})

	if !authz.CanManageMaterials(req) {
		t.Error("expected CanManageMaterials to return true for superadmin")
	}
}

func TestUserCtx_ReturnsSuperAdmin(t *testing.T) {
	userID := testUserID()
	req := httptest.NewRequest("GET", "/test", nil)
	req = auth.WithTestUser(req, &auth.SessionUser{
		ID:           userID,
		Role:         "superadmin",
		IsSuperAdmin: true,
	})

	role, _, actorID, ok := authz.UserCtx(req)
	if !ok {
		t.Fatal("expected UserCtx to return ok=true")
	}
	if role != "superadmin" {
		t.Errorf("expected role 'superadmin', got %q", role)
	}
	if actorID.Hex() != userID {
		t.Errorf("expected actorID %s, got %s", userID, actorID.Hex())
	}
}
