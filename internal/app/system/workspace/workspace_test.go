package workspace_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestFilter_AddsWorkspaceID(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	fixtures := testutil.NewFixtures(t, db)

	// Create a workspace
	ws := fixtures.CreateWorkspace(ctx, "Test Workspace", "test")

	// Create a mock request with workspace context
	req := httptest.NewRequest("GET", "/test", nil)
	req = workspace.WithTestWorkspace(req, ws.ID, "test", "Test Workspace")

	// Test Filter function
	filter := bson.M{"status": "active"}
	workspace.Filter(req, filter)

	// Verify workspace_id was added
	if filter["workspace_id"] != ws.ID {
		t.Errorf("expected workspace_id %s, got %v", ws.ID.Hex(), filter["workspace_id"])
	}

	// Verify original filter is preserved
	if filter["status"] != "active" {
		t.Errorf("expected status 'active', got %v", filter["status"])
	}
}

func TestFilter_NoWorkspaceContext(t *testing.T) {
	// Create a request without workspace context
	req := httptest.NewRequest("GET", "/test", nil)

	filter := bson.M{"status": "active"}
	workspace.Filter(req, filter)

	// Verify workspace_id was NOT added
	if _, exists := filter["workspace_id"]; exists {
		t.Error("expected workspace_id to not be added when no workspace context")
	}
}

func TestFilter_ApexDomain(t *testing.T) {
	// Create a request at apex domain (IsApex = true)
	req := httptest.NewRequest("GET", "/test", nil)
	req = workspace.WithTestApex(req)

	filter := bson.M{"status": "active"}
	workspace.Filter(req, filter)

	// Verify workspace_id was NOT added for apex domain
	if _, exists := filter["workspace_id"]; exists {
		t.Error("expected workspace_id to not be added at apex domain")
	}
}

func TestFilterCtx_AddsWorkspaceID(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	fixtures := testutil.NewFixtures(t, db)

	// Create a workspace
	ws := fixtures.CreateWorkspace(ctx, "Test Workspace", "test")

	// Add workspace to context
	ctxWithWs := workspace.WithTestWorkspaceCtx(ctx, ws.ID, "test", "Test Workspace")

	filter := bson.M{"status": "active"}
	workspace.FilterCtx(ctxWithWs, filter)

	// Verify workspace_id was added
	if filter["workspace_id"] != ws.ID {
		t.Errorf("expected workspace_id %s, got %v", ws.ID.Hex(), filter["workspace_id"])
	}
}

func TestMustFilter_ReturnsTrue(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	fixtures := testutil.NewFixtures(t, db)
	ws := fixtures.CreateWorkspace(ctx, "Test Workspace", "test")

	req := httptest.NewRequest("GET", "/test", nil)
	req = workspace.WithTestWorkspace(req, ws.ID, "test", "Test Workspace")

	filter := bson.M{}
	result := workspace.MustFilter(req, filter)

	if !result {
		t.Error("expected MustFilter to return true when workspace context exists")
	}
	if filter["workspace_id"] != ws.ID {
		t.Errorf("expected workspace_id %s, got %v", ws.ID.Hex(), filter["workspace_id"])
	}
}

func TestMustFilter_ReturnsFalse_NoContext(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)

	filter := bson.M{}
	result := workspace.MustFilter(req, filter)

	if result {
		t.Error("expected MustFilter to return false when no workspace context")
	}
}

func TestMustFilter_ReturnsFalse_ApexDomain(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)
	req = workspace.WithTestApex(req)

	filter := bson.M{}
	result := workspace.MustFilter(req, filter)

	if result {
		t.Error("expected MustFilter to return false at apex domain")
	}
}

func TestSetOnDoc_SetsWorkspaceID(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	fixtures := testutil.NewFixtures(t, db)
	ws := fixtures.CreateWorkspace(ctx, "Test Workspace", "test")

	req := httptest.NewRequest("GET", "/test", nil)
	req = workspace.WithTestWorkspace(req, ws.ID, "test", "Test Workspace")

	doc := make(map[string]interface{})
	resultID := workspace.SetOnDoc(req, doc)

	if resultID != ws.ID {
		t.Errorf("expected returned ID %s, got %s", ws.ID.Hex(), resultID.Hex())
	}
	if doc["workspace_id"] != ws.ID {
		t.Errorf("expected workspace_id %s in doc, got %v", ws.ID.Hex(), doc["workspace_id"])
	}
}

