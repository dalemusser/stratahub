// internal/app/features/errors/render.go
package errors

import (
	"html"
	"net/http"

	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/httpnav"
	"github.com/dalemusser/waffle/pantry/templates"
	"go.uber.org/zap"
)

// pageData is the basic view model for error pages.
type pageData struct {
	Title      string
	IsLoggedIn bool
	Role       string
	UserName   string
	Message    string
	BackURL    string

	// Layout fields (required by layout/menu templates)
	SiteName   string
	LogoURL    string
	FooterHTML string
}

// ErrorLogger provides error logging with context for HTTP handlers.
// Use NewErrorLogger to create an instance during handler initialization.
type ErrorLogger struct {
	logger *zap.Logger
}

// NewErrorLogger creates an ErrorLogger with the given zap logger.
func NewErrorLogger(logger *zap.Logger) *ErrorLogger {
	return &ErrorLogger{logger: logger}
}

// logError logs an error with context.
func (el *ErrorLogger) logError(r *http.Request, context string, err error) {
	if el.logger == nil || err == nil {
		return
	}
	el.logger.Error(context,
		zap.Error(err),
		zap.String("path", r.URL.Path),
		zap.String("method", r.Method))
}

// RenderUnauthorized shows a friendly "sign in required" page.
// If backURL is empty, it will default to /login.
func RenderUnauthorized(w http.ResponseWriter, r *http.Request, backURL string) {
	role, name, _, signed := authz.UserCtx(r)
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
		SiteName:   models.DefaultSiteName,
	}

	w.WriteHeader(http.StatusUnauthorized)
	templates.Render(w, r, "error_forbidden", data)
}

// RenderForbidden shows a friendly access error page with a message.
// If backURL is empty, it resolves a safe back URL with a default fallback.
func RenderForbidden(w http.ResponseWriter, r *http.Request, msg, backURL string) {
	role, name, _, signed := authz.UserCtx(r)
	if backURL == "" {
		backURL = httpnav.ResolveBackURL(r, "/")
	}

	data := pageData{
		Title:      "Access denied",
		IsLoggedIn: signed,
		Role:       role,
		UserName:   name,
		Message:    msg,
		BackURL:    backURL,
		SiteName:   models.DefaultSiteName,
	}

	w.WriteHeader(http.StatusForbidden)
	templates.Render(w, r, "error_forbidden", data)
}

// RenderServerError shows a friendly server error page.
// If backURL is empty, it resolves a safe back URL with a default fallback.
func RenderServerError(w http.ResponseWriter, r *http.Request, msg, backURL string) {
	role, name, _, signed := authz.UserCtx(r)
	if backURL == "" {
		backURL = httpnav.ResolveBackURL(r, "/")
	}
	if msg == "" {
		msg = "An unexpected error occurred. Please try again later."
	}

	data := pageData{
		Title:      "Server error",
		IsLoggedIn: signed,
		Role:       role,
		UserName:   name,
		Message:    msg,
		BackURL:    backURL,
		SiteName:   models.DefaultSiteName,
	}

	w.WriteHeader(http.StatusInternalServerError)
	templates.Render(w, r, "error_server", data)
}

// RenderBadRequest shows a friendly bad request error page.
// If backURL is empty, it resolves a safe back URL with a default fallback.
func RenderBadRequest(w http.ResponseWriter, r *http.Request, msg, backURL string) {
	role, name, _, signed := authz.UserCtx(r)
	if backURL == "" {
		backURL = httpnav.ResolveBackURL(r, "/")
	}
	if msg == "" {
		msg = "The request was invalid or malformed."
	}

	data := pageData{
		Title:      "Bad request",
		IsLoggedIn: signed,
		Role:       role,
		UserName:   name,
		Message:    msg,
		BackURL:    backURL,
		SiteName:   models.DefaultSiteName,
	}

	w.WriteHeader(http.StatusBadRequest)
	templates.Render(w, r, "error_forbidden", data)
}

// RenderNotFound shows a friendly not found error page.
// If backURL is empty, it resolves a safe back URL with a default fallback.
func RenderNotFound(w http.ResponseWriter, r *http.Request, msg, backURL string) {
	role, name, _, signed := authz.UserCtx(r)
	if backURL == "" {
		backURL = httpnav.ResolveBackURL(r, "/")
	}
	if msg == "" {
		msg = "The requested page or resource was not found."
	}

	data := pageData{
		Title:      "Not found",
		IsLoggedIn: signed,
		Role:       role,
		UserName:   name,
		Message:    msg,
		BackURL:    backURL,
		SiteName:   models.DefaultSiteName,
	}

	w.WriteHeader(http.StatusNotFound)
	templates.Render(w, r, "error_forbidden", data)
}

