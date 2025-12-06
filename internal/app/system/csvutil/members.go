// internal/app/system/csvutil/members.go
package csvutil

import (
	"encoding/csv"
	"html/template"
	"io"
	"strings"
)

// MemberCSVRow is the normalized row produced by PreScanMemberCSV.
type MemberCSVRow struct {
	FullName string
	Email    string
	Auth     string // canonical lower-case
}

// PreScanMemberCSV reads all rows from r, skips a header if present,
// validates each row, and either returns normalized rows OR a formatted
// HTML error message (template.HTML) describing the first few bad lines.
// It never writes to a DB; it's safe to call before any mutations.
func PreScanMembersCSV(r io.Reader) (rows []MemberCSVRow, htmlErr template.HTML, err error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true

	// Pull first row to check header
	first, ferr := reader.Read()
	if ferr == io.EOF {
		first = nil
	} else if ferr != nil {
		return nil, template.HTML(template.HTMLEscapeString(ferr.Error())), nil
	}
	var raw [][]string
	if len(first) >= 2 &&
		(strings.EqualFold(strings.TrimSpace(first[0]), "full name") ||
			strings.EqualFold(strings.TrimSpace(first[0]), "name")) &&
		strings.EqualFold(strings.TrimSpace(first[1]), "email") {
		// header detected → skip
	} else if first != nil {
		raw = append(raw, first)
	}
	for {
		rec, e := reader.Read()
		if e == io.EOF {
			break
		}
		if e != nil || len(rec) == 0 {
			continue
		}
		raw = append(raw, rec)
	}

	type rowErr struct{ Email, Name, Auth, Reason string }
	var errs []rowErr
	allowed := map[string]bool{
		"internal": true, "google": true, "classlink": true, "clever": true, "microsoft": true,
	}
	normalize := func(rec []string) MemberCSVRow {
		var n, e, a string
		if len(rec) > 0 {
			n = strings.TrimSpace(rec[0])
		}
		if len(rec) > 1 {
			e = strings.TrimSpace(rec[1])
		}
		if len(rec) > 2 {
			a = strings.TrimSpace(rec[2])
		}
		return MemberCSVRow{FullName: n, Email: e, Auth: a}
	}

	for _, rec := range raw {
		row := normalize(rec)
		if row.FullName == "" && row.Email == "" && row.Auth == "" {
			continue
		}
		if row.FullName == "" {
			errs = append(errs, rowErr{
				Email: strings.ToLower(row.Email), Name: row.FullName, Auth: row.Auth, Reason: "missing full name",
			})
		}
		if strings.TrimSpace(row.Email) == "" {
			errs = append(errs, rowErr{
				Email: row.Email, Name: row.FullName, Auth: row.Auth, Reason: "missing email",
			})
		}
		a := strings.ToLower(strings.TrimSpace(row.Auth))
		if a == "" || !allowed[a] {
			errs = append(errs, rowErr{
				Email: strings.ToLower(row.Email), Name: row.FullName, Auth: row.Auth, Reason: "invalid or missing auth method",
			})
		} else {
			row.Auth = a
		}
		rows = append(rows, row)
	}

	if len(errs) > 0 {
		var b strings.Builder
		b.WriteString("Upload rejected: one or more rows are invalid.<br>")
		b.WriteString("Each row must have a Full Name, an Email, and a valid Auth Method.<br>")
		b.WriteString("Allowed auth methods (case-insensitive): internal, google, classlink, clever, microsoft.<br>")

		max := 5
		if len(errs) < max {
			max = len(errs)
		}
		if max > 0 {
			b.WriteString("Examples:<br>")
			for i := 0; i < max; i++ {
				e := errs[i]
				email := strings.TrimSpace(e.Email)
				if email == "" {
					email = "(no email on row)"
				}
				auth := strings.TrimSpace(e.Auth)
				if auth == "" {
					auth = "(missing)"
				}
				name := strings.TrimSpace(e.Name)
				if name == "" {
					name = "(missing)"
				}
				b.WriteString("• ")
				b.WriteString(template.HTMLEscapeString(email))
				b.WriteString(" | ")
				b.WriteString(template.HTMLEscapeString(name))
				b.WriteString(" | ")
				b.WriteString(template.HTMLEscapeString(auth))
				b.WriteString(" → ")
				b.WriteString(template.HTMLEscapeString(e.Reason))
				b.WriteString("<br>")
			}
		}
		return nil, template.HTML(b.String()), nil
	}

	return rows, "", nil
}
