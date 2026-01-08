package workspacestore_test

import (
	"testing"

	workspacestore "github.com/dalemusser/stratahub/internal/app/store/workspaces"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestStore_Create(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := workspacestore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	ws := models.Workspace{
		Name:      "Test Workspace",
		Subdomain: "test",
	}

	created, err := store.Create(ctx, ws)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if created.ID == primitive.NilObjectID {
		t.Error("expected ID to be assigned")
	}
	if created.NameCI == "" {
		t.Error("expected NameCI to be set")
	}
	if created.Status != "active" {
		t.Errorf("expected status 'active', got %q", created.Status)
	}
	if created.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
}

func TestStore_Create_DuplicateSubdomain(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := workspacestore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	// Note: indexes are already created by SetupTestDB

	ws1 := models.Workspace{Name: "Workspace 1", Subdomain: "duplicate"}
	_, err := store.Create(ctx, ws1)
	if err != nil {
		t.Fatalf("first Create failed: %v", err)
	}

	ws2 := models.Workspace{Name: "Workspace 2", Subdomain: "duplicate"}
	_, err = store.Create(ctx, ws2)
	if err == nil {
		t.Error("expected error for duplicate subdomain")
	}
}

func TestStore_GetByID(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := workspacestore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	ws := models.Workspace{Name: "Test Workspace", Subdomain: "test"}
	created, err := store.Create(ctx, ws)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	found, err := store.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if found.Name != created.Name {
		t.Errorf("Name: got %q, want %q", found.Name, created.Name)
	}
	if found.Subdomain != created.Subdomain {
		t.Errorf("Subdomain: got %q, want %q", found.Subdomain, created.Subdomain)
	}
}

func TestStore_GetByID_NotFound(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := workspacestore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	_, err := store.GetByID(ctx, primitive.NewObjectID())
	if err != workspacestore.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestStore_GetBySubdomain(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := workspacestore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	ws := models.Workspace{Name: "Test Workspace", Subdomain: "myapp"}
	created, err := store.Create(ctx, ws)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	found, err := store.GetBySubdomain(ctx, "myapp")
	if err != nil {
		t.Fatalf("GetBySubdomain failed: %v", err)
	}

	if found.ID != created.ID {
		t.Errorf("ID: got %s, want %s", found.ID.Hex(), created.ID.Hex())
	}
}

func TestStore_GetBySubdomain_NotFound(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := workspacestore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	_, err := store.GetBySubdomain(ctx, "nonexistent")
	if err != workspacestore.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestStore_EnsureDefault_CreatesNew(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := workspacestore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	ws, err := store.EnsureDefault(ctx, "Default Workspace", "default")
	if err != nil {
		t.Fatalf("EnsureDefault failed: %v", err)
	}

	if ws.Name != "Default Workspace" {
		t.Errorf("Name: got %q, want %q", ws.Name, "Default Workspace")
	}
	if ws.Subdomain != "default" {
		t.Errorf("Subdomain: got %q, want %q", ws.Subdomain, "default")
	}
	if ws.Status != "active" {
		t.Errorf("Status: got %q, want %q", ws.Status, "active")
	}
}

func TestStore_EnsureDefault_ReturnsExisting(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := workspacestore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	// Create first workspace
	existing := models.Workspace{Name: "Existing Workspace", Subdomain: "existing"}
	created, err := store.Create(ctx, existing)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// EnsureDefault should return the existing workspace
	ws, err := store.EnsureDefault(ctx, "Different Name", "different")
	if err != nil {
		t.Fatalf("EnsureDefault failed: %v", err)
	}

	if ws.ID != created.ID {
		t.Errorf("expected existing workspace ID %s, got %s", created.ID.Hex(), ws.ID.Hex())
	}
}

func TestStore_GetFirst(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := workspacestore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	// Create workspaces
	ws1 := models.Workspace{Name: "Workspace 1", Subdomain: "ws1"}
	created1, _ := store.Create(ctx, ws1)

	ws2 := models.Workspace{Name: "Workspace 2", Subdomain: "ws2"}
	store.Create(ctx, ws2)

	// GetFirst should return the first created
	first, err := store.GetFirst(ctx)
	if err != nil {
		t.Fatalf("GetFirst failed: %v", err)
	}

	if first.ID != created1.ID {
		t.Errorf("expected first workspace ID %s, got %s", created1.ID.Hex(), first.ID.Hex())
	}
}

func TestStore_GetFirst_NotFound(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := workspacestore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	_, err := store.GetFirst(ctx)
	if err != workspacestore.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestStore_Update(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := workspacestore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	ws := models.Workspace{Name: "Original Name", Subdomain: "original"}
	created, err := store.Create(ctx, ws)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Update the workspace
	err = store.Update(ctx, created.ID, models.Workspace{
		Name:   "Updated Name",
		Status: "suspended",
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Verify update
	updated, err := store.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if updated.Name != "Updated Name" {
		t.Errorf("Name: got %q, want %q", updated.Name, "Updated Name")
	}
	if updated.Status != "suspended" {
		t.Errorf("Status: got %q, want %q", updated.Status, "suspended")
	}
}

func TestStore_Delete(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := workspacestore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	ws := models.Workspace{Name: "To Delete", Subdomain: "delete"}
	created, err := store.Create(ctx, ws)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	deleted, err := store.Delete(ctx, created.ID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if deleted != 1 {
		t.Errorf("expected 1 deleted, got %d", deleted)
	}

	// Verify deletion
	_, err = store.GetByID(ctx, created.ID)
	if err != workspacestore.ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestStore_Count(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := workspacestore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	// Initial count should be 0
	count, err := store.Count(ctx, nil)
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected count 0, got %d", count)
	}

	// Create workspaces
	store.Create(ctx, models.Workspace{Name: "WS 1", Subdomain: "ws1"})
	store.Create(ctx, models.Workspace{Name: "WS 2", Subdomain: "ws2"})
	store.Create(ctx, models.Workspace{Name: "WS 3", Subdomain: "ws3", Status: "suspended"})

	// Total count
	count, err = store.Count(ctx, nil)
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != 3 {
		t.Errorf("expected count 3, got %d", count)
	}
}
