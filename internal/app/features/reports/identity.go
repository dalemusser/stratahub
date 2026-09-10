// internal/app/features/reports/identity.go
package reports

// Identity selection for the Members Report export.
//
// The CSV carries two kinds of identity. The hex columns (workspace_id,
// user_id, organization_id, group_id) are opaque, immutable ObjectIDs: the
// durable join keys that de-identified data is keyed on. The human columns
// (workspace, full_name, login_id, email, organization, group, leaders) are
// names and logins: PII. The "identity" query parameter on /reports/members
// and /reports/members.csv picks which of them the export includes, so a
// de-identified roster can be produced without a name ever being written to
// the file. The column sets mirror the resource URL identity schemes (hex /
// human / both) described in docs/resource-identification/.

import "strings"

const (
	// IdentityDeidentified exports the hex ID columns only (plus status).
	IdentityDeidentified = "deidentified"
	// IdentityIdentified exports the name, login, and email columns only
	// (plus status). PII.
	IdentityIdentified = "identified"
	// IdentityBoth exports every column: the full crosswalk. PII.
	IdentityBoth = "both"
)

// DefaultIdentity is the selection used when the parameter is absent or not
// recognized. "both" is the complete crosswalk the report has always produced.
const DefaultIdentity = IdentityBoth

// identityOption describes one selector choice on the page.
type identityOption struct {
	Value string
	Label string
	PII   bool
}

// identityOptions lists the selector choices in display order.
var identityOptions = []identityOption{
	{Value: IdentityDeidentified, Label: "De-identified — hex IDs only", PII: false},
	{Value: IdentityIdentified, Label: "Identified — names, logins, emails (PII)", PII: true},
	{Value: IdentityBoth, Label: "Both — hex IDs and names (PII)", PII: true},
}

// normalizeIdentity returns a valid identity selection, falling back to
// DefaultIdentity for an empty or unrecognized value.
func normalizeIdentity(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case IdentityDeidentified:
		return IdentityDeidentified
	case IdentityIdentified:
		return IdentityIdentified
	case IdentityBoth:
		return IdentityBoth
	}
	return DefaultIdentity
}

// identityLabel returns the selector label for a normalized selection.
func identityLabel(identity string) string {
	for _, o := range identityOptions {
		if o.Value == identity {
			return o.Label
		}
	}
	return identity
}

// identityFilenameSuffix is appended to the download filename's label so a
// de-identified (or names-only) file is recognizable by name. The default
// selection keeps the plain filename.
func identityFilenameSuffix(identity string) string {
	switch identity {
	case IdentityDeidentified:
		return "_deidentified"
	case IdentityIdentified:
		return "_identified"
	}
	return ""
}

// csvColumn is one Members Report column and which exports include it.
type csvColumn struct {
	Name  string
	Hex   bool // part of the de-identified export
	Human bool // part of the identified export
}

// csvColumns is the complete column list in output order. The "both" export
// writes all of them; the other selections drop columns but keep this order,
// so a column always sits in the same position relative to its neighbours.
var csvColumns = []csvColumn{
	{Name: "workspace", Human: true},
	{Name: "workspace_id", Hex: true},
	{Name: "user_id", Hex: true},
	{Name: "full_name", Human: true},
	{Name: "login_id", Human: true},
	{Name: "email", Human: true},
	{Name: "organization", Human: true},
	{Name: "organization_id", Hex: true},
	{Name: "group", Human: true},
	{Name: "group_id", Hex: true},
	{Name: "leaders", Human: true},
	{Name: "status", Hex: true, Human: true},
}

// columnSelection is the resolved set of columns for one export.
type columnSelection struct {
	Identity string
	// Hex reports whether the hex ID columns are included.
	Hex bool
	// Human reports whether the name, login, and email columns are included,
	// i.e. whether the export needs any names looked up at all.
	Human   bool
	indexes []int // positions in csvColumns, ascending
}

// selectColumns resolves the columns for a normalized identity selection.
func selectColumns(identity string) columnSelection {
	sel := columnSelection{Identity: identity}
	switch identity {
	case IdentityDeidentified:
		sel.Hex = true
	case IdentityIdentified:
		sel.Human = true
	default:
		sel.Hex, sel.Human = true, true
	}
	for i, c := range csvColumns {
		if (sel.Hex && c.Hex) || (sel.Human && c.Human) {
			sel.indexes = append(sel.indexes, i)
		}
	}
	return sel
}

// Header returns the selected column names in output order.
func (s columnSelection) Header() []string {
	out := make([]string, 0, len(s.indexes))
	for _, i := range s.indexes {
		out = append(out, csvColumns[i].Name)
	}
	return out
}

// Row projects a full-width row (one value per csvColumns entry, in order)
// onto the selected columns.
func (s columnSelection) Row(full []string) []string {
	out := make([]string, 0, len(s.indexes))
	for _, i := range s.indexes {
		out = append(out, full[i])
	}
	return out
}
