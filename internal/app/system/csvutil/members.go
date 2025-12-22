// internal/app/system/csvutil/members.go
package csvutil

import (
	"encoding/csv"
	"errors"
	"fmt"
	"html/template"
	"io"
	"strconv"
	"strings"

	"github.com/dalemusser/stratahub/internal/app/system/inputval"
	norm "github.com/dalemusser/stratahub/internal/app/system/normalize"
)

// MemberRow represents a validated row from the CSV.
type MemberRow struct {
	FullName string
	Email    string
	Auth     string // canonical lower-case
}

// RowError represents a validation error for a specific row.
type RowError struct {
	Line   int
	Reason string
	Raw    []string // original values for display
}

// ParseResult holds the result of parsing a member CSV file.
type ParseResult struct {
	Rows   []MemberRow
	Errors []RowError
}

// ParseOptions configures CSV parsing behavior.
type ParseOptions struct {
	// MaxRows limits the number of data rows (0 = unlimited).
	MaxRows int
}

// DefaultParseOptions returns sensible defaults for CSV parsing.
func DefaultParseOptions() ParseOptions {
	return ParseOptions{
		MaxRows: 0, // unlimited
	}
}

// ErrTooManyRows is returned when the CSV exceeds MaxRows.
var ErrTooManyRows = errors.New("CSV file has too many rows")

// ParseMemberCSV reads and validates a member CSV file.
// It handles:
//   - BOM detection and removal
//   - Header row detection and skipping
//   - Row validation (full name, email, auth method)
//   - Parse error detection (malformed CSV)
//
// Returns structured results - callers can format errors as needed.
// Returns ErrTooManyRows if MaxRows is exceeded (when MaxRows > 0).
// Returns other errors for CSV parse failures.
func ParseMemberCSV(r io.Reader, opts ParseOptions) (ParseResult, error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1 // allow variable fields
	reader.TrimLeadingSpace = true

	var result ParseResult
	var parseErrors []string
	lineNum := 0

	// Read first row to check for header
	first, err := reader.Read()
	if err == io.EOF {
		return result, nil // empty file
	}
	if err != nil {
		return result, err
	}
	lineNum++

	// Handle BOM in first cell
	if len(first) > 0 {
		first[0] = strings.TrimPrefix(first[0], "\ufeff")
	}

	// Check if first row is a header
	isHeader := false
	if len(first) >= 2 {
		c0 := strings.ToLower(strings.TrimSpace(first[0]))
		c1 := strings.ToLower(strings.TrimSpace(first[1]))
		c2 := ""
		if len(first) > 2 {
			c2 = strings.ToLower(strings.TrimSpace(first[2]))
		}
		isHeader = (c0 == "full name" || c0 == "name") && c1 == "email"
		if !isHeader {
			isHeader = c1 == "email" && strings.Contains(c2, "auth")
		}
	}

	// Collect raw records
	var rawRecords [][]string
	if !isHeader {
		rawRecords = append(rawRecords, first)
	}

	// Read remaining rows
	for {
		rec, err := reader.Read()
		lineNum++
		if err == io.EOF {
			break
		}
		if err != nil {
			parseErrors = append(parseErrors, fmt.Sprintf("line %d: %s", lineNum, err.Error()))
			continue
		}
		if len(rec) == 0 {
			continue // skip empty lines
		}

		// Check row limit
		if opts.MaxRows > 0 && len(rawRecords) >= opts.MaxRows {
			return result, ErrTooManyRows
		}

		rawRecords = append(rawRecords, rec)
	}

	// If any parse errors, reject entire file
	if len(parseErrors) > 0 {
		for _, pe := range parseErrors {
			result.Errors = append(result.Errors, RowError{
				Line:   0, // parse errors don't have clean line numbers
				Reason: pe,
			})
		}
		return result, nil
	}

	// Track seen emails to detect duplicates within the CSV
	seenEmails := make(map[string]int) // email -> first line number

	// Validate each row
	for i, rec := range rawRecords {
		// Line number: +1 for 1-based, +1 if header was skipped
		line := i + 1
		if isHeader {
			line++
		}

		row := extractFields(rec)

		// Skip completely empty rows
		if row.FullName == "" && row.Email == "" && row.Auth == "" {
			continue
		}

		// Validate fields
		hasError := false

		if row.FullName == "" {
			result.Errors = append(result.Errors, RowError{
				Line:   line,
				Reason: "missing full name",
				Raw:    rec,
			})
			hasError = true
		}

		if strings.TrimSpace(row.Email) == "" {
			result.Errors = append(result.Errors, RowError{
				Line:   line,
				Reason: "missing email",
				Raw:    rec,
			})
			hasError = true
		} else if !inputval.IsValidEmail(row.Email) {
			result.Errors = append(result.Errors, RowError{
				Line:   line,
				Reason: "invalid email format",
				Raw:    rec,
			})
			hasError = true
		} else {
			// Check for duplicate email within CSV
			emailLower := strings.ToLower(row.Email)
			if firstLine, seen := seenEmails[emailLower]; seen {
				result.Errors = append(result.Errors, RowError{
					Line:   line,
					Reason: fmt.Sprintf("duplicate email (first appears on row %d)", firstLine),
					Raw:    rec,
				})
				hasError = true
			} else {
				seenEmails[emailLower] = line
			}
		}

		authNorm := norm.AuthMethod(row.Auth)
		if authNorm == "" || !inputval.IsValidAuthMethod(authNorm) {
			result.Errors = append(result.Errors, RowError{
				Line:   line,
				Reason: "invalid or missing auth method",
				Raw:    rec,
			})
			hasError = true
		} else {
			row.Auth = authNorm
		}

		// Only add valid rows
		if !hasError {
			result.Rows = append(result.Rows, row)
		}
	}

	return result, nil
}

