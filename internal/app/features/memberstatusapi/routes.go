// internal/app/features/memberstatusapi/routes.go
package memberstatusapi

import (
	"net/http"

	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/gorilla/csrf"
)

// MountRoutes registers the Member Status API on the root router. It is
// mounted outside the RequireWorkspace group: the handler resolves the
// workspace itself and answers with JSON (not a redirect) when it is missing.
//
//	POST /api/member-status       — record a member's status on an entity
//	POST /api/member-status/ping  — verify connectivity, key, and entity names
func MountRoutes(r chi.Router, h *Handler) {
	r.Route(PathPrefix, func(sr chi.Router) {
		// Bound every request so a stalled DB can't pin goroutines; nothing
		// here streams or long-polls.
		sr.Use(middleware.Timeout(timeouts.Medium()))
		sr.Post("/", h.HandleStatus)
		sr.Post("/ping", h.HandlePing)
	})
}

// CSRFExempt marks Member Status API requests as exempt from CSRF validation.
// The API is called server-to-server with a shared key in the body; there is
// no browser session or CSRF token to check.
//
// It must be installed on the root router BEFORE gorilla/csrf's Protect
// middleware — gorilla/csrf reads the skip flag at the start of its own
// ServeHTTP, and sub-router middleware runs too late.
func CSRFExempt(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isAPIPath(r.URL.Path) {
			r = csrf.UnsafeSkipCheck(r)
		}
		next.ServeHTTP(w, r)
	})
}
