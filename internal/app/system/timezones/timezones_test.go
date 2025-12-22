package timezones

import "testing"

func TestLoad(t *testing.T) {
	err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestAll(t *testing.T) {
	zones, err := All()
	if err != nil {
		t.Fatalf("All() error = %v", err)
	}
	if len(zones) == 0 {
		t.Error("All() returned empty zones list")
	}

	// Check that zones have required fields
	for _, z := range zones {
		if z.ID == "" {
			t.Error("Zone has empty ID")
		}
		if z.Label == "" {
			t.Errorf("Zone %q has empty Label", z.ID)
		}
	}
}

func TestLabel(t *testing.T) {
	// Ensure data is loaded
	if err := Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	tests := []struct {
		name    string
		id      string
		wantID  bool // if true, expect the ID back (not found case)
	}{
		{"known timezone", "America/New_York", false},
		{"known timezone UTC", "UTC", false},
		{"unknown timezone", "Invalid/Timezone", true},
		{"empty string", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Label(tt.id)
			if tt.wantID {
				if got != tt.id {
					t.Errorf("Label(%q) = %q, want %q (ID returned for unknown)", tt.id, got, tt.id)
				}
			} else {
				if got == tt.id {
					// For known timezones, label should differ from ID
					// (unless the label happens to be the same, which is rare)
					// This is a weak check - mainly ensuring no error
				}
				if got == "" {
					t.Errorf("Label(%q) returned empty string for known timezone", tt.id)
				}
			}
		})
	}
}

func TestValid(t *testing.T) {
	// Ensure data is loaded
	if err := Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	tests := []struct {
		id   string
		want bool
	}{
		{"America/New_York", true},
		{"UTC", true},
		{"Europe/London", true},
		{"Invalid/Timezone", false},
		{"", false},
		{"Not_A_Zone", false},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			got := Valid(tt.id)
			if got != tt.want {
				t.Errorf("Valid(%q) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

func TestGroups(t *testing.T) {
	groups, err := Groups()
	if err != nil {
		t.Fatalf("Groups() error = %v", err)
	}
	if len(groups) == 0 {
		t.Error("Groups() returned empty groups list")
	}

	// Check that groups have regions and zones
	for _, g := range groups {
		if g.Region == "" {
			t.Error("Group has empty Region")
		}
		if len(g.Zones) == 0 {
			t.Errorf("Group %q has no zones", g.Region)
		}
	}

	// Verify groups are sorted by region
	for i := 1; i < len(groups); i++ {
		if groups[i].Region < groups[i-1].Region {
			t.Errorf("Groups not sorted: %q comes after %q", groups[i].Region, groups[i-1].Region)
		}
	}
}

func TestZonesWithinGroupsSorted(t *testing.T) {
	groups, err := Groups()
	if err != nil {
		t.Fatalf("Groups() error = %v", err)
	}

	for _, g := range groups {
		for i := 1; i < len(g.Zones); i++ {
			if g.Zones[i].Label < g.Zones[i-1].Label {
				t.Errorf("Zones in group %q not sorted: %q comes after %q",
					g.Region, g.Zones[i].Label, g.Zones[i-1].Label)
			}
		}
	}
}
