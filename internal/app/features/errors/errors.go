// internal/app/features/errors/errors.go
package errors

import (
	"net/http"
)

// Handler is the errors feature handler for standalone error page routes.
// No DB needed; it delegates to the Render* functions in render.go.
type Handler struct{}

// NewHandler constructs an errors Handler.
func NewHandler() *Handler {
	return &Handler{}
}

// Forbidden renders a friendly "access denied" page.
// GET /forbidden
func (h *Handler) Forbidden(w http.ResponseWriter, r *http.Request) {
	RenderForbidden(w, r, "You don't have permission to view this page.", "/")
}

// Unauthorized renders a friendly "sign in required" page.
// GET /unauthorized
func (h *Handler) Unauthorized(w http.ResponseWriter, r *http.Request) {
	RenderUnauthorized(w, r, "/login")
}
