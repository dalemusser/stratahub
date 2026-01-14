package csvutil

import (
	"strings"
	"testing"
)

func TestParseMembersCSV_Empty(t *testing.T) {
	csv := ""
	result, err := ParseMembersCSV(strings.NewReader(csv), ParseOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Members) != 0 {
		t.Errorf("expected 0 members, got %d", len(result.Members))
	}
	if result.HasErrors() {
		t.Errorf("expected no errors, got %d", len(result.Errors))
	}
}

func TestParseMembersCSV_HeaderOnly(t *testing.T) {
	csv := "full_name,auth_type,email\n"
	result, err := ParseMembersCSV(strings.NewReader(csv), ParseOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Members) != 0 {
		t.Errorf("expected 0 members (header only), got %d", len(result.Members))
	}
	if result.HasErrors() {
		t.Errorf("expected no errors, got %d", len(result.Errors))
	}
}

func TestParseMembersCSV_WithBOM(t *testing.T) {
	// BOM is \ufeff at the start of the file
	csv := "\ufeffJohn Doe,email,john@example.com\n"
	result, err := ParseMembersCSV(strings.NewReader(csv), ParseOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Members) != 1 {
		t.Fatalf("expected 1 member, got %d", len(result.Members))
	}

	if result.Members[0].FullName != "John Doe" {
		t.Errorf("BOM should be stripped: got %q, want %q", result.Members[0].FullName, "John Doe")
	}
}

func TestParseMembersCSV_ValidRows(t *testing.T) {
	csv := `full_name,auth_type,email
John Doe,email,john@example.com
Jane Smith,google,jane@example.com
Bob Wilson,microsoft,bob@example.com`

	result, err := ParseMembersCSV(strings.NewReader(csv), ParseOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Members) != 3 {
		t.Errorf("expected 3 members, got %d", len(result.Members))
	}
	if result.HasErrors() {
		t.Errorf("expected no errors, got %d: %v", len(result.Errors), result.Errors)
	}

	// Verify first member
	if result.Members[0].FullName != "John Doe" {
		t.Errorf("member 0 FullName: got %q, want %q", result.Members[0].FullName, "John Doe")
	}
	if result.Members[0].AuthMethod != "email" {
		t.Errorf("member 0 AuthMethod: got %q, want %q", result.Members[0].AuthMethod, "email")
	}
	if result.Members[0].LoginID != "john@example.com" {
		t.Errorf("member 0 LoginID: got %q, want %q", result.Members[0].LoginID, "john@example.com")
	}
}

func TestParseMembersCSV_MixedValidAndErrors(t *testing.T) {
	csv := `full_name,auth_type,email
John Doe,email,john@example.com
,email,missing@example.com
Jane Smith,google,jane@example.com`

	result, err := ParseMembersCSV(strings.NewReader(csv), ParseOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Members) != 2 {
		t.Errorf("expected 2 valid members, got %d", len(result.Members))
	}
	if len(result.Errors) != 1 {
		t.Errorf("expected 1 error, got %d", len(result.Errors))
	}

	// Verify the error is about missing name
	if len(result.Errors) > 0 {
		if !strings.Contains(result.Errors[0].Reason, "missing full name") {
			t.Errorf("expected error about missing name, got: %s", result.Errors[0].Reason)
		}
	}
}

func TestParseMembersCSV_DuplicateLoginIDs(t *testing.T) {
	csv := `full_name,auth_type,email
John Doe,email,john@example.com
Jane Doe,email,john@example.com`

	result, err := ParseMembersCSV(strings.NewReader(csv), ParseOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Members) != 1 {
		t.Errorf("expected 1 member (first occurrence), got %d", len(result.Members))
	}
	if len(result.Errors) != 1 {
		t.Errorf("expected 1 error (duplicate), got %d", len(result.Errors))
	}

	if len(result.Errors) > 0 {
		if !strings.Contains(result.Errors[0].Reason, "duplicate login ID") {
			t.Errorf("expected error about duplicate, got: %s", result.Errors[0].Reason)
		}
	}
}

