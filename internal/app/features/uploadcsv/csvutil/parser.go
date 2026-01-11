// internal/app/features/uploadcsv/csvutil/parser.go
package csvutil

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import (
	"encoding/csv"
	"fmt"
	"io"
	"strings"

	"github.com/dalemusser/stratahub/internal/app/system/authutil"
	"github.com/dalemusser/stratahub/internal/app/system/inputval"
	norm "github.com/dalemusser/stratahub/internal/app/system/normalize"
)

// ParsedMember represents a validated member row from CSV with all auth fields.
type ParsedMember struct {
	FullName     string
	AuthMethod   string  // canonical lower-case (email, google, microsoft, password, clever, classlink, schoology)
	LoginID      string  // effective login identity
	Email        *string // optional email (nil if not provided)
	AuthReturnID *string // optional auth_return_id (nil if not provided)
	TempPassword *string // optional temp password for password auth (nil if not provided)
}

// ParsedResult holds the result of parsing a member CSV file with the new format.
type ParsedResult struct {
	Members []ParsedMember
	Errors  []RowError
}

// HasErrors returns true if there are any validation errors.
func (r *ParsedResult) HasErrors() bool {
	return len(r.Errors) > 0
}

// ParseMembersCSV parses a CSV file with the new format:
//
// Format by auth method:
//   - email/google/microsoft: full_name,auth_type,email
//   - password: full_name,password,login_id,temp_password[,email]
//   - clever/classlink/schoology (3 fields): full_name,auth_type,email
//     (email is used as login_id, auth_return_id, and email)
//   - clever/classlink/schoology (4+ fields): full_name,auth_type,login_id,auth_return_id[,email]
//
// Returns ErrTooManyRows if MaxRows is exceeded (when MaxRows > 0).
func ParseMembersCSV(r io.Reader, opts ParseOptions) (ParsedResult, error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1 // allow variable fields
	reader.TrimLeadingSpace = true

	var result ParsedResult
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
	isHeader := detectHeader(first)

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
				Line:   0,
				Reason: pe,
			})
		}
		return result, nil
	}

	// Track seen login IDs to detect duplicates within the CSV
	seenLoginIDs := make(map[string]int) // loginID -> first line number

	// Validate each row
	for i, rec := range rawRecords {
		// Line number: +1 for 1-based, +1 if header was skipped
		line := i + 1
		if isHeader {
			line++
		}

		member, rowErr := parseRow(rec, line)
		if rowErr != nil {
			result.Errors = append(result.Errors, *rowErr)
			continue
		}

		// Skip completely empty rows
		if member == nil {
			continue
		}

		// Check for duplicate login ID within CSV
		loginIDLower := strings.ToLower(member.LoginID)
		if firstLine, seen := seenLoginIDs[loginIDLower]; seen {
			result.Errors = append(result.Errors, RowError{
				Line:   line,
				Reason: fmt.Sprintf("duplicate login ID (first appears on row %d)", firstLine),
				Raw:    rec,
			})
			continue
		}
		seenLoginIDs[loginIDLower] = line

		result.Members = append(result.Members, *member)
	}

	return result, nil
}

// detectHeader checks if the first row looks like a header row.
func detectHeader(rec []string) bool {
	return isHeaderRow(rec)
}

// isHeaderRow checks if a row is a header row by examining the second column.
// A row is considered a header if the second column contains header-like text
// (e.g., "auth_type", "auth_method") rather than an actual auth method value
// (e.g., "google", "password"). This allows header rows anywhere in the file.
func isHeaderRow(rec []string) bool {
	if len(rec) < 2 {
		return false
	}

	c0 := strings.ToLower(strings.TrimSpace(rec[0]))
	c1 := strings.ToLower(strings.TrimSpace(rec[1]))

	// Check common header patterns in first column
	nameHeaderWords := []string{"full_name", "fullname", "full name", "name"}
	for _, hw := range nameHeaderWords {
		if c0 == hw {
			return true
		}
	}

	// Check if second column looks like an auth header word rather than an auth method value
	authHeaderWords := []string{"auth", "auth_type", "authtype", "auth_method", "authmethod", "authentication", "type"}
	for _, hw := range authHeaderWords {
		if c1 == hw || strings.Contains(c1, hw) {
			return true
		}
	}

	return false
}

