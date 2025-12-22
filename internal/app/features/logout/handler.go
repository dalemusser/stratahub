// internal/app/features/logout/handler.go
package logout

import (
	"net/http"

	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"go.uber.org/zap"
)

type Handler struct {
	Log        *zap.Logger
	SessionMgr *auth.SessionManager
}

func NewHandler(sessionMgr *auth.SessionManager, logger *zap.Logger) *Handler {
	return &Handler{
		Log:        logger,
		SessionMgr: sessionMgr,
	}
}

// ServeLogout handles GET /logout.
func (h *Handler) ServeLogout(w http.ResponseWriter, r *http.Request) {
	// Get current session.
	session, err := h.SessionMgr.GetSession(r)
	if err != nil {
		// Session decode failed. Log and continue - we'll still try to clear the cookie.
		h.Log.Warn("session decode failed during logout", zap.Error(err))
	}

	// Ensure the deletion-cookie matches the original store settings.
	opts := h.SessionMgr.Store().Options
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
