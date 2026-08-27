// internal/app/features/memberstatusapi/handler.go
//
// Member Status API: an inbound, key-authenticated JSON endpoint an external
// provider (e.g. the survey system) calls to report that a member started or
// completed an entity (a survey). Server-to-server only: no session, no
// cookie, no CSRF — the workspace's shared key travels in the request body.
// See docs/member-status-api/plan.md.
package memberstatusapi

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dalemusser/stratahub/internal/app/store/memberstatus"
	settingsstore "github.com/dalemusser/stratahub/internal/app/store/settings"
	userstore "github.com/dalemusser/stratahub/internal/app/store/users"
	"github.com/dalemusser/stratahub/internal/app/system/memberstatuscfg"
	"github.com/dalemusser/stratahub/internal/app/system/ratelimit"
	"github.com/dalemusser/stratahub/internal/app/system/status"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// PathPrefix is where the API is mounted. Referenced by the CSRF exemption
// and the maintenance-mode exemption, so keep the three in step.
const PathPrefix = "/api/member-status"

const (
	maxBodyBytes = 8 << 10 // 8 KB is ample for one event
	maxEntityLen = 100

	// Failed-authentication throttle, per client IP. Successful calls reset it.
	authFailureLimit  = 20
	authFailureWindow = 5 * time.Minute
)

// Handler owns the Member Status API endpoints.
type Handler struct {
	DB       *mongo.Database
	Settings *settingsstore.Store
	Users    *userstore.Store
	Status   *memberstatus.Store
	Config   *memberstatuscfg.Config
	Limiter  *ratelimit.Limiter // counts failed authentications per IP
	Log      *zap.Logger
}

// NewHandler constructs the handler. It loads the entity configuration up
// front so a malformed configuration fails at startup, not at first request.
func NewHandler(db *mongo.Database, logger *zap.Logger) (*Handler, error) {
	cfg, err := memberstatuscfg.Load()
	if err != nil {
		return nil, err
	}
	return &Handler{
		DB:       db,
		Settings: settingsstore.New(db),
		Users:    userstore.New(db),
		Status:   memberstatus.New(db),
		Config:   cfg,
		Limiter:  ratelimit.New(authFailureLimit, authFailureWindow),
		Log:      logger,
	}, nil
}

// HandlePing — POST /api/member-status/ping
//
// Verifies connectivity and the shared key, and reports the entity names in
// use so the provider can confirm the exact strings before sending events.
func (h *Handler) HandlePing(w http.ResponseWriter, r *http.Request) {
	ws, _, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, pingResponse{
		OK:        true,
		Workspace: ws.Subdomain,
		Entities:  h.Config.APINames(),
	})
}

// HandleStatus — POST /api/member-status
//
// Records one status event for a member. Idempotent and monotonic: see
// memberstatus.Store.Record for the exact semantics.
func (h *Handler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	ws, req, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	ctx := r.Context()

	// --- Validate the event fields ---

	userID, err := primitive.ObjectIDFromHex(strings.TrimSpace(req.UserID))
	if err != nil {
		writeError(w, http.StatusBadRequest, errInvalidUserID, "user_id must be a 24-character hex id.")
		return
	}

	entity := strings.TrimSpace(req.Entity)
	if entity == "" || len(entity) > maxEntityLen {
		writeError(w, http.StatusBadRequest, errInvalidEntity,
			fmt.Sprintf("entity is required and must be at most %d characters.", maxEntityLen))
		return
	}

	// Only provider-reportable states are accepted here; "opened" is written
	// by StrataHub itself when a member launches the linked resource.
	state := strings.ToLower(strings.TrimSpace(req.State))
	switch state {
	case models.MemberStatusStarted, models.MemberStatusCompleted:
	default:
		writeError(w, http.StatusBadRequest, errInvalidState, `state must be "started" or "completed".`)
		return
	}

	var occurredAt *time.Time
	if s := strings.TrimSpace(req.OccurredAt); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			writeError(w, http.StatusBadRequest, errInvalidOccurredAt, "occurred_at must be an RFC 3339 timestamp (e.g. 2026-08-25T14:03:11Z).")
			return
		}
		t = t.UTC()
		occurredAt = &t
	}

	// --- The member must exist, be active, and belong to this workspace ---

	// Unknown users are logged at Warn (with the reason, which the uniform
	// 404 deliberately omits) so a stream of them from the provider is
	// visible on the StrataHub side, not only on theirs.
	unknownUser := func(reason string) {
		h.Log.Warn("member status: unknown user",
			zap.String("reason", reason),
			zap.String("workspace_id", ws.ID.Hex()),
			zap.String("user_id", userID.Hex()),
			zap.String("entity", entity),
			zap.String("state", state),
			zap.String("remote_ip", ratelimit.ClientIP(r)))
		writeError(w, http.StatusNotFound, errUnknownUser, "No active member with that user_id in this workspace.")
	}

	user, err := h.Users.GetMemberByID(ctx, userID)
	switch {
	case errors.Is(err, mongo.ErrNoDocuments):
		unknownUser("no member with this id")
		return
	case err != nil:
		h.Log.Error("member status: user lookup failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, errServer, "Lookup failed; please retry.")
		return
	}
	switch {
	case user.WorkspaceID == nil || *user.WorkspaceID != ws.ID:
		// Same response as not-found: don't reveal that the id exists elsewhere.
		unknownUser("member belongs to a different workspace")
		return
	case user.Status != "" && user.Status != status.Active:
		unknownUser("member is not active")
		return
	}

	// --- Resolve the entity and record ---

	entityKey, item, known := h.Config.KeyFor(entity)
	display := entity
	if known {
		display = item.Title
	}

	doc, err := h.Status.Record(ctx, memberstatus.RecordInput{
		WorkspaceID: ws.ID,
		UserID:      userID,
		EntityKey:   entityKey,
		Entity:      display,
		State:       state,
		Source:      models.MemberStatusSourceAPI,
		OccurredAt:  occurredAt,
		RemoteIP:    ratelimit.ClientIP(r),
	})
	if err != nil {
		h.Log.Error("member status: record failed",
			zap.String("workspace_id", ws.ID.Hex()),
			zap.String("user_id", userID.Hex()),
			zap.String("entity_key", entityKey),
			zap.Error(err))
		writeError(w, http.StatusInternalServerError, errServer, "Could not record the event; please retry.")
		return
	}

	fields := []zap.Field{
		zap.String("workspace_id", ws.ID.Hex()),
		zap.String("user_id", userID.Hex()),
		zap.String("entity_key", entityKey),
		zap.String("state", state),
		zap.String("resulting_state", doc.State),
		zap.String("remote_ip", ratelimit.ClientIP(r)),
	}
	if known {
		h.Log.Info("member status recorded", fields...)
	} else {
		h.Log.Warn("member status recorded for unrecognized entity name",
			append(fields, zap.String("entity", entity))...)
	}

	writeJSON(w, http.StatusOK, statusResponse{
		OK:          true,
		UserID:      userID.Hex(),
		Entity:      doc.Entity,
		EntityKey:   entityKey,
		KnownEntity: known,
		State:       doc.State,
		OpenedAt:    doc.OpenedAt,
		StartedAt:   doc.StartedAt,
		CompletedAt: doc.CompletedAt,
	})
}

