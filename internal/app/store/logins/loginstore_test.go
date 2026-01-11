package loginstore_test

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import (
	"testing"
	"time"

	loginstore "github.com/dalemusser/stratahub/internal/app/store/logins"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestStore_Create(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := loginstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	userID := primitive.NewObjectID()
	rec := models.LoginRecord{
		UserID:   userID.String(),
		IP:       "192.168.1.1",
		Provider: "internal",
	}

	err := store.Create(ctx, rec)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Verify the record was inserted
	var found models.LoginRecord
	err = db.Collection("login_records").FindOne(ctx, bson.M{"user_id": userID.String()}).Decode(&found)
	if err != nil {
		t.Fatalf("failed to find login record: %v", err)
	}

	if found.UserID != userID.String() {
		t.Errorf("UserID: got %q, want %q", found.UserID, userID.String())
	}
	if found.IP != "192.168.1.1" {
		t.Errorf("IP: got %q, want %q", found.IP, "192.168.1.1")
	}
	if found.Provider != "internal" {
		t.Errorf("Provider: got %q, want %q", found.Provider, "internal")
	}
	// CreatedAt should be set automatically
	if found.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
}

func TestStore_Create_WithExplicitTimestamp(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := loginstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	userID := primitive.NewObjectID()
	customTime := time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC)
	rec := models.LoginRecord{
		UserID:    userID.String(),
		CreatedAt: customTime,
		IP:        "10.0.0.1",
		Provider:  "google",
	}

	err := store.Create(ctx, rec)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Verify the record preserves the explicit timestamp
	var found models.LoginRecord
	err = db.Collection("login_records").FindOne(ctx, bson.M{"user_id": userID.String()}).Decode(&found)
	if err != nil {
		t.Fatalf("failed to find login record: %v", err)
	}

	if !found.CreatedAt.Equal(customTime) {
		t.Errorf("CreatedAt: got %v, want %v", found.CreatedAt, customTime)
	}
}

func TestStore_Create_MultipleRecordsSameUser(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := loginstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	userID := primitive.NewObjectID()

	// Create multiple login records for same user
	for i := 0; i < 3; i++ {
		rec := models.LoginRecord{
			UserID:   userID.String(),
			IP:       "192.168.1.1",
			Provider: "internal",
		}
		err := store.Create(ctx, rec)
		if err != nil {
			t.Fatalf("Create %d failed: %v", i, err)
		}
	}

	// Verify all records were inserted
	count, err := db.Collection("login_records").CountDocuments(ctx, bson.M{"user_id": userID.String()})
	if err != nil {
		t.Fatalf("CountDocuments failed: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 login records, got %d", count)
	}
}