// extractFields extracts and trims fields from a CSV record.
func extractFields(rec []string) MemberRow {
	var row MemberRow
	if len(rec) > 0 {
		row.FullName = strings.TrimSpace(rec[0])
	}
	if len(rec) > 1 {
		row.Email = strings.TrimSpace(rec[1])
	}
	if len(rec) > 2 {
		row.Auth = strings.TrimSpace(rec[2])
	}
	return row
}

// HasErrors returns true if there are any validation errors.
func (r *ParseResult) HasErrors() bool {
	return len(r.Errors) > 0
}

// FormatErrorsHTML formats validation errors as HTML for display.
// This is a convenience method for handlers that want HTML output.
func (r *ParseResult) FormatErrorsHTML(maxShow int) template.HTML {
	if len(r.Errors) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("Upload rejected: ")
	b.WriteString(strconv.Itoa(len(r.Errors)))
	b.WriteString(" row(s) are invalid.<br>")
	b.WriteString("Each row must have a Full Name, an Email, and a valid Auth Method.<br>")
	b.WriteString("Allowed auth methods: ")
	b.WriteString(strings.Join(inputval.AllowedAuthMethodsList(), ", "))
	b.WriteString(".<br>")

	if maxShow <= 0 {
		maxShow = 5
	}
	if len(r.Errors) < maxShow {
		maxShow = len(r.Errors)
	}

	if maxShow > 0 {
		b.WriteString("Examples:<br>")
		for i := 0; i < maxShow; i++ {
			e := r.Errors[i]
			b.WriteString("• ")
			if e.Line > 0 {
				b.WriteString("row ")
				b.WriteString(strconv.Itoa(e.Line))
				b.WriteString(": ")
			}
			b.WriteString(template.HTMLEscapeString(e.Reason))
			if len(e.Raw) > 0 {
				b.WriteString(" | values: ")
				b.WriteString(template.HTMLEscapeString(strings.Join(e.Raw, ", ")))
			}
			b.WriteString("<br>")
		}
	}

	if len(r.Errors) > maxShow {
		b.WriteString("... and ")
		b.WriteString(strconv.Itoa(len(r.Errors) - maxShow))
		b.WriteString(" more.")
	}

	return template.HTML(b.String())
}
