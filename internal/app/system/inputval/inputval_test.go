package inputval

import "testing"

func TestIsValidEmail(t *testing.T) {
	tests := []struct {
		email string
		want  bool
	}{
		// Valid emails
		{"user@example.com", true},
		{"user.name@example.com", true},
		{"user+tag@example.com", true},
		{"user@subdomain.example.com", true},
		{"user123@example.co.uk", true},
		{"a@b.co", true},
		{"user@localhost", true},  // RFC 5322 allows single-label domains
		{"admin@mailserver", true}, // useful for dev/test environments

		// Invalid emails - empty/whitespace
		{"", false},
		{"   ", false},

		// Invalid emails - missing parts
		{"user", false},
		{"user@", false},
		{"@example.com", false},

		// Invalid emails - bad format (previously allowed by weak regex)
		{".user@example.com", false},   // leading dot in local
		{"user.@example.com", false},   // trailing dot in local
		{"user..name@example.com", false}, // consecutive dots
		{"user@.example.com", false},   // leading dot in domain
		{"user@example..com", false},   // consecutive dots in domain

		// Invalid emails - display name format (should be rejected)
		{"User Name <user@example.com>", false},

		// Invalid emails - other malformed
		{"user @example.com", false},  // space in local
		{"user@ example.com", false},  // space after @
		{"user@exam ple.com", false},  // space in domain
	}

	for _, tt := range tests {
		t.Run(tt.email, func(t *testing.T) {
			got := IsValidEmail(tt.email)
			if got != tt.want {
				t.Errorf("IsValidEmail(%q) = %v, want %v", tt.email, got, tt.want)
			}
		})
	}
}
