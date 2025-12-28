package organizationstore_test

import (
	"testing"

	organizationstore "github.com/dalemusser/stratahub/internal/app/store/organizations"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestStore_Create(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := organizationstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org := models.Organization{
		Name:     "Test Organization",
		City:     "New York",
		State:    "NY",
		TimeZone: "America/New_York",
	}

	created, err := store.Create(ctx, org)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Verify ID was assigned
	if created.ID == primitive.NilObjectID {
		t.Error("expected ID to be assigned")
	}

	// Verify normalized fields
	if created.NameCI == "" {
		t.Error("expected NameCI to be set")
	}
	if created.CityCI == "" {
		t.Error("expected CityCI to be set")
	}
	if created.StateCI == "" {
		t.Error("expected StateCI to be set")
	}

	// Verify timestamps
	if created.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
	if created.UpdatedAt.IsZero() {
		t.Error("expected UpdatedAt to be set")
	}

	// Verify default status
	if created.Status != "active" {
		t.Errorf("expected status 'active', got %q", created.Status)
	}
}

func TestStore_Create_DuplicateName(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := organizationstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org := models.Organization{
		Name:     "Duplicate Test",
		City:     "Boston",
		State:    "MA",
		TimeZone: "America/New_York",
	}

	// Create first org
	_, err := store.Create(ctx, org)
	if err != nil {
		t.Fatalf("first Create failed: %v", err)
	}

	// Try to create duplicate
	_, err = store.Create(ctx, org)
	if err != organizationstore.ErrDuplicateOrganization {
		t.Errorf("expected ErrDuplicateOrganization, got %v", err)
	}
}

func TestStore_GetByID(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := organizationstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	// Create an organization first
	org := models.Organization{
		Name:        "GetByID Test",
		City:        "Chicago",
		State:       "IL",
		TimeZone:    "America/Chicago",
		ContactInfo: "test@example.com",
	}
	created, err := store.Create(ctx, org)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Retrieve by ID
	found, err := store.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	// Verify fields match
	if found.Name != created.Name {
		t.Errorf("Name: got %q, want %q", found.Name, created.Name)
	}
	if found.City != created.City {
		t.Errorf("City: got %q, want %q", found.City, created.City)
	}
	if found.State != created.State {
		t.Errorf("State: got %q, want %q", found.State, created.State)
	}
	if found.TimeZone != created.TimeZone {
		t.Errorf("TimeZone: got %q, want %q", found.TimeZone, created.TimeZone)
	}
	if found.ContactInfo != created.ContactInfo {
		t.Errorf("ContactInfo: got %q, want %q", found.ContactInfo, created.ContactInfo)
	}
}

func TestStore_GetByID_NotFound(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := organizationstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	// Try to get a non-existent organization
	fakeID := primitive.NewObjectID()
	_, err := store.GetByID(ctx, fakeID)
	if err != mongo.ErrNoDocuments {
		t.Errorf("expected mongo.ErrNoDocuments, got %v", err)
	}
}

func TestStore_Create_CaseInsensitiveFields(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := organizationstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org := models.Organization{
		Name:     "École Française",
		City:     "São Paulo",
		State:    "SP",
		TimeZone: "America/Sao_Paulo",
	}

	created, err := store.Create(ctx, org)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Verify case-insensitive fields are lowercase and diacritics-stripped
	if created.NameCI != "ecole francaise" {
		t.Errorf("NameCI: got %q, want %q", created.NameCI, "ecole francaise")
	}
	if created.CityCI != "sao paulo" {
		t.Errorf("CityCI: got %q, want %q", created.CityCI, "sao paulo")
	}
}
