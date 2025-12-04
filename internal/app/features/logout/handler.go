// internal/app/features/logout/handler.go
package logout

import (
	"net/http"

	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"go.uber.org/zap"
)

type Handler struct {
	Log *zap.Logger
}

func NewHandler(logger *zap.Logger) *Handler {
	return &Handler{
		Log: logger,
	}
}

// ServeLogout handles GET /logout.
func (h *Handler) ServeLogout(w http.ResponseWriter, r *http.Request) {
	// If the session store isn't initialized, just redirect home.
	if auth.Store == nil {
		h.Log.Warn("logout called but session store is not initialized")
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Get current session.
	session, _ := auth.Store.Get(r, auth.SessionName)

	// Ensure the deletion-cookie matches the original store settings.
	opts := auth.Store.Options
	if opts != nil {
		session.Options.Domain = opts.Domain
		session.Options.Path = opts.Path
		session.Options.Secure = opts.Secure
		session.Options.HttpOnly = opts.HttpOnly
		session.Options.SameSite = opts.SameSite
	}
	session.Options.MaxAge = -1 // delete immediately

	if err := session.Save(r, w); err != nil {
		h.Log.Error("logout: save session", zap.Error(err))
	}

	// HTMX handling: use HX-Redirect to force a client-side navigation to "/".
	if r.Header.Get("HX-Request") != "" {
		w.Header().Set("HX-Redirect", "/")
		// We don't really care about the status code here; HTMX uses HX-Redirect.
		w.WriteHeader(http.StatusOK)
		return
	}

	// Non-HTMX: standard redirect home.
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
