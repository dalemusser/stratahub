package csvutil

import (
	"strings"
	"testing"
)

func TestParseMemberCSV_ValidRows(t *testing.T) {
	csv := `Full Name,Email,Auth Method
John Doe,john@example.com,internal
Jane Smith,jane@example.com,google
Bob Wilson,bob@example.com,classlink`

	result, err := ParseMemberCSV(strings.NewReader(csv), DefaultParseOptions())
	if err != nil {
		t.Fatalf("ParseMemberCSV() error = %v", err)
	}

	if len(result.Rows) != 3 {
		t.Errorf("ParseMemberCSV() got %d rows, want 3", len(result.Rows))
	}

	if result.HasErrors() {
		t.Errorf("ParseMemberCSV() unexpected errors: %v", result.Errors)
	}

	// Check first row
	if result.Rows[0].FullName != "John Doe" {
		t.Errorf("Row 0 FullName = %q, want %q", result.Rows[0].FullName, "John Doe")
	}
	if result.Rows[0].Email != "john@example.com" {
		t.Errorf("Row 0 Email = %q, want %q", result.Rows[0].Email, "john@example.com")
	}
	if result.Rows[0].Auth != "internal" {
		t.Errorf("Row 0 Auth = %q, want %q", result.Rows[0].Auth, "internal")
	}
}

func TestParseMemberCSV_NoHeader(t *testing.T) {
	csv := `John Doe,john@example.com,internal
Jane Smith,jane@example.com,google`

	result, err := ParseMemberCSV(strings.NewReader(csv), DefaultParseOptions())
	if err != nil {
		t.Fatalf("ParseMemberCSV() error = %v", err)
	}

	if len(result.Rows) != 2 {
		t.Errorf("ParseMemberCSV() got %d rows, want 2", len(result.Rows))
	}
}

func TestParseMemberCSV_BOMHandling(t *testing.T) {
	// CSV with UTF-8 BOM
	csv := "\ufeffFull Name,Email,Auth Method\nJohn Doe,john@example.com,internal"

	result, err := ParseMemberCSV(strings.NewReader(csv), DefaultParseOptions())
	if err != nil {
		t.Fatalf("ParseMemberCSV() error = %v", err)
	}

	if len(result.Rows) != 1 {
		t.Errorf("ParseMemberCSV() got %d rows, want 1", len(result.Rows))
	}

	if result.HasErrors() {
		t.Errorf("ParseMemberCSV() unexpected errors with BOM: %v", result.Errors)
	}
}

func TestParseMemberCSV_EmptyFile(t *testing.T) {
	result, err := ParseMemberCSV(strings.NewReader(""), DefaultParseOptions())
	if err != nil {
		t.Fatalf("ParseMemberCSV() error = %v", err)
	}

	if len(result.Rows) != 0 {
		t.Errorf("ParseMemberCSV() got %d rows, want 0", len(result.Rows))
	}
}

func TestParseMemberCSV_MissingFields(t *testing.T) {
	tests := []struct {
		name        string
		csv         string
		wantErrors  int
		errContains string
	}{
		{
			name:        "missing name",
			csv:         ",john@example.com,internal",
			wantErrors:  1,
			errContains: "missing full name",
		},
		{
			name:        "missing email",
			csv:         "John Doe,,internal",
			wantErrors:  1,
			errContains: "missing email",
		},
		{
			name:        "missing auth",
			csv:         "John Doe,john@example.com,",
			wantErrors:  1,
			errContains: "auth method",
		},
		{
			name:        "invalid email",
			csv:         "John Doe,not-an-email,internal",
			wantErrors:  1,
			errContains: "invalid email",
		},
		{
			name:        "invalid auth method",
			csv:         "John Doe,john@example.com,invalid",
			wantErrors:  1,
			errContains: "auth method",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseMemberCSV(strings.NewReader(tt.csv), DefaultParseOptions())
			if err != nil {
				t.Fatalf("ParseMemberCSV() error = %v", err)
			}

			if len(result.Errors) != tt.wantErrors {
				t.Errorf("ParseMemberCSV() got %d errors, want %d", len(result.Errors), tt.wantErrors)
			}

			if tt.wantErrors > 0 && !strings.Contains(result.Errors[0].Reason, tt.errContains) {
				t.Errorf("Error reason %q doesn't contain %q", result.Errors[0].Reason, tt.errContains)
			}
		})
	}
}

func TestParseMemberCSV_DuplicateEmails(t *testing.T) {
	csv := `John Doe,john@example.com,internal
Jane Doe,john@example.com,google`

	result, err := ParseMemberCSV(strings.NewReader(csv), DefaultParseOptions())
	if err != nil {
		t.Fatalf("ParseMemberCSV() error = %v", err)
	}

	if len(result.Errors) != 1 {
		t.Errorf("ParseMemberCSV() got %d errors, want 1 for duplicate", len(result.Errors))
	}

	if len(result.Errors) > 0 && !strings.Contains(result.Errors[0].Reason, "duplicate") {
		t.Errorf("Error reason %q doesn't mention duplicate", result.Errors[0].Reason)
	}
}

