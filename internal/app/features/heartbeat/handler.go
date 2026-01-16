// internal/app/features/heartbeat/handler.go
package heartbeat

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/dalemusser/stratahub/internal/app/store/activity"
	"github.com/dalemusser/stratahub/internal/app/store/sessions"
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/app/system/ratelimit"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

// Handler handles heartbeat requests for activity tracking.
type Handler struct {
	Sessions   *sessions.Store
	Activity   *activity.Store
	SessionMgr *auth.SessionManager
	Log        *zap.Logger
}

// NewHandler creates a new heartbeat handler.
func NewHandler(sessStore *sessions.Store, activityStore *activity.Store, sessionMgr *auth.SessionManager, logger *zap.Logger) *Handler {
	return &Handler{
		Sessions:   sessStore,
		Activity:   activityStore,
		SessionMgr: sessionMgr,
		Log:        logger,
	}
}

// heartbeatRequest is the JSON body for the heartbeat endpoint.
type heartbeatRequest struct {
	Page string `json:"page"`
}

// ServeHeartbeat handles POST /api/heartbeat.
// Updates the LastActivity timestamp for the user's current session.
// If the session was closed due to inactivity, creates a new one.
// Returns 401 if the session has been terminated by an admin.
func (h *Handler) ServeHeartbeat(w http.ResponseWriter, r *http.Request) {
	// Get session token from the cookie
	sess, err := h.SessionMgr.GetSession(r)
	if err != nil {
		w.WriteHeader(http.StatusOK) // Silent fail - invalid session
		return
	}

	sessionToken, ok := sess.Values["session_token"].(string)
	if !ok || sessionToken == "" {
		w.WriteHeader(http.StatusOK) // Silent fail - no session token
		return
	}

	// Parse request body to get current page
	var req heartbeatRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req) // Ignore error, page is optional
	}

	// Check if the session is still valid (not terminated by admin)
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	dbSession, err := h.Sessions.GetByToken(ctx, sessionToken)
	if err != nil || dbSession == nil {
		// Session was terminated or doesn't exist - tell client to logout
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// Update last activity time and current page (using token-based lookup)

	result, err := h.Sessions.UpdateCurrentPage(ctx, sessionToken, req.Page)
	if err != nil {
		h.Log.Warn("failed to update session activity",
			zap.Error(err),
			zap.String("page", req.Page))
		w.WriteHeader(http.StatusOK)
		return
	}

	// Record page view event if page changed (but not on first heartbeat when PreviousPage is empty)
	if result.Updated && req.Page != "" && result.PreviousPage != "" && req.Page != result.PreviousPage && h.Activity != nil {
		userIDStr, _ := sess.Values["user_id"].(string)
		if userOID, err := primitive.ObjectIDFromHex(userIDStr); err == nil {
			// Get optional org ID
			var orgOID *primitive.ObjectID
			if orgIDStr, ok := sess.Values["org_id"].(string); ok && orgIDStr != "" {
				if oid, err := primitive.ObjectIDFromHex(orgIDStr); err == nil {
					orgOID = &oid
				}
			}
			// Look up session to get its ID for activity recording
			sessionDoc, _ := h.Sessions.GetByToken(ctx, sessionToken)
			var sessionOID primitive.ObjectID
			if sessionDoc != nil {
				sessionOID = sessionDoc.ID
			}
			if err := h.Activity.RecordPageView(ctx, userOID, sessionOID, orgOID, req.Page); err != nil {
				h.Log.Warn("failed to record page view",
					zap.Error(err),
					zap.String("page", req.Page))
			}
		}
	}

	// If session wasn't updated (already closed), create a new one
	if !result.Updated {
		userIDStr, ok := sess.Values["user_id"].(string)
		if !ok || userIDStr == "" {
			w.WriteHeader(http.StatusOK)
			return
		}

		userOID, err := primitive.ObjectIDFromHex(userIDStr)
		if err != nil {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Get optional org ID
		var orgOID *primitive.ObjectID
		if orgIDStr, ok := sess.Values["org_id"].(string); ok && orgIDStr != "" {
			if oid, err := primitive.ObjectIDFromHex(orgIDStr); err == nil {
				orgOID = &oid
			}
		}

		// Generate new session token
		newToken, tokenErr := auth.GenerateSessionToken()
		if tokenErr != nil {
			h.Log.Warn("failed to generate session token",
				zap.Error(tokenErr),
				zap.String("user_id", userIDStr))
			w.WriteHeader(http.StatusOK)
			return
		}

		// Create new activity session
		now := time.Now()
		newSess := sessions.Session{
			Token:          newToken,
			UserID:         userOID,
			OrganizationID: orgOID,
			IP:             ratelimit.ClientIP(r),
			UserAgent:      r.UserAgent(),
			LoginAt:        now,
			LastActivity:   now,
			CreatedBy:      sessions.CreatedByHeartbeat,
			ExpiresAt:      now.Add(24 * 30 * time.Hour), // 30 days
		}
		if err := h.Sessions.Create(ctx, newSess); err != nil {
			h.Log.Warn("failed to create new activity session after timeout",
				zap.Error(err),
				zap.String("user_id", userIDStr))
			w.WriteHeader(http.StatusOK)
			return
		}

		// Update cookie with new session token
		sess.Values["session_token"] = newToken
		if err := sess.Save(r, w); err != nil {
			h.Log.Warn("failed to save session with new session_token",
				zap.Error(err))
		}

		h.Log.Info("created new activity session after inactivity timeout",
			zap.String("user_id", userIDStr))
	}

	w.WriteHeader(http.StatusOK)
}