func TestSetOnDoc_NoContext(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)

	doc := make(map[string]interface{})
	resultID := workspace.SetOnDoc(req, doc)

	if resultID != primitive.NilObjectID {
		t.Errorf("expected NilObjectID, got %s", resultID.Hex())
	}
	if _, exists := doc["workspace_id"]; exists {
		t.Error("expected workspace_id to not be set when no context")
	}
}

func TestRequireWorkspace_Passes(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	fixtures := testutil.NewFixtures(t, db)
	ws := fixtures.CreateWorkspace(ctx, "Test Workspace", "test")

	called := false
	handler := workspace.RequireWorkspace(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req = workspace.WithTestWorkspace(req, ws.ID, "test", "Test Workspace")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("expected handler to be called when workspace context exists")
	}
}

func TestRequireWorkspace_Rejects_NoContext(t *testing.T) {
	called := false
	handler := workspace.RequireWorkspace(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if called {
		t.Error("expected handler to NOT be called when no workspace context")
	}
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rr.Code)
	}
}

func TestRequireWorkspace_Rejects_ApexDomain(t *testing.T) {
	called := false
	handler := workspace.RequireWorkspace(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req = workspace.WithTestApex(req)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if called {
		t.Error("expected handler to NOT be called at apex domain")
	}
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rr.Code)
	}
}

func TestRequireApex_Passes(t *testing.T) {
	called := false
	handler := workspace.RequireApex(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req = workspace.WithTestApex(req)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("expected handler to be called at apex domain")
	}
}

func TestRequireApex_Rejects_SubdomainContext(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	fixtures := testutil.NewFixtures(t, db)
	ws := fixtures.CreateWorkspace(ctx, "Test Workspace", "test")

	called := false
	handler := workspace.RequireApex(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req = workspace.WithTestWorkspace(req, ws.ID, "test", "Test Workspace")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if called {
		t.Error("expected handler to NOT be called when in subdomain context")
	}
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", rr.Code)
	}
}

func TestFromRequest_ReturnsInfo(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	fixtures := testutil.NewFixtures(t, db)
	ws := fixtures.CreateWorkspace(ctx, "Test Workspace", "test")

	req := httptest.NewRequest("GET", "/test", nil)
	req = workspace.WithTestWorkspace(req, ws.ID, "test", "Test Workspace")

	info := workspace.FromRequest(req)
	if info == nil {
		t.Fatal("expected Info, got nil")
	}
	if info.ID != ws.ID {
		t.Errorf("expected ID %s, got %s", ws.ID.Hex(), info.ID.Hex())
	}
	if info.Subdomain != "test" {
		t.Errorf("expected subdomain 'test', got %q", info.Subdomain)
	}
	if info.Name != "Test Workspace" {
		t.Errorf("expected name 'Test Workspace', got %q", info.Name)
	}
	if info.IsApex {
		t.Error("expected IsApex to be false")
	}
}

func TestFromRequest_ReturnsNil(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)

	info := workspace.FromRequest(req)
	if info != nil {
		t.Error("expected nil when no workspace context")
	}
}

func TestIDFromRequest_ReturnsID(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	fixtures := testutil.NewFixtures(t, db)
	ws := fixtures.CreateWorkspace(ctx, "Test Workspace", "test")

	req := httptest.NewRequest("GET", "/test", nil)
	req = workspace.WithTestWorkspace(req, ws.ID, "test", "Test Workspace")

	id := workspace.IDFromRequest(req)
	if id != ws.ID {
		t.Errorf("expected ID %s, got %s", ws.ID.Hex(), id.Hex())
	}
}

func TestIDFromRequest_ReturnsNilObjectID_NoContext(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)

	id := workspace.IDFromRequest(req)
	if id != primitive.NilObjectID {
		t.Errorf("expected NilObjectID, got %s", id.Hex())
	}
}

func TestIDFromRequest_ReturnsNilObjectID_ApexDomain(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)
	req = workspace.WithTestApex(req)

	id := workspace.IDFromRequest(req)
	if id != primitive.NilObjectID {
		t.Errorf("expected NilObjectID at apex domain, got %s", id.Hex())
	}
}