// parseRow parses a single CSV row and returns a ParsedMember or error.
// Returns nil,nil for empty rows or header rows that should be skipped.
func parseRow(rec []string, line int) (*ParsedMember, *RowError) {
	if len(rec) == 0 {
		return nil, nil
	}

	// Trim all fields
	for i := range rec {
		rec[i] = strings.TrimSpace(rec[i])
	}

	// Check for completely empty row
	allEmpty := true
	for _, f := range rec {
		if f != "" {
			allEmpty = false
			break
		}
	}
	if allEmpty {
		return nil, nil
	}

	// Need at least 3 fields: full_name, auth_type, third_field
	if len(rec) < 3 {
		return nil, &RowError{
			Line:   line,
			Reason: "row must have at least 3 fields (full_name, auth_type, and at least one more)",
			Raw:    rec,
		}
	}

	// Check if this is a header row (can appear anywhere in the file)
	// Header rows have header-like text in the second column instead of an auth method
	if isHeaderRow(rec) {
		return nil, nil // skip header rows
	}

	fullName := rec[0]
	authType := norm.AuthMethod(rec[1])

	// Validate required fields
	if fullName == "" {
		return nil, &RowError{
			Line:   line,
			Reason: "missing full name",
			Raw:    rec,
		}
	}

	if authType == "" || !inputval.IsValidAuthMethod(authType) {
		return nil, &RowError{
			Line:   line,
			Reason: fmt.Sprintf("invalid or missing auth method (allowed: %s)", strings.Join(inputval.AllowedAuthMethodsList(), ", ")),
			Raw:    rec,
		}
	}

	member := &ParsedMember{
		FullName:   fullName,
		AuthMethod: authType,
	}

	// Parse remaining fields based on auth method
	var parseErr *RowError
	if authutil.EmailIsLogin(authType) {
		// email, google, microsoft: full_name,auth_type,email
		parseErr = parseEmailAuthRow(member, rec, line)
	} else if authType == "password" {
		// password: full_name,password,login_id,temp_password[,email]
		parseErr = parsePasswordAuthRow(member, rec, line)
	} else if authType == "trust" {
		// trust: full_name,trust,login_id[,email]
		parseErr = parseTrustAuthRow(member, rec, line)
	} else if authutil.RequiresAuthReturnID(authType) {
		// clever, classlink, schoology
		parseErr = parseAuthReturnIDRow(member, rec, line)
	} else {
		parseErr = &RowError{
			Line:   line,
			Reason: fmt.Sprintf("unknown auth method: %s", authType),
			Raw:    rec,
		}
	}

	if parseErr != nil {
		return nil, parseErr
	}

	return member, nil
}

// parseEmailAuthRow parses a row for email/google/microsoft auth.
// Format: full_name,auth_type,email
func parseEmailAuthRow(member *ParsedMember, rec []string, line int) *RowError {
	// rec[2] is email (required)
	email := rec[2]
	if email == "" {
		return &RowError{
			Line:   line,
			Reason: fmt.Sprintf("email is required for %s authentication", member.AuthMethod),
			Raw:    rec,
		}
	}

	if !inputval.IsValidEmail(email) {
		return &RowError{
			Line:   line,
			Reason: "invalid email format",
			Raw:    rec,
		}
	}

	// For email auth methods, email is the login ID
	member.LoginID = email
	member.Email = &email

	return nil
}

