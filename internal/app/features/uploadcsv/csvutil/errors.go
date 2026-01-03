package csvutil

import (
	"html/template"
	"strconv"
	"strings"
)

// FormatParseErrors formats CSV parsing errors for display in the UI.
// If maxShow is <= 0, it defaults to 5. All errors are shown if there
// are fewer than maxShow errors.
func FormatParseErrors(errors []RowError, maxShow int) template.HTML {
	var b strings.Builder
	b.WriteString("CSV file contains errors. Please fix and re-upload.<br><br>")

	if maxShow <= 0 {
		maxShow = 5
	}
	if len(errors) < maxShow {
		maxShow = len(errors)
	}

	for i := 0; i < maxShow; i++ {
		e := errors[i]
		b.WriteString("• ")
		if e.Line > 0 {
			b.WriteString("Row ")
			b.WriteString(strings.TrimSpace(strings.Split(strings.Split(e.Reason, ":")[0], " ")[0]))
			if e.Line > 0 {
				b.WriteString(" (line ")
				b.WriteString(strconv.Itoa(e.Line))
				b.WriteString(")")
			}
			b.WriteString(": ")
		}
		b.WriteString(e.Reason)
		b.WriteString("<br>")
	}

	if len(errors) > maxShow {
		b.WriteString("<br>... and ")
		b.WriteString(strconv.Itoa(len(errors) - maxShow))
		b.WriteString(" more errors.")
	}

	return template.HTML(b.String())
}

// FormatConflictErrors formats organization conflict errors for display.
// Each conflict shows the login ID and the organization it belongs to.
func FormatConflictErrors(conflicts []LoginConflict) template.HTML {
	var b strings.Builder
	b.WriteString("Some login IDs already exist in a different organization. ")
	b.WriteString("Cannot import these members:<br><br>")

	for _, c := range conflicts {
		b.WriteString("• ")
		b.WriteString(template.HTMLEscapeString(c.LoginID))
		b.WriteString(" (")
		b.WriteString(template.HTMLEscapeString(c.OrgName))
		b.WriteString(")<br>")
	}

	return template.HTML(b.String())
}
