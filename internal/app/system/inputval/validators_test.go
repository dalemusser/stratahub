package inputval

import "testing"

func TestIsValidAuthMethod(t *testing.T) {
	tests := []struct {
		method string
		want   bool
	}{
		// Valid methods
		{"internal", true},
		{"google", true},
		{"classlink", true},
		{"clever", true},
		{"microsoft", true},

		// Valid methods - case insensitive
		{"INTERNAL", true},
		{"Google", true},
		{"ClassLink", true},
		{"CLEVER", true},
		{"Microsoft", true},

		// Valid with whitespace
		{"  internal  ", true},
		{"\tgoogle\t", true},

		// Invalid methods
		{"", false},
		{"   ", false},
		{"facebook", false},
		{"oauth", false},
		{"saml", false},
		{"ldap", false},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			got := IsValidAuthMethod(tt.method)
			if got != tt.want {
				t.Errorf("IsValidAuthMethod(%q) = %v, want %v", tt.method, got, tt.want)
			}
		})
	}
}

func TestAllowedAuthMethodsList(t *testing.T) {
	list := AllowedAuthMethodsList()

	if len(list) != 5 {
		t.Errorf("AllowedAuthMethodsList() has %d items, want 5", len(list))
	}

	expected := []string{"internal", "google", "classlink", "clever", "microsoft"}
	for i, want := range expected {
		if list[i] != want {
			t.Errorf("AllowedAuthMethodsList()[%d] = %q, want %q", i, list[i], want)
		}
	}
}

