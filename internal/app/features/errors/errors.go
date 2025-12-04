// internal/app/features/errors/errors.go
package errors

import (
	"net/http"

	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/waffle/templates"
)

// pageData is the basic view model for error pages.
type pageData struct {
	Title      string
	IsLoggedIn bool
	Role       string
	UserName   string
	Message    string
	BackURL    string
}

// Handler is the errors feature handler.
// No DB needed; it just renders templates.
type Handler struct{}

// NewHandler constructs an errors Handler.
func NewHandler() *Handler {
	return &Handler{}
}

// Forbidden renders a friendly "access denied" page.
// GET /forbidden
func (h *Handler) Forbidden(w http.ResponseWriter, r *http.Request) {
	role, name, _, signedIn := authz.UserCtx(r)

	data := pageData{
		Title:      "Access denied",
		IsLoggedIn: signedIn,
		Role:       role,
		UserName:   name,
		Message:    "You don't have permission to view this page.",
		BackURL:    "/", // can be overridden by helpers using nav.
	}

	templates.Render(w, r, "error_forbidden", data)
}

// Unauthorized renders a friendly "sign in required" page.
// GET /unauthorized
func (h *Handler) Unauthorized(w http.ResponseWriter, r *http.Request) {
	role, name, _, signedIn := authz.UserCtx(r)

	data := pageData{
		Title:      "Sign in required",
		IsLoggedIn: signedIn,
		Role:       role,
		UserName:   name,
		Message:    "Please sign in to continue.",
		BackURL:    "/login",
	}

	// For now we reuse error_forbidden template; you can add
	// error_unauthorized.gohtml later if you want a distinct view.
	templates.Render(w, r, "error_forbidden", data)
}
