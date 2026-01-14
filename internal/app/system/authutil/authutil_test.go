package authutil

import (
	"strings"
	"testing"
)

// Test EmailIsLogin for various auth methods

func TestEmailIsLogin_EmailMethod(t *testing.T) {
	if !EmailIsLogin("email") {
		t.Error("expected email method to return true for EmailIsLogin")
	}
}

func TestEmailIsLogin_GoogleMethod(t *testing.T) {
	if !EmailIsLogin("google") {
		t.Error("expected google method to return true for EmailIsLogin")
	}
}

func TestEmailIsLogin_MicrosoftMethod(t *testing.T) {
	if !EmailIsLogin("microsoft") {
		t.Error("expected microsoft method to return true for EmailIsLogin")
	}
}

func TestEmailIsLogin_PasswordMethod(t *testing.T) {
	if EmailIsLogin("password") {
		t.Error("expected password method to return false for EmailIsLogin")
	}
}

func TestEmailIsLogin_TrustMethod(t *testing.T) {
	if EmailIsLogin("trust") {
		t.Error("expected trust method to return false for EmailIsLogin")
	}
}

func TestEmailIsLogin_CleverMethod(t *testing.T) {
	if EmailIsLogin("clever") {
		t.Error("expected clever method to return false for EmailIsLogin")
	}
}

// Test RequiresAuthReturnID for various auth methods

func TestRequiresAuthReturnID_CleverMethod(t *testing.T) {
	if !RequiresAuthReturnID("clever") {
		t.Error("expected clever method to return true for RequiresAuthReturnID")
	}
}

func TestRequiresAuthReturnID_ClassLinkMethod(t *testing.T) {
	if !RequiresAuthReturnID("classlink") {
		t.Error("expected classlink method to return true for RequiresAuthReturnID")
	}
}

func TestRequiresAuthReturnID_SchoologyMethod(t *testing.T) {
	if !RequiresAuthReturnID("schoology") {
		t.Error("expected schoology method to return true for RequiresAuthReturnID")
	}
}

func TestRequiresAuthReturnID_PasswordMethod(t *testing.T) {
	if RequiresAuthReturnID("password") {
		t.Error("expected password method to return false for RequiresAuthReturnID")
	}
}

func TestRequiresAuthReturnID_EmailMethod(t *testing.T) {
	if RequiresAuthReturnID("email") {
		t.Error("expected email method to return false for RequiresAuthReturnID")
	}
}

// Test isValidEmail helper function

func TestIsValidEmail_Valid(t *testing.T) {
	validEmails := []string{
		"test@example.com",
		"user@domain.org",
		"name.surname@company.co.uk",
		"a@b.co",
	}

	for _, email := range validEmails {
		if !isValidEmail(email) {
			t.Errorf("expected %q to be valid", email)
		}
	}
}

func TestIsValidEmail_MissingAt(t *testing.T) {
	if isValidEmail("testexample.com") {
		t.Error("expected email without @ to be invalid")
	}
}

func TestIsValidEmail_MultipleAt(t *testing.T) {
	if isValidEmail("test@@example.com") {
		t.Error("expected email with multiple @ to be invalid")
	}
}

func TestIsValidEmail_EmptyLocalPart(t *testing.T) {
	if isValidEmail("@example.com") {
		t.Error("expected email with empty local part to be invalid")
	}
}

func TestIsValidEmail_NoDomainDot(t *testing.T) {
	if isValidEmail("test@example") {
		t.Error("expected email without domain dot to be invalid")
	}
}

func TestIsValidEmail_DotAtEnd(t *testing.T) {
	if isValidEmail("test@example.") {
		t.Error("expected email with dot at end to be invalid")
	}
}

func TestIsValidEmail_DotAtStart(t *testing.T) {
	if isValidEmail("test@.com") {
		t.Error("expected email with dot at start of domain to be invalid")
	}
}

// Test ValidateAndResolve for email-login methods

func TestValidateAndResolve_EmailMethod_Valid(t *testing.T) {
	input := AuthInput{
		Method: "email",
		Email:  "user@example.com",
	}

	result, err := ValidateAndResolve(input)
	if err != nil {
		t.Fatalf("ValidateAndResolve failed: %v", err)
	}

	if result.EffectiveLoginID != "user@example.com" {
		t.Errorf("EffectiveLoginID: got %q, want %q", result.EffectiveLoginID, "user@example.com")
	}
	if result.Email == nil || *result.Email != "user@example.com" {
		t.Error("expected Email to be set")
	}
}

