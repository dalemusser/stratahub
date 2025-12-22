package normalize

import "testing"

func TestEmail(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"user@example.com", "user@example.com"},
		{"USER@EXAMPLE.COM", "user@example.com"},
		{"  User@Example.Com  ", "user@example.com"},
		{"", ""},
		{"   ", ""},
		{"Mixed.Case@Domain.ORG", "mixed.case@domain.org"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := Email(tt.input)
			if got != tt.want {
				t.Errorf("Email(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"John Doe", "John Doe"},
		{"  John Doe  ", "John Doe"},
		{"", ""},
		{"   ", ""},
		{"UPPERCASE NAME", "UPPERCASE NAME"}, // Name preserves case
		{"lowercase name", "lowercase name"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := Name(tt.input)
			if got != tt.want {
				t.Errorf("Name(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestAuthMethod(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"internal", "internal"},
		{"INTERNAL", "internal"},
		{"  Google  ", "google"},
		{"CLASSLINK", "classlink"},
		{"", ""},
		{"   ", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := AuthMethod(tt.input)
			if got != tt.want {
				t.Errorf("AuthMethod(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestStatus(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"active", "active"},
		{"ACTIVE", "active"},
		{"  Disabled  ", "disabled"},
		{"", ""},
		{"   ", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := Status(tt.input)
			if got != tt.want {
				t.Errorf("Status(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestRole(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"admin", "admin"},
		{"ADMIN", "admin"},
		{"  Leader  ", "leader"},
		{"MEMBER", "member"},
		{"", ""},
		{"   ", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := Role(tt.input)
			if got != tt.want {
				t.Errorf("Role(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestQueryParam(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"search term", "search term"},
		{"  trimmed  ", "trimmed"},
		{"", ""},
		{"   ", ""},
		{"UPPERCASE", "UPPERCASE"}, // Preserves case
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := QueryParam(tt.input)
			if got != tt.want {
				t.Errorf("QueryParam(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestOrgID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"507f1f77bcf86cd799439011", "507f1f77bcf86cd799439011"},
		{"  507f1f77bcf86cd799439011  ", "507f1f77bcf86cd799439011"},
		{"all", ""},      // "all" converts to empty
		{"ALL", ""},      // case-insensitive
		{"  All  ", ""},  // with whitespace
		{"", ""},
		{"   ", ""},
		{"somevalue", "somevalue"}, // non-"all" values preserved
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := OrgID(tt.input)
			if got != tt.want {
				t.Errorf("OrgID(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