// parsePasswordAuthRow parses a row for password auth.
// Format: full_name,password,login_id,temp_password[,email]
func parsePasswordAuthRow(member *ParsedMember, rec []string, line int) *RowError {
	if len(rec) < 4 {
		return &RowError{
			Line:   line,
			Reason: "password authentication requires: full_name,password,login_id,temp_password",
			Raw:    rec,
		}
	}

	loginID := rec[2]
	tempPassword := rec[3]

	if loginID == "" {
		return &RowError{
			Line:   line,
			Reason: "login ID is required for password authentication",
			Raw:    rec,
		}
	}

	if tempPassword == "" {
		return &RowError{
			Line:   line,
			Reason: "temporary password is required for password authentication",
			Raw:    rec,
		}
	}

	member.LoginID = loginID
	member.TempPassword = &tempPassword

	// Optional email in position 5
	if len(rec) >= 5 && rec[4] != "" {
		email := rec[4]
		if !inputval.IsValidEmail(email) {
			return &RowError{
				Line:   line,
				Reason: "invalid email format in optional email field",
				Raw:    rec,
			}
		}
		member.Email = &email
	}

	return nil
}

// parseTrustAuthRow parses a row for trust auth.
// Format: full_name,trust,login_id[,email]
func parseTrustAuthRow(member *ParsedMember, rec []string, line int) *RowError {
	// rec[2] is login_id (required)
	loginID := rec[2]
	if loginID == "" {
		return &RowError{
			Line:   line,
			Reason: "login ID is required for trust authentication",
			Raw:    rec,
		}
	}

	member.LoginID = loginID

	// Optional email in position 4
	if len(rec) >= 4 && rec[3] != "" {
		email := rec[3]
		if !inputval.IsValidEmail(email) {
			return &RowError{
				Line:   line,
				Reason: "invalid email format in optional email field",
				Raw:    rec,
			}
		}
		member.Email = &email
	}

	return nil
}

// parseAuthReturnIDRow parses a row for clever/classlink/schoology auth.
// Format (3 fields): full_name,auth_type,email (email used as login_id, auth_return_id, and email)
// Format (4+ fields): full_name,auth_type,login_id,auth_return_id[,email]
func parseAuthReturnIDRow(member *ParsedMember, rec []string, line int) *RowError {
	if len(rec) == 3 {
		// Short form: email is used for login_id, auth_return_id, and email
		email := rec[2]
		if email == "" {
			return &RowError{
				Line:   line,
				Reason: fmt.Sprintf("email is required for %s authentication (short form)", member.AuthMethod),
				Raw:    rec,
			}
		}

		if !inputval.IsValidEmail(email) {
			return &RowError{
				Line:   line,
				Reason: "invalid email format",
				Raw:    rec,
			}
		}

		// Email is used as login_id, auth_return_id, and email
		member.LoginID = email
		member.AuthReturnID = &email
		member.Email = &email
		return nil
	}

	// Long form: 4+ fields
	if len(rec) < 4 {
		return &RowError{
			Line:   line,
			Reason: fmt.Sprintf("%s authentication requires either 3 fields (full_name,auth_type,email) or 4+ fields (full_name,auth_type,login_id,auth_return_id)", member.AuthMethod),
			Raw:    rec,
		}
	}

	loginID := rec[2]
	authReturnID := rec[3]

	if loginID == "" {
		return &RowError{
			Line:   line,
			Reason: fmt.Sprintf("login ID is required for %s authentication", member.AuthMethod),
			Raw:    rec,
		}
	}

	if authReturnID == "" {
		return &RowError{
			Line:   line,
			Reason: fmt.Sprintf("auth return ID is required for %s authentication", member.AuthMethod),
			Raw:    rec,
		}
	}

	member.LoginID = loginID
	member.AuthReturnID = &authReturnID

	// Optional email in position 5
	if len(rec) >= 5 && rec[4] != "" {
		email := rec[4]
		if !inputval.IsValidEmail(email) {
			return &RowError{
				Line:   line,
				Reason: "invalid email format in optional email field",
				Raw:    rec,
			}
		}
		member.Email = &email
	}

	return nil
}