func TestValidateAndResolve_GoogleMethod_Valid(t *testing.T) {
	input := AuthInput{
		Method: "google",
		Email:  "user@gmail.com",
	}

	result, err := ValidateAndResolve(input)
	if err != nil {
		t.Fatalf("ValidateAndResolve failed: %v", err)
	}

	if result.EffectiveLoginID != "user@gmail.com" {
		t.Errorf("EffectiveLoginID: got %q, want %q", result.EffectiveLoginID, "user@gmail.com")
	}
}

func TestValidateAndResolve_EmailMethod_MissingEmail(t *testing.T) {
	input := AuthInput{
		Method: "email",
		Email:  "",
	}

	_, err := ValidateAndResolve(input)
	if err != ErrEmailRequired {
		t.Errorf("expected ErrEmailRequired, got %v", err)
	}
}

func TestValidateAndResolve_EmailMethod_InvalidEmail(t *testing.T) {
	input := AuthInput{
		Method: "email",
		Email:  "invalid-email",
	}

	_, err := ValidateAndResolve(input)
	if err != ErrInvalidEmail {
		t.Errorf("expected ErrInvalidEmail, got %v", err)
	}
}

// Test ValidateAndResolve for password method

func TestValidateAndResolve_PasswordMethod_Valid(t *testing.T) {
	input := AuthInput{
		Method:       "password",
		LoginID:      "jsmith",
		TempPassword: "SecurePass123",
	}

	result, err := ValidateAndResolve(input)
	if err != nil {
		t.Fatalf("ValidateAndResolve failed: %v", err)
	}

	if result.EffectiveLoginID != "jsmith" {
		t.Errorf("EffectiveLoginID: got %q, want %q", result.EffectiveLoginID, "jsmith")
	}
	if result.PasswordHash == nil {
		t.Error("expected PasswordHash to be set")
	}
	if result.PasswordTemp == nil || !*result.PasswordTemp {
		t.Error("expected PasswordTemp to be true")
	}
}

func TestValidateAndResolve_PasswordMethod_MissingLoginID(t *testing.T) {
	input := AuthInput{
		Method:       "password",
		LoginID:      "",
		TempPassword: "SecurePass123",
	}

	_, err := ValidateAndResolve(input)
	if err != ErrLoginIDRequired {
		t.Errorf("expected ErrLoginIDRequired, got %v", err)
	}
}

func TestValidateAndResolve_PasswordMethod_MissingPassword(t *testing.T) {
	input := AuthInput{
		Method:       "password",
		LoginID:      "jsmith",
		TempPassword: "",
		IsEdit:       false,
	}

	_, err := ValidateAndResolve(input)
	if err != ErrPasswordRequired {
		t.Errorf("expected ErrPasswordRequired, got %v", err)
	}
}

func TestValidateAndResolve_PasswordMethod_EditNoPassword(t *testing.T) {
	input := AuthInput{
		Method:       "password",
		LoginID:      "jsmith",
		TempPassword: "",
		IsEdit:       true, // In edit mode, password is optional
	}

	result, err := ValidateAndResolve(input)
	if err != nil {
		t.Fatalf("ValidateAndResolve failed: %v", err)
	}

	if result.PasswordHash != nil {
		t.Error("expected PasswordHash to be nil when password not provided in edit mode")
	}
}

func TestValidateAndResolve_PasswordMethod_WithOptionalEmail(t *testing.T) {
	input := AuthInput{
		Method:       "password",
		LoginID:      "jsmith",
		Email:        "jsmith@example.com",
		TempPassword: "SecurePass123",
	}

	result, err := ValidateAndResolve(input)
	if err != nil {
		t.Fatalf("ValidateAndResolve failed: %v", err)
	}

	if result.Email == nil || *result.Email != "jsmith@example.com" {
		t.Error("expected Email to be set")
	}
}

// Test ValidateAndResolve for trust method

func TestValidateAndResolve_TrustMethod_Valid(t *testing.T) {
	input := AuthInput{
		Method:  "trust",
		LoginID: "student123",
	}

	result, err := ValidateAndResolve(input)
	if err != nil {
		t.Fatalf("ValidateAndResolve failed: %v", err)
	}

	if result.EffectiveLoginID != "student123" {
		t.Errorf("EffectiveLoginID: got %q, want %q", result.EffectiveLoginID, "student123")
	}
}

