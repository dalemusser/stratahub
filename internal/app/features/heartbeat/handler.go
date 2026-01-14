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
// Updates the LastActiveAt timestamp for the user's current session.
// If the session was closed due to inactivity, creates a new one.
func (h *Handler) ServeHeartbeat(w http.ResponseWriter, r *http.Request) {
	// Get activity session ID from the session cookie
	sess, err := h.SessionMgr.GetSession(r)
	if err != nil {
		w.WriteHeader(http.StatusOK) // Silent fail - invalid session
		return
	}

	activitySessionID, ok := sess.Values["activity_session_id"].(string)
	if !ok || activitySessionID == "" {
		w.WriteHeader(http.StatusOK) // Silent fail - no activity session
		return
	}

	oid, err := primitive.ObjectIDFromHex(activitySessionID)
	if err != nil {
		w.WriteHeader(http.StatusOK) // Silent fail - invalid ID
		return
	}

	// Parse request body to get current page
	var req heartbeatRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req) // Ignore error, page is optional
	}

	// Update last active time and current page
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	result, err := h.Sessions.UpdateLastActive(ctx, oid, req.Page)
	if err != nil {
		h.Log.Warn("failed to update session last_active_at",
			zap.Error(err),
			zap.String("session_id", activitySessionID))
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
			if err := h.Activity.RecordPageView(ctx, userOID, oid, orgOID, req.Page); err != nil {
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

		// Create new activity session
		ip := ratelimit.ClientIP(r)
		newSess, err := h.Sessions.Create(ctx, userOID, orgOID, ip, r.UserAgent(), sessions.CreatedByHeartbeat)
		if err != nil {
			h.Log.Warn("failed to create new activity session after timeout",
				zap.Error(err),
				zap.String("user_id", userIDStr))
			w.WriteHeader(http.StatusOK)
			return
		}

		// Update cookie with new session ID
		sess.Values["activity_session_id"] = newSess.ID.Hex()
		if err := sess.Save(r, w); err != nil {
			h.Log.Warn("failed to save session with new activity_session_id",
				zap.Error(err))
		}

		h.Log.Info("created new activity session after inactivity timeout",
			zap.String("user_id", userIDStr),
			zap.String("new_session_id", newSess.ID.Hex()))
	}

	w.WriteHeader(http.StatusOK)
}