func TestParseMembersCSV_DuplicateLoginIDs_CaseInsensitive(t *testing.T) {
	csv := `John Doe,email,JOHN@example.com
Jane Doe,email,john@example.com`

	result, err := ParseMembersCSV(strings.NewReader(csv), ParseOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Members) != 1 {
		t.Errorf("expected 1 member (case insensitive duplicate check), got %d", len(result.Members))
	}
	if len(result.Errors) != 1 {
		t.Errorf("expected 1 error, got %d", len(result.Errors))
	}
}

func TestParseMembersCSV_TooManyRows(t *testing.T) {
	csv := `John Doe,email,john@example.com
Jane Doe,email,jane@example.com
Bob Wilson,email,bob@example.com`

	result, err := ParseMembersCSV(strings.NewReader(csv), ParseOptions{MaxRows: 2})
	if err != ErrTooManyRows {
		t.Errorf("expected ErrTooManyRows, got %v", err)
	}
	_ = result // result may be partial
}

func TestIsHeaderRow_ValidHeaders(t *testing.T) {
	testCases := []struct {
		name   string
		row    []string
		expect bool
	}{
		{"full_name header", []string{"full_name", "auth_type", "email"}, true},
		{"fullname header", []string{"fullname", "auth", "email"}, true},
		{"name header", []string{"name", "type", "email"}, true},
		{"Full Name header", []string{"Full Name", "Auth Method", "Email"}, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isHeaderRow(tc.row)
			if result != tc.expect {
				t.Errorf("isHeaderRow(%v) = %v, want %v", tc.row, result, tc.expect)
			}
		})
	}
}

func TestIsHeaderRow_CaseInsensitive(t *testing.T) {
	testCases := [][]string{
		{"FULL_NAME", "AUTH_TYPE", "EMAIL"},
		{"Full_Name", "Auth_Type", "Email"},
		{"FULLNAME", "AUTH", "EMAIL"},
	}

	for _, row := range testCases {
		if !isHeaderRow(row) {
			t.Errorf("isHeaderRow should be case insensitive for: %v", row)
		}
	}
}

func TestIsHeaderRow_NotHeader(t *testing.T) {
	testCases := []struct {
		name string
		row  []string
	}{
		{"actual data", []string{"John Doe", "email", "john@example.com"}},
		{"password row", []string{"John Doe", "password", "jdoe", "secret123"}},
		{"trust row", []string{"Jane", "trust", "jane123"}},
		{"too few columns", []string{"name"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if isHeaderRow(tc.row) {
				t.Errorf("isHeaderRow should return false for: %v", tc.row)
			}
		})
	}
}

func TestParseRow_EmptyRow(t *testing.T) {
	member, err := parseRow([]string{}, 1)
	if member != nil {
		t.Errorf("expected nil member for empty row")
	}
	if err != nil {
		t.Errorf("expected nil error for empty row")
	}
}

func TestParseRow_AllEmptyFields(t *testing.T) {
	member, err := parseRow([]string{"", "", ""}, 1)
	if member != nil {
		t.Errorf("expected nil member for all empty fields")
	}
	if err != nil {
		t.Errorf("expected nil error for all empty fields")
	}
}

func TestParseRow_TooFewFields(t *testing.T) {
	member, err := parseRow([]string{"John Doe", "email"}, 1)
	if member != nil {
		t.Errorf("expected nil member for too few fields")
	}
	if err == nil {
		t.Fatal("expected error for too few fields")
	}
	if !strings.Contains(err.Reason, "at least 3 fields") {
		t.Errorf("expected error about too few fields, got: %s", err.Reason)
	}
}

func TestParseRow_MissingFullName(t *testing.T) {
	member, err := parseRow([]string{"", "email", "john@example.com"}, 1)
	if member != nil {
		t.Errorf("expected nil member for missing name")
	}
	if err == nil {
		t.Fatal("expected error for missing name")
	}
	if !strings.Contains(err.Reason, "missing full name") {
		t.Errorf("expected error about missing name, got: %s", err.Reason)
	}
}

func TestParseRow_InvalidAuthMethod(t *testing.T) {
	// Use "foobar" instead of "invalid_auth" since "auth" would match header detection
	member, err := parseRow([]string{"John Doe", "foobar", "john@example.com"}, 1)
	if member != nil {
		t.Errorf("expected nil member for invalid auth")
	}
	if err == nil {
		t.Fatal("expected error for invalid auth")
	}
	if !strings.Contains(err.Reason, "invalid or missing auth method") {
		t.Errorf("expected error about invalid auth, got: %s", err.Reason)
	}
}