func TestValidateAndResolve_TrustMethod_MissingLoginID(t *testing.T) {
	input := AuthInput{
		Method:  "trust",
		LoginID: "",
	}

	_, err := ValidateAndResolve(input)
	if err != ErrLoginIDRequired {
		t.Errorf("expected ErrLoginIDRequired, got %v", err)
	}
}

// Test ValidateAndResolve for auth_return_id methods (clever, classlink, schoology)

func TestValidateAndResolve_CleverMethod_Valid(t *testing.T) {
	input := AuthInput{
		Method:       "clever",
		LoginID:      "student123",
		AuthReturnID: "clever-id-12345",
	}

	result, err := ValidateAndResolve(input)
	if err != nil {
		t.Fatalf("ValidateAndResolve failed: %v", err)
	}

	if result.EffectiveLoginID != "student123" {
		t.Errorf("EffectiveLoginID: got %q, want %q", result.EffectiveLoginID, "student123")
	}
	if result.AuthReturnID == nil || *result.AuthReturnID != "clever-id-12345" {
		t.Error("expected AuthReturnID to be set")
	}
}

func TestValidateAndResolve_CleverMethod_MissingAuthReturnID(t *testing.T) {
	input := AuthInput{
		Method:       "clever",
		LoginID:      "student123",
		AuthReturnID: "",
	}

	_, err := ValidateAndResolve(input)
	if err != ErrAuthReturnIDRequired {
		t.Errorf("expected ErrAuthReturnIDRequired, got %v", err)
	}
}

func TestValidateAndResolve_ClassLinkMethod_Valid(t *testing.T) {
	input := AuthInput{
		Method:       "classlink",
		LoginID:      "student456",
		AuthReturnID: "classlink-id-67890",
	}

	result, err := ValidateAndResolve(input)
	if err != nil {
		t.Fatalf("ValidateAndResolve failed: %v", err)
	}

	if result.AuthReturnID == nil || *result.AuthReturnID != "classlink-id-67890" {
		t.Error("expected AuthReturnID to be set")
	}
}

func TestValidateAndResolve_SchoologyMethod_MissingAuthReturnID(t *testing.T) {
	input := AuthInput{
		Method:       "schoology",
		LoginID:      "student789",
		AuthReturnID: "",
	}

	_, err := ValidateAndResolve(input)
	if err != ErrAuthReturnIDRequired {
		t.Errorf("expected ErrAuthReturnIDRequired, got %v", err)
	}
}

// Test TemplateData helper methods

func TestTemplateData_EmailIsLoginMethod_True(t *testing.T) {
	data := TemplateData{Auth: "email"}
	if !data.EmailIsLoginMethod() {
		t.Error("expected EmailIsLoginMethod to return true for email auth")
	}
}

func TestTemplateData_EmailIsLoginMethod_False(t *testing.T) {
	data := TemplateData{Auth: "password"}
	if data.EmailIsLoginMethod() {
		t.Error("expected EmailIsLoginMethod to return false for password auth")
	}
}

func TestTemplateData_RequiresAuthReturnIDMethod_True(t *testing.T) {
	data := TemplateData{Auth: "clever"}
	if !data.RequiresAuthReturnIDMethod() {
		t.Error("expected RequiresAuthReturnIDMethod to return true for clever auth")
	}
}

func TestTemplateData_RequiresAuthReturnIDMethod_False(t *testing.T) {
	data := TemplateData{Auth: "email"}
	if data.RequiresAuthReturnIDMethod() {
		t.Error("expected RequiresAuthReturnIDMethod to return false for email auth")
	}
}

func TestTemplateData_IsPasswordMethod_True(t *testing.T) {
	data := TemplateData{Auth: "password"}
	if !data.IsPasswordMethod() {
		t.Error("expected IsPasswordMethod to return true for password auth")
	}
}

func TestTemplateData_IsPasswordMethod_False(t *testing.T) {
	data := TemplateData{Auth: "email"}
	if data.IsPasswordMethod() {
		t.Error("expected IsPasswordMethod to return false for email auth")
	}
}

// Test password validation

