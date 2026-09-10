package reports

import (
	"reflect"
	"testing"
)

func TestNormalizeIdentity(t *testing.T) {
	cases := map[string]string{
		"":               IdentityBoth,
		"both":           IdentityBoth,
		"deidentified":   IdentityDeidentified,
		"identified":     IdentityIdentified,
		" Deidentified ": IdentityDeidentified,
		"hex":            IdentityBoth, // not a report selection; falls back
		"bogus":          IdentityBoth,
	}
	for in, want := range cases {
		if got := normalizeIdentity(in); got != want {
			t.Errorf("normalizeIdentity(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSelectColumns(t *testing.T) {
	all := []string{"workspace", "workspace_id", "user_id", "full_name", "login_id", "email", "organization", "organization_id", "group", "group_id", "leaders", "status"}

	tests := []struct {
		identity   string
		wantHeader []string
		hex, human bool
	}{
		{IdentityBoth, all, true, true},
		{IdentityDeidentified, []string{"workspace_id", "user_id", "organization_id", "group_id", "status"}, true, false},
		{IdentityIdentified, []string{"workspace", "full_name", "login_id", "email", "organization", "group", "leaders", "status"}, false, true},
	}
	for _, tc := range tests {
		sel := selectColumns(tc.identity)
		if got := sel.Header(); !reflect.DeepEqual(got, tc.wantHeader) {
			t.Errorf("%s: Header() = %v, want %v", tc.identity, got, tc.wantHeader)
		}
		if sel.Hex != tc.hex || sel.Human != tc.human {
			t.Errorf("%s: Hex=%v Human=%v, want Hex=%v Human=%v", tc.identity, sel.Hex, sel.Human, tc.hex, tc.human)
		}
	}

	// Every column belongs to at least one export, and the full row maps
	// onto the selection in order.
	full := make([]string, len(csvColumns))
	for i, c := range csvColumns {
		if !c.Hex && !c.Human {
			t.Errorf("column %q is in no export", c.Name)
		}
		full[i] = "v:" + c.Name
	}
	got := selectColumns(IdentityDeidentified).Row(full)
	want := []string{"v:workspace_id", "v:user_id", "v:organization_id", "v:group_id", "v:status"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Row() = %v, want %v", got, want)
	}
	if got := selectColumns(IdentityBoth).Row(full); !reflect.DeepEqual(got, full) {
		t.Errorf("both: Row() should be the full row, got %v", got)
	}
}

func TestIdentityLabelsAndSuffix(t *testing.T) {
	for _, o := range identityOptions {
		if identityLabel(o.Value) != o.Label {
			t.Errorf("identityLabel(%q) = %q, want %q", o.Value, identityLabel(o.Value), o.Label)
		}
		if o.PII != (o.Value != IdentityDeidentified) {
			t.Errorf("%s: PII flag %v is inconsistent with the column set", o.Value, o.PII)
		}
	}
	if s := identityFilenameSuffix(IdentityBoth); s != "" {
		t.Errorf("both: suffix %q, want none (the default keeps the plain filename)", s)
	}
	if s := identityFilenameSuffix(IdentityDeidentified); s != "_deidentified" {
		t.Errorf("deidentified: suffix %q", s)
	}
	if s := identityFilenameSuffix(IdentityIdentified); s != "_identified" {
		t.Errorf("identified: suffix %q", s)
	}
}
