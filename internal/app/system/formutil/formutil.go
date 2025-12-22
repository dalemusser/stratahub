// Package formutil provides helpers for form re-rendering with validation errors.
//
// When a form submission fails validation, the form should be re-rendered with:
// - The user's previously entered values (echoed back)
// - An error message explaining what went wrong
// - All the context data needed for the form (dropdowns, etc.)
//
// This package provides a Base struct that can be embedded in form data structs
// to handle the common fields, and helper functions to populate them.
//
// Example usage:
//
//	type newMemberData struct {
//		formutil.Base
//		FullName string
//		Email    string
//		Orgs     []orgOption
//	}
//
//	// In your handler:
//	data := newMemberData{FullName: full, Email: email}
//	formutil.SetBase(&data.Base, r, "Add Member", "/members")
//	data.Error = template.HTML("Email is required.")
//	templates.Render(w, r, "member_new", data)
package formutil

import (
	"html/template"
	"net/http"

	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/waffle/pantry/httpnav"
)

// Base contains common fields for form pages that can be embedded in form data structs.
type Base struct {
	Title       string
	IsLoggedIn  bool
	Role        string
	UserName    string
	BackURL     string
	CurrentPath string
	Error       template.HTML
}

// SetBase populates the common Base fields from the request context.
// It extracts user info from authz.UserCtx and sets navigation fields.
//
// Parameters:
//   - b: pointer to the Base struct to populate
//   - r: the HTTP request
//   - title: the page title
//   - backDefault: default URL for the back button if none in request
func SetBase(b *Base, r *http.Request, title, backDefault string) {
	role, uname, _, _ := authz.UserCtx(r)
	b.Title = title
	b.IsLoggedIn = true
	b.Role = role
	b.UserName = uname
	b.BackURL = httpnav.ResolveBackURL(r, backDefault)
	b.CurrentPath = httpnav.CurrentPath(r)
}

// SetError sets the error message on a Base struct.
// This is a convenience method for setting Error as template.HTML.
func (b *Base) SetError(msg string) {
	b.Error = template.HTML(msg)
}