func TestParseMemberCSV_MaxRows(t *testing.T) {
	// Create CSV with more rows than limit
	var sb strings.Builder
	sb.WriteString("Name,Email,Auth\n")
	for i := 0; i < 10; i++ {
		sb.WriteString("User,user@example.com,internal\n")
	}

	opts := ParseOptions{MaxRows: 5}
	_, err := ParseMemberCSV(strings.NewReader(sb.String()), opts)

	if err != ErrTooManyRows {
		t.Errorf("ParseMemberCSV() error = %v, want ErrTooManyRows", err)
	}
}

func TestParseMemberCSV_SkipsEmptyRows(t *testing.T) {
	csv := `John Doe,john@example.com,internal

Jane Smith,jane@example.com,google

`

	result, err := ParseMemberCSV(strings.NewReader(csv), DefaultParseOptions())
	if err != nil {
		t.Fatalf("ParseMemberCSV() error = %v", err)
	}

	if len(result.Rows) != 2 {
		t.Errorf("ParseMemberCSV() got %d rows, want 2", len(result.Rows))
	}
}

func TestParseMemberCSV_AuthMethodNormalization(t *testing.T) {
	csv := `John Doe,john@example.com,INTERNAL
Jane Smith,jane@example.com,Google
Bob Wilson,bob@example.com,ClassLink`

	result, err := ParseMemberCSV(strings.NewReader(csv), DefaultParseOptions())
	if err != nil {
		t.Fatalf("ParseMemberCSV() error = %v", err)
	}

	if result.HasErrors() {
		t.Errorf("ParseMemberCSV() unexpected errors: %v", result.Errors)
	}

	// Auth methods should be normalized to lowercase
	if result.Rows[0].Auth != "internal" {
		t.Errorf("Auth not normalized: got %q, want %q", result.Rows[0].Auth, "internal")
	}
	if result.Rows[1].Auth != "google" {
		t.Errorf("Auth not normalized: got %q, want %q", result.Rows[1].Auth, "google")
	}
	if result.Rows[2].Auth != "classlink" {
		t.Errorf("Auth not normalized: got %q, want %q", result.Rows[2].Auth, "classlink")
	}
}

func TestParseResult_HasErrors(t *testing.T) {
	t.Run("no errors", func(t *testing.T) {
		r := &ParseResult{}
		if r.HasErrors() {
			t.Error("HasErrors() = true for empty errors")
		}
	})

	t.Run("with errors", func(t *testing.T) {
		r := &ParseResult{
			Errors: []RowError{{Line: 1, Reason: "test"}},
		}
		if !r.HasErrors() {
			t.Error("HasErrors() = false when errors present")
		}
	})
}

func TestParseResult_FormatErrorsHTML(t *testing.T) {
	t.Run("no errors", func(t *testing.T) {
		r := &ParseResult{}
		html := r.FormatErrorsHTML(5)
		if html != "" {
			t.Errorf("FormatErrorsHTML() = %q, want empty", html)
		}
	})

	t.Run("with errors", func(t *testing.T) {
		r := &ParseResult{
			Errors: []RowError{
				{Line: 1, Reason: "missing name", Raw: []string{"", "email@example.com", "internal"}},
				{Line: 2, Reason: "invalid email", Raw: []string{"John", "bad-email", "internal"}},
			},
		}
		html := r.FormatErrorsHTML(5)

		if !strings.Contains(string(html), "2 row(s) are invalid") {
			t.Error("FormatErrorsHTML() doesn't contain error count")
		}
		if !strings.Contains(string(html), "missing name") {
			t.Error("FormatErrorsHTML() doesn't contain error reason")
		}
	})

	t.Run("truncates to maxShow", func(t *testing.T) {
		r := &ParseResult{
			Errors: make([]RowError, 10),
		}
		for i := range r.Errors {
			r.Errors[i] = RowError{Line: i + 1, Reason: "error"}
		}

		html := r.FormatErrorsHTML(3)
		if !strings.Contains(string(html), "and 7 more") {
			t.Error("FormatErrorsHTML() doesn't show remaining count")
		}
	})
}

func TestDefaultParseOptions(t *testing.T) {
	opts := DefaultParseOptions()
	if opts.MaxRows != 0 {
		t.Errorf("DefaultParseOptions().MaxRows = %d, want 0 (unlimited)", opts.MaxRows)
	}
}

func TestConstants(t *testing.T) {
	if MaxUploadSize != 5<<20 {
		t.Errorf("MaxUploadSize = %d, want %d (5MB)", MaxUploadSize, 5<<20)
	}
	if MaxRows != 20000 {
		t.Errorf("MaxRows = %d, want 20000", MaxRows)
	}
}