func TestParseEmailAuthRow_Valid(t *testing.T) {
	member, err := parseRow([]string{"John Doe", "email", "john@example.com"}, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err.Reason)
	}
	if member == nil {
		t.Fatal("expected member, got nil")
	}

	if member.FullName != "John Doe" {
		t.Errorf("FullName: got %q, want %q", member.FullName, "John Doe")
	}
	if member.AuthMethod != "email" {
		t.Errorf("AuthMethod: got %q, want %q", member.AuthMethod, "email")
	}
	if member.LoginID != "john@example.com" {
		t.Errorf("LoginID: got %q, want %q", member.LoginID, "john@example.com")
	}
	if member.Email == nil || *member.Email != "john@example.com" {
		t.Errorf("Email: got %v, want %q", member.Email, "john@example.com")
	}
}

func TestParseEmailAuthRow_MissingEmail(t *testing.T) {
	member, err := parseRow([]string{"John Doe", "email", ""}, 1)
	if member != nil {
		t.Errorf("expected nil member")
	}
	if err == nil {
		t.Fatal("expected error for missing email")
	}
	if !strings.Contains(err.Reason, "email is required") {
		t.Errorf("expected error about required email, got: %s", err.Reason)
	}
}

func TestParseEmailAuthRow_InvalidEmail(t *testing.T) {
	member, err := parseRow([]string{"John Doe", "email", "not-an-email"}, 1)
	if member != nil {
		t.Errorf("expected nil member")
	}
	if err == nil {
		t.Fatal("expected error for invalid email")
	}
	if !strings.Contains(err.Reason, "invalid email") {
		t.Errorf("expected error about invalid email, got: %s", err.Reason)
	}
}

func TestParsePasswordAuthRow_Valid(t *testing.T) {
	member, err := parseRow([]string{"John Doe", "password", "jdoe", "secret123"}, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err.Reason)
	}
	if member == nil {
		t.Fatal("expected member, got nil")
	}

	if member.AuthMethod != "password" {
		t.Errorf("AuthMethod: got %q, want %q", member.AuthMethod, "password")
	}
	if member.LoginID != "jdoe" {
		t.Errorf("LoginID: got %q, want %q", member.LoginID, "jdoe")
	}
	if member.TempPassword == nil || *member.TempPassword != "secret123" {
		t.Errorf("TempPassword: got %v, want %q", member.TempPassword, "secret123")
	}
	if member.Email != nil {
		t.Errorf("Email should be nil without optional email, got %v", member.Email)
	}
}

func TestParsePasswordAuthRow_MissingLoginID(t *testing.T) {
	member, err := parseRow([]string{"John Doe", "password", "", "secret123"}, 1)
	if member != nil {
		t.Errorf("expected nil member")
	}
	if err == nil {
		t.Fatal("expected error for missing login ID")
	}
	if !strings.Contains(err.Reason, "login ID is required") {
		t.Errorf("expected error about login ID, got: %s", err.Reason)
	}
}

func TestParsePasswordAuthRow_MissingPassword(t *testing.T) {
	member, err := parseRow([]string{"John Doe", "password", "jdoe", ""}, 1)
	if member != nil {
		t.Errorf("expected nil member")
	}
	if err == nil {
		t.Fatal("expected error for missing password")
	}
	if !strings.Contains(err.Reason, "temporary password is required") {
		t.Errorf("expected error about password, got: %s", err.Reason)
	}
}

func TestParsePasswordAuthRow_TooFewFields(t *testing.T) {
	member, err := parseRow([]string{"John Doe", "password", "jdoe"}, 1)
	if member != nil {
		t.Errorf("expected nil member")
	}
	if err == nil {
		t.Fatal("expected error for too few fields")
	}
	if !strings.Contains(err.Reason, "password authentication requires") {
		t.Errorf("expected error about fields, got: %s", err.Reason)
	}
}

func TestParsePasswordAuthRow_OptionalEmail(t *testing.T) {
	member, err := parseRow([]string{"John Doe", "password", "jdoe", "secret123", "john@example.com"}, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err.Reason)
	}
	if member == nil {
		t.Fatal("expected member, got nil")
	}

	if member.Email == nil || *member.Email != "john@example.com" {
		t.Errorf("Email: got %v, want %q", member.Email, "john@example.com")
	}
}

