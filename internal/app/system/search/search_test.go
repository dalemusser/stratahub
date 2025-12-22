package search

import "testing"

func TestEmailPivotOK(t *testing.T) {
	tests := []struct {
		name   string
		search string
		status string
		hasOrg bool
		want   bool
	}{
		// Should pivot - all conditions met
		{"email search with active status and org", "user@example.com", "active", true, true},
		{"email search with disabled status and org", "user@", "disabled", true, true},
		{"partial email with active and org", "@domain", "active", true, true},

		// Should NOT pivot - missing @
		{"name search with active and org", "john doe", "active", true, false},
		{"empty search with active and org", "", "active", true, false},

		// Should NOT pivot - status not constrained
		{"email search with empty status and org", "user@example.com", "", true, false},
		{"email search with all status and org", "user@example.com", "all", true, false},

		// Should NOT pivot - no org
		{"email search with active but no org", "user@example.com", "active", false, false},
		{"email search with disabled but no org", "user@example.com", "disabled", false, false},

		// Case insensitivity for status
		{"email with ACTIVE status", "user@example.com", "ACTIVE", true, true},
		{"email with Active status", "user@example.com", "Active", true, true},
		{"email with DISABLED status", "user@example.com", "DISABLED", true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EmailPivotOK(tt.search, tt.status, tt.hasOrg)
			if got != tt.want {
				t.Errorf("EmailPivotOK(%q, %q, %v) = %v, want %v",
					tt.search, tt.status, tt.hasOrg, got, tt.want)
			}
		})
	}
}

func TestEmailPivotNoOrgOK(t *testing.T) {
	tests := []struct {
		name   string
		search string
		status string
		want   bool
	}{
		// Should pivot - email search with constrained status
		{"email search with active status", "user@example.com", "active", true},
		{"email search with disabled status", "user@", "disabled", true},
		{"partial email with active", "@domain", "active", true},

		// Should NOT pivot - missing @
		{"name search with active", "john doe", "active", false},
		{"empty search with active", "", "active", false},

		// Should NOT pivot - status not constrained
		{"email search with empty status", "user@example.com", "", false},
		{"email search with all status", "user@example.com", "all", false},
		{"email search with invalid status", "user@example.com", "pending", false},

		// Case insensitivity for status
		{"email with ACTIVE status", "user@example.com", "ACTIVE", true},
		{"email with Disabled status", "user@example.com", "Disabled", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EmailPivotNoOrgOK(tt.search, tt.status)
			if got != tt.want {
				t.Errorf("EmailPivotNoOrgOK(%q, %q) = %v, want %v",
					tt.search, tt.status, got, tt.want)
			}
		})
	}
}