// isHTMXRequest checks if the request is an HTMX partial request.
func isHTMXRequest(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

// HTMXError returns an appropriate error response for HTMX requests.
// For HTMX requests, it returns a simple text error that can be swapped in.
// For regular requests, it falls back to the provided fallback function.
// This helps maintain consistency while supporting both full-page and partial requests.
func HTMXError(w http.ResponseWriter, r *http.Request, status int, msg string, fallback func()) {
	if isHTMXRequest(r) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(status)
		// Return a styled error message for HTMX to swap in.
		// Escape msg to prevent HTML/script injection.
		w.Write([]byte(`<div class="error-message" role="alert">` + html.EscapeString(msg) + `</div>`))
		return
	}
	fallback()
}

// HTMXServerError handles server errors for both HTMX and regular requests.
func HTMXServerError(w http.ResponseWriter, r *http.Request, msg, backURL string) {
	if msg == "" {
		msg = "A server error occurred."
	}
	HTMXError(w, r, http.StatusInternalServerError, msg, func() {
		RenderServerError(w, r, msg, backURL)
	})
}

// HTMXBadRequest handles bad request errors for both HTMX and regular requests.
func HTMXBadRequest(w http.ResponseWriter, r *http.Request, msg, backURL string) {
	if msg == "" {
		msg = "Invalid request."
	}
	HTMXError(w, r, http.StatusBadRequest, msg, func() {
		RenderBadRequest(w, r, msg, backURL)
	})
}

// HTMXForbidden handles forbidden errors for both HTMX and regular requests.
func HTMXForbidden(w http.ResponseWriter, r *http.Request, msg, backURL string) {
	if msg == "" {
		msg = "Access denied."
	}
	HTMXError(w, r, http.StatusForbidden, msg, func() {
		RenderForbidden(w, r, msg, backURL)
	})
}

// HTMXNotFound handles not found errors for both HTMX and regular requests.
func HTMXNotFound(w http.ResponseWriter, r *http.Request, msg, backURL string) {
	if msg == "" {
		msg = "Not found."
	}
	HTMXError(w, r, http.StatusNotFound, msg, func() {
		RenderNotFound(w, r, msg, backURL)
	})
}

/*─────────────────────────────────────────────────────────────────────────────*
| Logging variants - use these when you have an error to log                  |
*─────────────────────────────────────────────────────────────────────────────*/

// LogServerError logs the error and renders a server error page.
// Use this instead of RenderServerError when you have an actual error to log.
func (el *ErrorLogger) LogServerError(w http.ResponseWriter, r *http.Request, context string, err error, msg, backURL string) {
	el.logError(r, context, err)
	RenderServerError(w, r, msg, backURL)
}

// LogBadRequest logs the error and renders a bad request page.
// Use this instead of RenderBadRequest when you have an actual error to log.
func (el *ErrorLogger) LogBadRequest(w http.ResponseWriter, r *http.Request, context string, err error, msg, backURL string) {
	el.logError(r, context, err)
	RenderBadRequest(w, r, msg, backURL)
}

// LogForbidden logs the error and renders a forbidden page.
// Use this instead of RenderForbidden when you have an actual error to log.
func (el *ErrorLogger) LogForbidden(w http.ResponseWriter, r *http.Request, context string, err error, msg, backURL string) {
	el.logError(r, context, err)
	RenderForbidden(w, r, msg, backURL)
}

// LogNotFound logs the error and renders a not found page.
// Use this instead of RenderNotFound when you have an actual error to log.
func (el *ErrorLogger) LogNotFound(w http.ResponseWriter, r *http.Request, context string, err error, msg, backURL string) {
	el.logError(r, context, err)
	RenderNotFound(w, r, msg, backURL)
}

// HTMXLogServerError logs the error and handles server errors for HTMX/regular requests.
func (el *ErrorLogger) HTMXLogServerError(w http.ResponseWriter, r *http.Request, context string, err error, msg, backURL string) {
	el.logError(r, context, err)
	HTMXServerError(w, r, msg, backURL)
}

// HTMXLogBadRequest logs the error and handles bad request errors for HTMX/regular requests.
func (el *ErrorLogger) HTMXLogBadRequest(w http.ResponseWriter, r *http.Request, context string, err error, msg, backURL string) {
	el.logError(r, context, err)
	HTMXBadRequest(w, r, msg, backURL)
}

// HTMXLogForbidden logs the error and handles forbidden errors for HTMX/regular requests.
func (el *ErrorLogger) HTMXLogForbidden(w http.ResponseWriter, r *http.Request, context string, err error, msg, backURL string) {
	el.logError(r, context, err)
	HTMXForbidden(w, r, msg, backURL)
}

// HTMXLogNotFound logs the error and handles not found errors for HTMX/regular requests.
func (el *ErrorLogger) HTMXLogNotFound(w http.ResponseWriter, r *http.Request, context string, err error, msg, backURL string) {
	el.logError(r, context, err)
	HTMXNotFound(w, r, msg, backURL)
}