func TestParsePasswordAuthRow_InvalidOptionalEmail(t *testing.T) {
	member, err := parseRow([]string{"John Doe", "password", "jdoe", "secret123", "not-an-email"}, 1)
	if member != nil {
		t.Errorf("expected nil member")
	}
	if err == nil {
		t.Fatal("expected error for invalid email")
	}
	if !strings.Contains(err.Reason, "invalid email format in optional") {
		t.Errorf("expected error about optional email, got: %s", err.Reason)
	}
}

func TestParseTrustAuthRow_Valid(t *testing.T) {
	member, err := parseRow([]string{"John Doe", "trust", "jdoe123"}, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err.Reason)
	}
	if member == nil {
		t.Fatal("expected member, got nil")
	}

	if member.AuthMethod != "trust" {
		t.Errorf("AuthMethod: got %q, want %q", member.AuthMethod, "trust")
	}
	if member.LoginID != "jdoe123" {
		t.Errorf("LoginID: got %q, want %q", member.LoginID, "jdoe123")
	}
}

func TestParseTrustAuthRow_MissingLoginID(t *testing.T) {
	member, err := parseRow([]string{"John Doe", "trust", ""}, 1)
	if member != nil {
		t.Errorf("expected nil member")
	}
	if err == nil {
		t.Fatal("expected error for missing login ID")
	}
	if !strings.Contains(err.Reason, "login ID is required for trust") {
		t.Errorf("expected error about login ID, got: %s", err.Reason)
	}
}

func TestParseTrustAuthRow_OptionalEmail(t *testing.T) {
	member, err := parseRow([]string{"John Doe", "trust", "jdoe123", "john@example.com"}, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err.Reason)
	}
	if member == nil {
		t.Fatal("expected member, got nil")
	}

	if member.Email == nil || *member.Email != "john@example.com" {
		t.Errorf("Email: got %v, want %q", member.Email, "john@example.com")
	}
}

func TestParseAuthReturnIDRow_ShortForm(t *testing.T) {
	// Short form: email used as login_id, auth_return_id, and email
	member, err := parseRow([]string{"John Doe", "clever", "john@example.com"}, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err.Reason)
	}
	if member == nil {
		t.Fatal("expected member, got nil")
	}

	if member.AuthMethod != "clever" {
		t.Errorf("AuthMethod: got %q, want %q", member.AuthMethod, "clever")
	}
	if member.LoginID != "john@example.com" {
		t.Errorf("LoginID: got %q, want %q", member.LoginID, "john@example.com")
	}
	if member.AuthReturnID == nil || *member.AuthReturnID != "john@example.com" {
		t.Errorf("AuthReturnID: got %v, want %q", member.AuthReturnID, "john@example.com")
	}
	if member.Email == nil || *member.Email != "john@example.com" {
		t.Errorf("Email: got %v, want %q", member.Email, "john@example.com")
	}
}

func TestParseAuthReturnIDRow_ShortForm_MissingEmail(t *testing.T) {
	member, err := parseRow([]string{"John Doe", "clever", ""}, 1)
	if member != nil {
		t.Errorf("expected nil member")
	}
	if err == nil {
		t.Fatal("expected error for missing email")
	}
	if !strings.Contains(err.Reason, "email is required") {
		t.Errorf("expected error about email, got: %s", err.Reason)
	}
}

func TestParseAuthReturnIDRow_LongForm(t *testing.T) {
	// Long form: separate login_id and auth_return_id
	member, err := parseRow([]string{"John Doe", "classlink", "jdoe123", "auth-id-456"}, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err.Reason)
	}
	if member == nil {
		t.Fatal("expected member, got nil")
	}

	if member.AuthMethod != "classlink" {
		t.Errorf("AuthMethod: got %q, want %q", member.AuthMethod, "classlink")
	}
	if member.LoginID != "jdoe123" {
		t.Errorf("LoginID: got %q, want %q", member.LoginID, "jdoe123")
	}
	if member.AuthReturnID == nil || *member.AuthReturnID != "auth-id-456" {
		t.Errorf("AuthReturnID: got %v, want %q", member.AuthReturnID, "auth-id-456")
	}
	if member.Email != nil {
		t.Errorf("Email should be nil without optional email")
	}
}