func TestValidatePassword_Valid(t *testing.T) {
	validPasswords := []string{
		"secure123",
		"MyP@ssw0rd",
		"abcdef1", // 7 chars, just above minimum
	}

	for _, pw := range validPasswords {
		if err := ValidatePassword(pw); err != nil {
			t.Errorf("expected %q to be valid, got error: %v", pw, err)
		}
	}
}

func TestValidatePassword_TooShort(t *testing.T) {
	shortPasswords := []string{
		"",
		"a",
		"ab",
		"abc",
		"abcd",
		"abcde", // 5 chars, below minimum of 6
	}

	for _, pw := range shortPasswords {
		err := ValidatePassword(pw)
		if err != ErrPasswordTooShort {
			t.Errorf("expected ErrPasswordTooShort for %q, got %v", pw, err)
		}
	}
}

func TestValidatePassword_TooLong(t *testing.T) {
	// Create a password that's 129 characters (over MaxPasswordLength of 128)
	longPassword := strings.Repeat("a", 129)

	err := ValidatePassword(longPassword)
	if err != ErrPasswordTooLong {
		t.Errorf("expected ErrPasswordTooLong, got %v", err)
	}
}

func TestValidatePassword_AtMaxLength(t *testing.T) {
	// Create a password that's exactly 128 characters
	maxPassword := strings.Repeat("a", 128)

	if err := ValidatePassword(maxPassword); err != nil {
		t.Errorf("expected password at max length to be valid, got %v", err)
	}
}

func TestValidatePassword_Common(t *testing.T) {
	commonPwds := []string{
		"123456",
		"password",
		"qwerty",
		"abc123",
		"iloveyou",
		"letmein",
		"football",
		"welcome",
	}

	for _, pw := range commonPwds {
		err := ValidatePassword(pw)
		if err != ErrPasswordCommon {
			t.Errorf("expected ErrPasswordCommon for %q, got %v", pw, err)
		}
	}
}

func TestValidatePassword_CommonCaseInsensitive(t *testing.T) {
	// Test that common password check is case-insensitive
	caseVariants := []string{
		"PASSWORD",
		"Password",
		"QWERTY",
		"Qwerty",
		"ILOVEYOU",
		"ILoveYou",
	}

	for _, pw := range caseVariants {
		err := ValidatePassword(pw)
		if err != ErrPasswordCommon {
			t.Errorf("expected ErrPasswordCommon for %q (case variant), got %v", pw, err)
		}
	}
}

// Test password hashing

func TestHashPassword_Valid(t *testing.T) {
	password := "SecurePassword123"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	if hash == "" {
		t.Error("expected hash to be non-empty")
	}
	if hash == password {
		t.Error("hash should not equal plain password")
	}
	// bcrypt hashes start with $2a$ or $2b$
	if hash[0] != '$' {
		t.Error("expected bcrypt hash to start with $")
	}
}

func TestHashPassword_DifferentHashesForSamePassword(t *testing.T) {
	password := "SecurePassword123"

	hash1, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	hash2, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	// bcrypt uses random salt, so hashes should be different
	if hash1 == hash2 {
		t.Error("expected different hashes for same password (random salt)")
	}
}

// Test password checking

func TestCheckPassword_Correct(t *testing.T) {
	password := "SecurePassword123"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	if !CheckPassword(password, hash) {
		t.Error("expected CheckPassword to return true for correct password")
	}
}

func TestCheckPassword_Incorrect(t *testing.T) {
	password := "SecurePassword123"
	wrongPassword := "WrongPassword456"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	if CheckPassword(wrongPassword, hash) {
		t.Error("expected CheckPassword to return false for wrong password")
	}
}

func TestCheckPassword_EmptyPassword(t *testing.T) {
	password := "SecurePassword123"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	if CheckPassword("", hash) {
		t.Error("expected CheckPassword to return false for empty password")
	}
}

func TestCheckPassword_InvalidHash(t *testing.T) {
	if CheckPassword("password", "not-a-valid-hash") {
		t.Error("expected CheckPassword to return false for invalid hash")
	}
}

// Test PasswordRules

func TestPasswordRules(t *testing.T) {
	rules := PasswordRules()
	if rules == "" {
		t.Error("expected PasswordRules to return non-empty string")
	}
	// Should mention minimum length
	if !contains(rules, "6") {
		t.Error("expected PasswordRules to mention minimum length of 6")
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