func TestIsValidHTTPURL(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		// Valid URLs
		{"http://example.com", true},
		{"https://example.com", true},
		{"http://example.com/path", true},
		{"https://example.com/path?query=1", true},
		{"http://localhost:8080", true},
		{"https://sub.domain.example.com", true},

		// Valid with whitespace (trimmed)
		{"  https://example.com  ", true},

		// Invalid URLs
		{"", false},
		{"   ", false},
		{"ftp://example.com", false},
		{"mailto:user@example.com", false},
		{"example.com", false},
		{"//example.com", false},
		{"not a url", false},
		{"file:///path/to/file", false},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			got := IsValidHTTPURL(tt.url)
			if got != tt.want {
				t.Errorf("IsValidHTTPURL(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

func TestIsValidObjectID(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		// Valid ObjectIDs (24 hex characters)
		{"507f1f77bcf86cd799439011", true},
		{"000000000000000000000000", true},
		{"ffffffffffffffffffffffff", true},
		{"FFFFFFFFFFFFFFFFFFFFFFFF", true}, // uppercase hex is valid

		// Valid with whitespace (trimmed)
		{"  507f1f77bcf86cd799439011  ", true},

		// Invalid ObjectIDs
		{"", false},
		{"   ", false},
		{"507f1f77bcf86cd79943901", false},   // too short (23 chars)
		{"507f1f77bcf86cd7994390111", false}, // too long (25 chars)
		{"507f1f77bcf86cd79943901g", false},  // invalid hex char
		{"not-a-valid-id", false},
		{"12345", false},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			got := IsValidObjectID(tt.id)
			if got != tt.want {
				t.Errorf("IsValidObjectID(%q) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	type TestInput struct {
		Name  string `validate:"required,max=10" label:"Full name"`
		Email string `validate:"required,email" label:"Email address"`
	}

	tests := []struct {
		name       string
		input      TestInput
		wantErrors bool
		wantFirst  string
	}{
		{
			name:       "valid input",
			input:      TestInput{Name: "John", Email: "john@example.com"},
			wantErrors: false,
		},
		{
			name:       "missing name",
			input:      TestInput{Name: "", Email: "john@example.com"},
			wantErrors: true,
			wantFirst:  "Full name is required.",
		},
		{
			name:       "name too long",
			input:      TestInput{Name: "VeryLongNameThatExceedsLimit", Email: "john@example.com"},
			wantErrors: true,
			wantFirst:  "Full name must be at most 10 characters.",
		},
		{
			name:       "invalid email",
			input:      TestInput{Name: "John", Email: "not-an-email"},
			wantErrors: true,
			wantFirst:  "A valid email address is required.",
		},
		{
			name:       "missing both",
			input:      TestInput{Name: "", Email: ""},
			wantErrors: true,
			wantFirst:  "Full name is required.", // First error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Validate(tt.input)

			if result.HasErrors() != tt.wantErrors {
				t.Errorf("Validate() HasErrors = %v, want %v", result.HasErrors(), tt.wantErrors)
			}

			if tt.wantErrors && result.First() != tt.wantFirst {
				t.Errorf("Validate() First() = %q, want %q", result.First(), tt.wantFirst)
			}
		})
	}
}

func TestResult_All(t *testing.T) {
	t.Run("no errors", func(t *testing.T) {
		r := &Result{}
		if r.All() != "" {
			t.Errorf("All() = %q, want empty", r.All())
		}
	})

	t.Run("one error", func(t *testing.T) {
		r := &Result{
			Errors: []FieldError{{Message: "Error 1"}},
		}
		if r.All() != "Error 1" {
			t.Errorf("All() = %q, want %q", r.All(), "Error 1")
		}
	})

	t.Run("multiple errors", func(t *testing.T) {
		r := &Result{
			Errors: []FieldError{
				{Message: "Error 1"},
				{Message: "Error 2"},
			},
		}
		want := "Error 1; Error 2"
		if r.All() != want {
			t.Errorf("All() = %q, want %q", r.All(), want)
		}
	})
}

func TestResult_First(t *testing.T) {
	t.Run("no errors", func(t *testing.T) {
		r := &Result{}
		if r.First() != "" {
			t.Errorf("First() = %q, want empty", r.First())
		}
	})

	t.Run("with errors", func(t *testing.T) {
		r := &Result{
			Errors: []FieldError{
				{Message: "First error"},
				{Message: "Second error"},
			},
		}
		if r.First() != "First error" {
			t.Errorf("First() = %q, want %q", r.First(), "First error")
		}
	})
}

func TestValidate_CustomRules(t *testing.T) {
	type AuthInput struct {
		Method string `validate:"required,authmethod" label:"Auth method"`
	}

	type URLInput struct {
		URL string `validate:"required,httpurl" label:"Launch URL"`
	}

	type IDInput struct {
		ID string `validate:"required,objectid" label:"Resource ID"`
	}

	t.Run("valid auth method", func(t *testing.T) {
		result := Validate(AuthInput{Method: "google"})
		if result.HasErrors() {
			t.Errorf("Validate(valid auth) has errors: %v", result.Errors)
		}
	})

	t.Run("invalid auth method", func(t *testing.T) {
		result := Validate(AuthInput{Method: "invalid"})
		if !result.HasErrors() {
			t.Error("Validate(invalid auth) should have errors")
		}
	})

	t.Run("valid URL", func(t *testing.T) {
		result := Validate(URLInput{URL: "https://example.com"})
		if result.HasErrors() {
			t.Errorf("Validate(valid URL) has errors: %v", result.Errors)
		}
	})

	t.Run("invalid URL", func(t *testing.T) {
		result := Validate(URLInput{URL: "not-a-url"})
		if !result.HasErrors() {
			t.Error("Validate(invalid URL) should have errors")
		}
	})

	t.Run("valid ObjectID", func(t *testing.T) {
		result := Validate(IDInput{ID: "507f1f77bcf86cd799439011"})
		if result.HasErrors() {
			t.Errorf("Validate(valid ID) has errors: %v", result.Errors)
		}
	})

	t.Run("invalid ObjectID", func(t *testing.T) {
		result := Validate(IDInput{ID: "invalid-id"})
		if !result.HasErrors() {
			t.Error("Validate(invalid ID) should have errors")
		}
	})
}