func TestParseAuthReturnIDRow_LongForm_WithEmail(t *testing.T) {
	member, err := parseRow([]string{"John Doe", "schoology", "jdoe123", "auth-id-456", "john@example.com"}, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err.Reason)
	}
	if member == nil {
		t.Fatal("expected member, got nil")
	}

	if member.Email == nil || *member.Email != "john@example.com" {
		t.Errorf("Email: got %v, want %q", member.Email, "john@example.com")
	}
}

func TestParseAuthReturnIDRow_LongForm_MissingLoginID(t *testing.T) {
	member, err := parseRow([]string{"John Doe", "clever", "", "auth-id-456"}, 1)
	if member != nil {
		t.Errorf("expected nil member")
	}
	if err == nil {
		t.Fatal("expected error for missing login ID")
	}
	if !strings.Contains(err.Reason, "login ID is required") {
		t.Errorf("expected error about login ID, got: %s", err.Reason)
	}
}

func TestParseAuthReturnIDRow_LongForm_MissingAuthReturnID(t *testing.T) {
	member, err := parseRow([]string{"John Doe", "clever", "jdoe123", ""}, 1)
	if member != nil {
		t.Errorf("expected nil member")
	}
	if err == nil {
		t.Fatal("expected error for missing auth return ID")
	}
	if !strings.Contains(err.Reason, "auth return ID is required") {
		t.Errorf("expected error about auth return ID, got: %s", err.Reason)
	}
}

func TestParseMembersCSV_GoogleAuth(t *testing.T) {
	csv := `John Doe,google,john@example.com`
	result, err := ParseMembersCSV(strings.NewReader(csv), ParseOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Members) != 1 {
		t.Fatalf("expected 1 member, got %d", len(result.Members))
	}
	if result.Members[0].AuthMethod != "google" {
		t.Errorf("AuthMethod: got %q, want %q", result.Members[0].AuthMethod, "google")
	}
}

func TestParseMembersCSV_MicrosoftAuth(t *testing.T) {
	csv := `John Doe,microsoft,john@example.com`
	result, err := ParseMembersCSV(strings.NewReader(csv), ParseOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Members) != 1 {
		t.Fatalf("expected 1 member, got %d", len(result.Members))
	}
	if result.Members[0].AuthMethod != "microsoft" {
		t.Errorf("AuthMethod: got %q, want %q", result.Members[0].AuthMethod, "microsoft")
	}
}

func TestParseMembersCSV_WhitespaceTrimming(t *testing.T) {
	csv := `  John Doe  ,  email  ,  john@example.com  `
	result, err := ParseMembersCSV(strings.NewReader(csv), ParseOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Members) != 1 {
		t.Fatalf("expected 1 member, got %d", len(result.Members))
	}
	if result.Members[0].FullName != "John Doe" {
		t.Errorf("FullName should be trimmed: got %q", result.Members[0].FullName)
	}
	if result.Members[0].LoginID != "john@example.com" {
		t.Errorf("LoginID should be trimmed: got %q", result.Members[0].LoginID)
	}
}

func TestParseMembersCSV_SkipsEmptyRows(t *testing.T) {
	csv := `full_name,auth_type,email
John Doe,email,john@example.com

Jane Smith,email,jane@example.com
,,
Bob Wilson,email,bob@example.com`

	result, err := ParseMembersCSV(strings.NewReader(csv), ParseOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Members) != 3 {
		t.Errorf("expected 3 members (empty rows skipped), got %d", len(result.Members))
	}
	if result.HasErrors() {
		t.Errorf("expected no errors for empty rows, got: %v", result.Errors)
	}
}

func TestParsedResult_HasErrors(t *testing.T) {
	// No errors
	result := ParsedResult{}
	if result.HasErrors() {
		t.Error("HasErrors should be false when no errors")
	}

	// With errors
	result.Errors = append(result.Errors, RowError{Line: 1, Reason: "test"})
	if !result.HasErrors() {
		t.Error("HasErrors should be true when errors exist")
	}
}

func TestParseRow_HeaderRowInMiddle(t *testing.T) {
	// Header rows can appear anywhere and should be skipped
	member, err := parseRow([]string{"full_name", "auth_type", "email"}, 5)
	if member != nil {
		t.Errorf("header rows should return nil member")
	}
	if err != nil {
		t.Errorf("header rows should not return error")
	}
}
