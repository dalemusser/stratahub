// internal/domain/models/authmethods.go
package models

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import "strings"

// AuthMethod represents an authentication method option for the UI.
type AuthMethod struct {
	Value string // The value stored in the database
	Label string // The display label in the UI
}

// AllAuthMethods contains all supported auth methods with their display labels.
// This is used for validation and as a reference for all possible values.
var AllAuthMethods = []AuthMethod{
	{Value: "trust", Label: "Trust"},
	{Value: "password", Label: "Password"},
	{Value: "email", Label: "Email Verification"},
	{Value: "google", Label: "Google"},
	{Value: "microsoft", Label: "Microsoft"},
	{Value: "clever", Label: "Clever"},
	{Value: "classlink", Label: "Classlink"},
	{Value: "schoology", Label: "Schoology"},
}

// EnabledAuthMethods contains the auth methods currently available in the UI.
// Modify this list to control which options appear in Auth Method dropdowns,
// CSV uploads, and related features. All handling logic for disabled methods
// remains in place for when they're enabled.
//
// Auth method categories (for CSV format display):
//   - Email-as-login: email, google, microsoft (format: name, method, email)
//   - Password: password (format: name, password, login_id, temp_password [, email])
//   - Trust: trust (format: name, trust, login_id [, email])
//   - SSO with auth_return_id: clever, classlink, schoology (two formats supported)
var EnabledAuthMethods = []AuthMethod{
	{Value: "trust", Label: "Trust"},
	{Value: "password", Label: "Password"},
	{Value: "email", Label: "Email Verification"},
	{Value: "google", Label: "Google"},
	// {Value: "microsoft", Label: "Microsoft"},
	// {Value: "clever", Label: "Clever"},
	// {Value: "classlink", Label: "Classlink"},
	// {Value: "schoology", Label: "Schoology"},
}

// IsValidAuthMethod checks if a value is a valid auth method.
func IsValidAuthMethod(value string) bool {
	for _, m := range AllAuthMethods {
		if m.Value == value {
			return true
		}
	}
	return false
}

// IsEnabledAuthMethod checks if a value is an enabled auth method.
func IsEnabledAuthMethod(value string) bool {
	for _, m := range EnabledAuthMethods {
		if m.Value == value {
			return true
		}
	}
	return false
}

// EnabledAuthMethodValues returns just the values of enabled auth methods.
func EnabledAuthMethodValues() []string {
	values := make([]string, len(EnabledAuthMethods))
	for i, m := range EnabledAuthMethods {
		values[i] = m.Value
	}
	return values
}

// EnabledAuthMethodsForCSV provides information about enabled auth methods
// for CSV upload templates, grouped by format category.
type EnabledAuthMethodsForCSV struct {
	// EmailLoginMethods are enabled methods where email is the login ID (email, google, microsoft)
	EmailLoginMethods []string
	// HasPassword indicates if password auth is enabled
	HasPassword bool
	// HasTrust indicates if trust auth is enabled
	HasTrust bool
	// SSOAuthMethods are enabled SSO methods requiring auth_return_id (clever, classlink, schoology)
	SSOAuthMethods []string
}

// GetEnabledAuthMethodsForCSV returns enabled auth methods grouped by CSV format category.
func GetEnabledAuthMethodsForCSV() EnabledAuthMethodsForCSV {
	return GetAuthMethodsForCSV(EnabledAuthMethods)
}

// GetAuthMethodsForCSV returns auth methods grouped by CSV format category for a given list.
func GetAuthMethodsForCSV(methods []AuthMethod) EnabledAuthMethodsForCSV {
	result := EnabledAuthMethodsForCSV{}

	emailLoginSet := map[string]bool{"email": true, "google": true, "microsoft": true}
	ssoSet := map[string]bool{"clever": true, "classlink": true, "schoology": true}

	for _, m := range methods {
		if emailLoginSet[m.Value] {
			result.EmailLoginMethods = append(result.EmailLoginMethods, m.Value)
		} else if m.Value == "password" {
			result.HasPassword = true
		} else if m.Value == "trust" {
			result.HasTrust = true
		} else if ssoSet[m.Value] {
			result.SSOAuthMethods = append(result.SSOAuthMethods, m.Value)
		}
	}

	return result
}

// EmailLoginMethodsLabel returns a comma-separated label for email login methods.
// Example: "email, google, or microsoft"
func (e EnabledAuthMethodsForCSV) EmailLoginMethodsLabel() string {
	return joinMethodsWithOr(e.EmailLoginMethods)
}

// SSOAuthMethodsLabel returns a comma-separated label for SSO auth methods.
// Example: "clever, classlink, or schoology"
func (e EnabledAuthMethodsForCSV) SSOAuthMethodsLabel() string {
	return joinMethodsWithOr(e.SSOAuthMethods)
}

// HasEmailLoginMethods returns true if any email-login methods are enabled.
func (e EnabledAuthMethodsForCSV) HasEmailLoginMethods() bool {
	return len(e.EmailLoginMethods) > 0
}

// HasSSOAuthMethods returns true if any SSO auth methods are enabled.
func (e EnabledAuthMethodsForCSV) HasSSOAuthMethods() bool {
	return len(e.SSOAuthMethods) > 0
}

// joinMethodsWithOr joins method names with commas and "or" before the last one.
func joinMethodsWithOr(methods []string) string {
	if len(methods) == 0 {
		return ""
	}
	if len(methods) == 1 {
		return methods[0]
	}
	if len(methods) == 2 {
		return methods[0] + " or " + methods[1]
	}
	return strings.Join(methods[:len(methods)-1], ", ") + ", or " + methods[len(methods)-1]
}
