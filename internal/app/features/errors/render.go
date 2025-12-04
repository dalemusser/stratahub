// internal/app/features/errors/render.go
package errors

import (
	"net/http"

	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/waffle/templates"
	nav "github.com/dalemusser/waffle/toolkit/ui/nav"
)

// RenderUnauthorized shows a friendly “sign in required” page.
// If backURL is empty, it will default to /login.
func RenderUnauthorized(w http.ResponseWriter, r *http.Request, backURL string) {
	u, signed := auth.CurrentUser(r)
	role, name := "", ""
	if signed && u != nil {
		role, name = u.Role, u.Name
	}
	if backURL == "" {
		backURL = "/login"
	}

	data := pageData{
		Title:      "Sign in required",
		IsLoggedIn: signed,
		Role:       role,
		UserName:   name,
		Message:    "Please sign in to continue.",
		BackURL:    backURL,
	}

	// You can switch to a distinct error_unauthorized template later.
	templates.Render(w, r, "error_forbidden", data)
}

// RenderForbidden shows a friendly access error page with a message.
// If backURL is empty, it resolves a safe back URL with a default fallback.
func RenderForbidden(w http.ResponseWriter, r *http.Request, msg, backURL string) {
	u, signed := auth.CurrentUser(r)
	role, name := "", ""
	if signed && u != nil {
		role, name = u.Role, u.Name
	}
	if backURL == "" {
		backURL = nav.ResolveBackURL(r, "/")
	}

	data := pageData{
		Title:      "Access denied",
		IsLoggedIn: signed,
		Role:       role,
		UserName:   name,
		Message:    msg,
		BackURL:    backURL,
	}

	templates.Render(w, r, "error_forbidden", data)
}
