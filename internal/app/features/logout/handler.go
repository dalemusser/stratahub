// internal/app/features/logout/handler.go
package logout

import (
	"context"
	"net/http"
	"time"

	"github.com/dalemusser/stratahub/internal/app/store/sessions"
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/app/system/auditlog"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

type Handler struct {
	Log        *zap.Logger
	SessionMgr *auth.SessionManager
	AuditLog   *auditlog.Logger
	Sessions   *sessions.Store // Activity session tracking
}

func NewHandler(sessionMgr *auth.SessionManager, audit *auditlog.Logger, sessStore *sessions.Store, logger *zap.Logger) *Handler {
	return &Handler{
		Log:        logger,
		SessionMgr: sessionMgr,
		AuditLog:   audit,
		Sessions:   sessStore,
	}
}

// ServeLogout handles GET /logout.
func (h *Handler) ServeLogout(w http.ResponseWriter, r *http.Request) {
	// Get current user for audit logging (before destroying session)
	if h.AuditLog != nil {
		if user, ok := auth.CurrentUser(r); ok && user != nil {
			h.AuditLog.Logout(r.Context(), r, user.ID, user.OrganizationID)
		}
	}

	// Get current session.
	session, err := h.SessionMgr.GetSession(r)
	if err != nil {
		// Session decode failed. Log and continue - we'll still try to clear the cookie.
		h.Log.Warn("session decode failed during logout", zap.Error(err))
	}

	// Close activity session if one exists
	if h.Sessions != nil {
		if activitySessionID, ok := session.Values["activity_session_id"].(string); ok && activitySessionID != "" {
			if oid, err := primitive.ObjectIDFromHex(activitySessionID); err == nil {
				ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
				if err := h.Sessions.Close(ctx, oid, "logout"); err != nil {
					h.Log.Warn("failed to close activity session", zap.Error(err), zap.String("session_id", activitySessionID))
				}
				cancel()
			}
		}
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