// authenticate resolves the workspace, decodes the body, and validates the
// shared key. On any failure it has already written the response and returns
// ok=false. Failed key checks are throttled per client IP.
func (h *Handler) authenticate(w http.ResponseWriter, r *http.Request) (ws *workspace.Info, req statusRequest, ok bool) {
	ws = workspace.FromRequest(r)
	if ws == nil || ws.IsApex || ws.ID.IsZero() {
		writeError(w, http.StatusBadRequest, errWorkspaceRequired,
			"Post to the workspace's own host (for example https://<workspace>.<domain>"+PathPrefix+").")
		return nil, req, false
	}

	ip := ratelimit.ClientIP(r)
	if h.Limiter.Remaining(ip) <= 0 {
		writeError(w, http.StatusTooManyRequests, errRateLimited, "Too many failed authentications; try again later.")
		return nil, req, false
	}

	if err := json.NewDecoder(io.LimitReader(r.Body, maxBodyBytes)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, errBadJSON, "Request body must be a JSON object.")
		return nil, req, false
	}

	key := strings.TrimSpace(req.Key)
	if key == "" {
		// Undocumented alternative: Authorization: Bearer <key>.
		if auth := r.Header.Get("Authorization"); auth != "" {
			if parts := strings.SplitN(auth, " ", 2); len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
				key = strings.TrimSpace(parts[1])
			}
		}
	}
	if key == "" {
		h.Limiter.Allow(ip)
		writeError(w, http.StatusUnauthorized, errUnauthorized, `Missing shared key ("key" in the JSON body).`)
		return nil, req, false
	}

	settings, err := h.Settings.Get(r.Context(), ws.ID)
	if err != nil {
		h.Log.Error("member status: settings lookup failed", zap.String("workspace_id", ws.ID.Hex()), zap.Error(err))
		writeError(w, http.StatusInternalServerError, errServer, "Lookup failed; please retry.")
		return nil, req, false
	}
	if settings.MemberStatusAPIKey == "" {
		h.Log.Warn("member status: request to workspace with no API key configured",
			zap.String("workspace_id", ws.ID.Hex()), zap.String("remote_ip", ip))
		writeError(w, http.StatusUnauthorized, errNotConfigured, "The Member Status API is not enabled for this workspace.")
		return nil, req, false
	}

	if subtle.ConstantTimeCompare([]byte(key), []byte(settings.MemberStatusAPIKey)) != 1 {
		h.Limiter.Allow(ip)
		h.Log.Warn("member status: invalid shared key",
			zap.String("workspace_id", ws.ID.Hex()),
			zap.String("path", r.URL.Path),
			zap.String("remote_ip", ip))
		writeError(w, http.StatusUnauthorized, errUnauthorized, "Invalid shared key.")
		return nil, req, false
	}

	h.Limiter.Reset(ip)
	return ws, req, true
}
