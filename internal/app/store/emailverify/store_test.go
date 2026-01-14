package emailverify_test

import (
	"testing"
	"time"

	"github.com/dalemusser/stratahub/internal/app/store/emailverify"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestNew_DefaultExpiry(t *testing.T) {
	db := testutil.SetupTestDB(t)

	// Zero expiry should use default
	store := emailverify.New(db, 0)
	if store.Expiry() != emailverify.DefaultExpiry {
		t.Errorf("expected default expiry %v, got %v", emailverify.DefaultExpiry, store.Expiry())
	}
}

func TestNew_CustomExpiry(t *testing.T) {
	db := testutil.SetupTestDB(t)

	customExpiry := 30 * time.Minute
	store := emailverify.New(db, customExpiry)
	if store.Expiry() != customExpiry {
		t.Errorf("expected expiry %v, got %v", customExpiry, store.Expiry())
	}
}

func TestNew_NegativeExpiry(t *testing.T) {
	db := testutil.SetupTestDB(t)

	// Negative expiry should use default
	store := emailverify.New(db, -1*time.Minute)
	if store.Expiry() != emailverify.DefaultExpiry {
		t.Errorf("expected default expiry %v, got %v", emailverify.DefaultExpiry, store.Expiry())
	}
}

func TestStore_Create(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := emailverify.New(db, emailverify.DefaultExpiry)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	userID := primitive.NewObjectID()
	email := "test@example.com"

	result, err := store.Create(ctx, userID, email, false)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if result.Code == "" {
		t.Error("expected code to be generated")
	}
	if len(result.Code) != emailverify.CodeLength {
		t.Errorf("expected code length %d, got %d", emailverify.CodeLength, len(result.Code))
	}
	if result.Token == "" {
		t.Error("expected token to be generated")
	}
	if result.ResendCount != 0 {
		t.Errorf("expected resend count 0, got %d", result.ResendCount)
	}
}

func TestStore_Create_CodeFormat(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := emailverify.New(db, emailverify.DefaultExpiry)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	userID := primitive.NewObjectID()
	email := "test@example.com"

	result, err := store.Create(ctx, userID, email, false)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Code should be 6 digits
	for _, c := range result.Code {
		if c < '0' || c > '9' {
			t.Errorf("code should be numeric, got %q", result.Code)
			break
		}
	}
}

func TestStore_Create_ReplacesExisting(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := emailverify.New(db, emailverify.DefaultExpiry)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	userID := primitive.NewObjectID()
	email := "test@example.com"

	// Create first verification
	result1, err := store.Create(ctx, userID, email, false)
	if err != nil {
		t.Fatalf("First Create failed: %v", err)
	}

	// Create second verification
	result2, err := store.Create(ctx, userID, email, false)
	if err != nil {
		t.Fatalf("Second Create failed: %v", err)
	}

	// Old code should not work
	_, err = store.VerifyCode(ctx, userID, result1.Code)
	if err != emailverify.ErrNotFound && err != emailverify.ErrInvalidCode {
		t.Errorf("expected old code to fail, got err=%v", err)
	}

	// New code should work
	v, err := store.VerifyCode(ctx, userID, result2.Code)
	if err != nil {
		t.Errorf("new code verification failed: %v", err)
	}
	if v == nil {
		t.Error("expected verification record")
	}
}

func TestStore_VerifyCode(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := emailverify.New(db, emailverify.DefaultExpiry)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	userID := primitive.NewObjectID()
	email := "test@example.com"

	result, err := store.Create(ctx, userID, email, false)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Verify with correct code
	v, err := store.VerifyCode(ctx, userID, result.Code)
	if err != nil {
		t.Fatalf("VerifyCode failed: %v", err)
	}

	if v.UserID != userID {
		t.Errorf("expected userID %v, got %v", userID, v.UserID)
	}
	if v.Email != email {
		t.Errorf("expected email %q, got %q", email, v.Email)
	}
}

func TestStore_VerifyCode_InvalidCode(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := emailverify.New(db, emailverify.DefaultExpiry)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	userID := primitive.NewObjectID()
	email := "test@example.com"

	_, err := store.Create(ctx, userID, email, false)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Verify with wrong code
	_, err = store.VerifyCode(ctx, userID, "000000")
	if err != emailverify.ErrInvalidCode {
		t.Errorf("expected ErrInvalidCode, got %v", err)
	}
}

func TestStore_VerifyCode_NotFound(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := emailverify.New(db, emailverify.DefaultExpiry)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	userID := primitive.NewObjectID()

	// Verify without creating
	_, err := store.VerifyCode(ctx, userID, "123456")
	if err != emailverify.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestStore_VerifyCode_SingleUse(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := emailverify.New(db, emailverify.DefaultExpiry)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	userID := primitive.NewObjectID()
	email := "test@example.com"

	result, err := store.Create(ctx, userID, email, false)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// First verification should succeed
	_, err = store.VerifyCode(ctx, userID, result.Code)
	if err != nil {
		t.Fatalf("First VerifyCode failed: %v", err)
	}

	// Second verification should fail (record deleted)
	_, err = store.VerifyCode(ctx, userID, result.Code)
	if err != emailverify.ErrNotFound {
		t.Errorf("expected ErrNotFound for reused code, got %v", err)
	}
}

func TestStore_VerifyCode_TooManyAttempts(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := emailverify.New(db, emailverify.DefaultExpiry)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	userID := primitive.NewObjectID()
	email := "test@example.com"

	_, err := store.Create(ctx, userID, email, false)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Make MaxVerifyAttempts failed attempts
	for i := 0; i < emailverify.MaxVerifyAttempts; i++ {
		_, err = store.VerifyCode(ctx, userID, "000000")
		if err != emailverify.ErrInvalidCode {
			t.Errorf("attempt %d: expected ErrInvalidCode, got %v", i+1, err)
		}
	}

	// Next attempt should be blocked
	_, err = store.VerifyCode(ctx, userID, "123456")
	if err != emailverify.ErrTooManyAttempts {
		t.Errorf("expected ErrTooManyAttempts, got %v", err)
	}
}

func TestStore_VerifyToken(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := emailverify.New(db, emailverify.DefaultExpiry)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	userID := primitive.NewObjectID()
	email := "test@example.com"

	result, err := store.Create(ctx, userID, email, false)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Verify with token
	v, err := store.VerifyToken(ctx, result.Token)
	if err != nil {
		t.Fatalf("VerifyToken failed: %v", err)
	}

	if v.UserID != userID {
		t.Errorf("expected userID %v, got %v", userID, v.UserID)
	}
	if v.Email != email {
		t.Errorf("expected email %q, got %q", email, v.Email)
	}
}

func TestStore_VerifyToken_NotFound(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := emailverify.New(db, emailverify.DefaultExpiry)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	// Verify with non-existent token
	_, err := store.VerifyToken(ctx, "invalid-token")
	if err != emailverify.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestStore_VerifyToken_SingleUse(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := emailverify.New(db, emailverify.DefaultExpiry)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	userID := primitive.NewObjectID()
	email := "test@example.com"

	result, err := store.Create(ctx, userID, email, false)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// First verification should succeed
	_, err = store.VerifyToken(ctx, result.Token)
	if err != nil {
		t.Fatalf("First VerifyToken failed: %v", err)
	}

	// Second verification should fail (record deleted)
	_, err = store.VerifyToken(ctx, result.Token)
	if err != emailverify.ErrNotFound {
		t.Errorf("expected ErrNotFound for reused token, got %v", err)
	}
}

func TestStore_DeleteByUser(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := emailverify.New(db, emailverify.DefaultExpiry)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	userID := primitive.NewObjectID()
	email := "test@example.com"

	result, err := store.Create(ctx, userID, email, false)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Delete by user
	err = store.DeleteByUser(ctx, userID)
	if err != nil {
		t.Fatalf("DeleteByUser failed: %v", err)
	}

	// Verification should fail
	_, err = store.VerifyCode(ctx, userID, result.Code)
	if err != emailverify.ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestStore_DeleteByUser_NoRecords(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := emailverify.New(db, emailverify.DefaultExpiry)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	userID := primitive.NewObjectID()

	// Delete non-existent user should not error
	err := store.DeleteByUser(ctx, userID)
	if err != nil {
		t.Fatalf("DeleteByUser should not error for non-existent user: %v", err)
	}
}

func TestStore_Create_Resend(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := emailverify.New(db, emailverify.DefaultExpiry)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	userID := primitive.NewObjectID()
	email := "test@example.com"

	// First create
	result1, err := store.Create(ctx, userID, email, false)
	if err != nil {
		t.Fatalf("First Create failed: %v", err)
	}
	if result1.ResendCount != 0 {
		t.Errorf("expected resend count 0, got %d", result1.ResendCount)
	}

	// Resend
	result2, err := store.Create(ctx, userID, email, true)
	if err != nil {
		t.Fatalf("Resend Create failed: %v", err)
	}
	if result2.ResendCount != 1 {
		t.Errorf("expected resend count 1, got %d", result2.ResendCount)
	}

	// New code should be different
	if result2.Code == result1.Code {
		t.Error("expected new code on resend")
	}
}

func TestStore_Create_ResendRateLimit(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := emailverify.New(db, emailverify.DefaultExpiry)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	userID := primitive.NewObjectID()
	email := "test@example.com"

	// First create
	_, err := store.Create(ctx, userID, email, false)
	if err != nil {
		t.Fatalf("First Create failed: %v", err)
	}

	// Resend up to the limit
	for i := 0; i < emailverify.MaxResends; i++ {
		_, err = store.Create(ctx, userID, email, true)
		if err != nil {
			t.Fatalf("Resend %d failed: %v", i+1, err)
		}
	}

	// Next resend should fail
	_, err = store.Create(ctx, userID, email, true)
	if err != emailverify.ErrTooManyResends {
		t.Errorf("expected ErrTooManyResends, got %v", err)
	}
}

func TestStore_EnsureIndexes(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := emailverify.New(db, emailverify.DefaultExpiry)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	// EnsureIndexes should not error
	err := store.EnsureIndexes(ctx)
	if err != nil {
		t.Fatalf("EnsureIndexes failed: %v", err)
	}

	// Calling again should be idempotent
	err = store.EnsureIndexes(ctx)
	if err != nil {
		t.Fatalf("Second EnsureIndexes failed: %v", err)
	}
}

func TestStore_VerifyToken_ExpiredNotReturned(t *testing.T) {
	db := testutil.SetupTestDB(t)
	// Use a very short expiry
	store := emailverify.New(db, 1*time.Millisecond)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	userID := primitive.NewObjectID()
	email := "test@example.com"

	result, err := store.Create(ctx, userID, email, false)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Wait for expiry
	time.Sleep(10 * time.Millisecond)

	// Verify should fail because it's expired
	_, err = store.VerifyToken(ctx, result.Token)
	if err != emailverify.ErrNotFound {
		t.Errorf("expected ErrNotFound for expired token, got %v", err)
	}
}

func TestStore_VerifyCode_ExpiredNotReturned(t *testing.T) {
	db := testutil.SetupTestDB(t)
	// Use a very short expiry
	store := emailverify.New(db, 1*time.Millisecond)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	userID := primitive.NewObjectID()
	email := "test@example.com"

	result, err := store.Create(ctx, userID, email, false)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Wait for expiry
	time.Sleep(10 * time.Millisecond)

	// Verify should fail because it's expired
	_, err = store.VerifyCode(ctx, userID, result.Code)
	if err != emailverify.ErrNotFound {
		t.Errorf("expected ErrNotFound for expired code, got %v", err)
	}
}
