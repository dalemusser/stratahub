// internal/app/features/missionhydrosci/api_manifest.go
package missionhydrosci

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	groupappstore "github.com/dalemusser/stratahub/internal/app/store/groupapps"
	"github.com/dalemusser/stratahub/internal/app/store/mhsbuilds"
	"github.com/dalemusser/stratahub/internal/app/store/mhscollections"
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/app/system/staffauth"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

// resolvedCollection holds the result of collection resolution.
type resolvedCollection struct {
	Collection models.MHSCollection
	Found      bool
	IsOverride bool // true if resolved via per-user override
}

// resolveCollection resolves which collection applies for the current request.
//
// Resolution order (most specific wins):
//  1. Per-user override (mhs_user_progress.collection_override_id)
//  2. Per-group pin (group_app_settings.mhs_collection_id for user's MHS group)
//  3. Workspace active collection (site_settings.mhs_active_collection_id)
//  4. None
func (h *Handler) resolveCollection(r *http.Request) resolvedCollection {
	ctx := r.Context()
	user, authenticated := auth.CurrentUser(r)

	if authenticated {
		userOID, err := primitive.ObjectIDFromHex(user.ID)
		if err == nil {
			wsID := workspace.IDFromRequest(r)

			// 1. Per-user override
			progress, err := h.ProgressStore.GetOrCreate(ctx, wsID, userOID)
			if err == nil && progress.CollectionOverrideID != nil && !progress.CollectionOverrideID.IsZero() {
				coll, err := h.CollectionStore.GetByID(ctx, *progress.CollectionOverrideID)
				if err == nil {
					return resolvedCollection{Collection: coll, Found: true, IsOverride: true}
				}
				if err != mhscollections.ErrNotFound {
					h.Log.Error("failed to load user override collection", zap.Error(err))
				}
			}

			// 2. Per-group pin
			if groupCollID := groupappstore.GetMHSCollectionForUser(ctx, h.DB, userOID); groupCollID != nil {
				coll, err := h.CollectionStore.GetByID(ctx, *groupCollID)
				if err == nil {
					return resolvedCollection{Collection: coll, Found: true}
				}
				if err != mhscollections.ErrNotFound {
					h.Log.Error("failed to load group pinned collection", zap.Error(err))
				}
			}
		}
	}

	// 3. Workspace active
	wsID := workspace.IDFromRequest(r)
	settings, err := h.SettingsStore.Get(ctx, wsID)
	if err != nil {
		h.Log.Error("failed to load workspace settings for manifest", zap.Error(err))
		return resolvedCollection{}
	}

	if settings.MHSActiveCollectionID != nil && !settings.MHSActiveCollectionID.IsZero() {
		coll, err := h.CollectionStore.GetByID(ctx, *settings.MHSActiveCollectionID)
		if err == nil {
			return resolvedCollection{Collection: coll, Found: true}
		}
		if err != mhscollections.ErrNotFound {
			h.Log.Error("failed to load active collection", zap.Error(err))
		}
	}

	return resolvedCollection{}
}

// resolveManifest resolves the active content manifest for the current request.
func (h *Handler) resolveManifest(r *http.Request) (ContentManifest, bool) {
	rc := h.resolveCollection(r)
	if !rc.Found {
		return ContentManifest{CDNBaseURL: h.CDNBaseURL}, false
	}
	return h.collectionToManifest(r.Context(), rc.Collection), true
}

// collectionInfo holds resolved collection metadata for the UI.
type collectionInfo struct {
	Name        string
	ID          string
	Description string
	IsOverride  bool
}

// resolveEffectiveCollectionInfo returns metadata about the effective collection for the UI.
func (h *Handler) resolveEffectiveCollectionInfo(r *http.Request) collectionInfo {
	rc := h.resolveCollection(r)
	if !rc.Found {
		return collectionInfo{}
	}
	return collectionInfo{
		Name:        rc.Collection.Name,
		ID:          rc.Collection.ID.Hex(),
		Description: rc.Collection.Description,
		IsOverride:  rc.IsOverride,
	}
}

// checkMemberAuth enforces the workspace's MHS member auth method for a
// member, given credentials extracted by the caller (form values or a JSON
// body). It returns (0, "") when the request is authorized; otherwise an
// HTTP status and message for the caller to write. Non-members always pass.
// The client auth modal alone can be bypassed with a direct POST, so every
// gated member action must run this server-side.
//
// An active staff unlock (see unlockKey) authorizes without credentials and
// slides its expiry; fresh credentials grant a new unlock as a side effect.
func (h *Handler) checkMemberAuth(r *http.Request, role, authToken, keyword string) (int, string) {
	if role != "member" {
		return 0, ""
	}

	wsID := workspace.IDFromRequest(r)
	settings, err := h.SettingsStore.Get(r.Context(), wsID)
	if err != nil {
		h.Log.Error("failed to load settings for member auth", zap.Error(err))
		return http.StatusInternalServerError, "internal error"
	}

	mode := settings.GetMHSMemberAuth()
	if mode == "trust" {
		return 0, ""
	}

	// An active staff unlock authorizes the action; slide its expiry.
	key, wsOID, userOID, keyOK := h.unlockKey(r)
	if keyOK {
		if unlock, err := h.UnlockStore.GetActive(r.Context(), key); err != nil {
			h.Log.Error("failed to check staff unlock", zap.Error(err))
		} else if unlock != nil {
			if err := h.UnlockStore.Refresh(r.Context(), key, settings.GetMHSStaffUnlockDuration()); err != nil {
				h.Log.Warn("failed to refresh staff unlock", zap.Error(err))
			}
			return 0, ""
		}
	}

	// Back off a member who is repeatedly failing credential checks (idle
	// scripting of the gate). Keyed per member+workspace so it survives a new
	// session token. Non-attempts (empty credential) are not counted below.
	throttleKey := userOID.Hex() + ":" + wsOID.Hex()
	now := time.Now()
	if wait := h.authThrottle.retryAfter(throttleKey, now); wait > 0 {
		h.Log.Warn("throttled MHS member-auth attempt",
			zap.String("member_user_id", userOID.Hex()),
			zap.Duration("retry_after", wait))
		return http.StatusTooManyRequests, "Too many attempts. Please wait a moment and try again."
	}

	grantedBy := ""
	authFailed := false
	switch mode {
	case "staffauth":
		if authToken == "" {
			return http.StatusForbidden, "authorization token required"
		}
		ch, err := h.StaffAuthVerifier.Store.ValidateAndConsumeToken(r.Context(), authToken)
		if err != nil {
			authFailed = true
			break
		}
		grantedBy = ch.StaffName
	case "keyword":
		if keyword == "" {
			return http.StatusForbidden, "invalid keyword"
		}
		// Constant-time to avoid leaking how many leading characters matched.
		if subtle.ConstantTimeCompare([]byte(keyword), []byte(settings.MHSMemberAuthKeyword)) != 1 {
			authFailed = true
			break
		}
		grantedBy = "keyword"
	}
	if authFailed {
		h.authThrottle.fail(throttleKey, now)
		if mode == "staffauth" {
			return http.StatusForbidden, "invalid or expired authorization token"
		}
		return http.StatusForbidden, "invalid keyword"
	}
	h.authThrottle.success(throttleKey)

	// Fresh credentials start a staff unlock so follow-up actions in this
	// session don't re-prompt until it expires or is locked.
	if keyOK {
		if err := h.UnlockStore.Grant(r.Context(), key, wsOID, userOID, grantedBy, settings.GetMHSStaffUnlockDuration()); err != nil {
			h.Log.Warn("failed to grant staff unlock", zap.Error(err))
		} else {
			h.Log.Info("staff unlock granted",
				zap.String("workspace_id", wsOID.Hex()),
				zap.String("member_user_id", userOID.Hex()),
				zap.String("granted_by", grantedBy),
			)
		}
	}
	return 0, ""
}

// authThrottleKey returns the per-user backoff key for the current session
// (userID:workspace), and false when there is no user context. It matches the
// key checkMemberAuth uses so the two credential surfaces share one budget.
func (h *Handler) authThrottleKey(r *http.Request) (string, bool) {
	user, ok := auth.CurrentUser(r)
	if !ok {
		return "", false
	}
	return user.ID + ":" + workspace.IDFromRequest(r).Hex(), true
}

// unlockKey derives the staff-unlock lookup key for the current request's
// member session. Returns ok=false when the user context is unavailable.
func (h *Handler) unlockKey(r *http.Request) (key string, wsID, userID primitive.ObjectID, ok bool) {
	user, found := auth.CurrentUser(r)
	if !found {
		return "", primitive.NilObjectID, primitive.NilObjectID, false
	}
	userID, err := primitive.ObjectIDFromHex(user.ID)
	if err != nil {
		return "", primitive.NilObjectID, primitive.NilObjectID, false
	}
	wsID = workspace.IDFromRequest(r)
	return staffauth.UnlockKey(wsID, userID, user.SessionToken()), wsID, userID, true
}

// requireMemberAuth enforces the workspace's MHS member auth method for a
// member request, using the auth_token / keyword form values as proof.
// Returns false after writing an error response when not authorized.
func (h *Handler) requireMemberAuth(w http.ResponseWriter, r *http.Request, user *auth.SessionUser) bool {
	if user.Role != "member" {
		return true
	}
	if status, msg := h.checkMemberAuth(r, user.Role, r.FormValue("auth_token"), r.FormValue("keyword")); status != 0 {
		http.Error(w, msg, status)
		return false
	}
	return true
}

// HandleSetCollectionOverride sets or clears the per-user collection override.
// For members, requires the workspace's MHS member auth method.
// For admins/coordinators, no extra auth needed.
func (h *Handler) HandleSetCollectionOverride(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.CurrentUser(r)
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if err := r.ParseMultipartForm(1024); err != nil {
		if err2 := r.ParseForm(); err2 != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
	}

	if !h.requireMemberAuth(w, r, user) {
		return
	}

	collIDStr := r.FormValue("collection_id")

	userOID, err := primitive.ObjectIDFromHex(user.ID)
	if err != nil {
		http.Error(w, "invalid user", http.StatusBadRequest)
		return
	}

	wsID := workspace.IDFromRequest(r)
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	if collIDStr == "" || collIDStr == "default" {
		// Clear the override
		if err := h.ProgressStore.SetCollectionOverride(ctx, wsID, userOID, nil); err != nil {
			h.Log.Error("failed to clear collection override", zap.Error(err))
			http.Error(w, "failed to clear override", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true,"cleared":true}`))
		return
	}

	// Verify collection exists
	oid, err := primitive.ObjectIDFromHex(collIDStr)
	if err != nil {
		http.Error(w, "invalid collection_id", http.StatusBadRequest)
		return
	}
	if _, err := h.CollectionStore.GetByID(ctx, oid); err != nil {
		http.Error(w, "collection not found", http.StatusNotFound)
		return
	}

	// Set the override
	if err := h.ProgressStore.SetCollectionOverride(ctx, wsID, userOID, &oid); err != nil {
		h.Log.Error("failed to set collection override", zap.Error(err))
		http.Error(w, "failed to set override", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"ok":true}`))
}

// HandleClearCollectionOverride clears the per-user collection override.
// Reverting to the default collection is also a version switch, so members
// need the same authorization as setting an override.
func (h *Handler) HandleClearCollectionOverride(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.CurrentUser(r)
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if !h.requireMemberAuth(w, r, user) {
		return
	}

	userOID, err := primitive.ObjectIDFromHex(user.ID)
	if err != nil {
		http.Error(w, "invalid user", http.StatusBadRequest)
		return
	}

	wsID := workspace.IDFromRequest(r)
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	if err := h.ProgressStore.SetCollectionOverride(ctx, wsID, userOID, nil); err != nil {
		h.Log.Error("failed to clear collection override", zap.Error(err))
		http.Error(w, "failed to clear override", http.StatusInternalServerError)
		return
	}

	// Redirect back to units page
	http.Redirect(w, r, "/missionhydrosci/units", http.StatusSeeOther)
}

// ServeCollectionList returns available collections as JSON for the picker.
func (h *Handler) ServeCollectionList(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	collections, err := h.CollectionStore.List(ctx, 50)
	if err != nil {
		h.Log.Error("failed to list collections", zap.Error(err))
		http.Error(w, "failed to list collections", http.StatusInternalServerError)
		return
	}

	type collectionItem struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		UnitsSummary string `json:"units_summary"`
	}

	items := make([]collectionItem, len(collections))
	for i, c := range collections {
		var parts []string
		for _, u := range c.Units {
			parts = append(parts, u.UnitID+":v"+u.Version)
		}
		summary := strings.Join(parts, ", ")
		items[i] = collectionItem{
			ID:           c.ID.Hex(),
			Name:         c.Name,
			UnitsSummary: summary,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items)
}

// collectionToManifest converts an MHSCollection to the ContentManifest
// format expected by the service worker and client.
// collectionToManifest converts a collection to a ContentManifest by looking up
// file data from mhs_builds (the single source of truth).
func (h *Handler) collectionToManifest(ctx context.Context, coll models.MHSCollection) ContentManifest {
	// Batch lookup all unit+version pairs from the collection
	pairs := make([]mhsbuilds.UnitVersionPair, len(coll.Units))
	for i, u := range coll.Units {
		pairs[i] = mhsbuilds.UnitVersionPair{UnitID: u.UnitID, Version: u.Version}
	}
	buildMap, err := h.BuildStore.GetByUnitVersionBatch(ctx, pairs)
	if err != nil {
		h.Log.Error("failed to batch lookup builds for manifest", zap.Error(err))
		buildMap = make(map[string]models.MHSBuild)
	}

	units := make([]ContentManifestUnit, 0, len(coll.Units))
	for _, u := range coll.Units {
		key := u.UnitID + ":" + u.Version
		build, found := buildMap[key]
		if !found {
			h.Log.Warn("build record missing for collection unit — unit will be excluded from manifest",
				zap.String("unit_id", u.UnitID),
				zap.String("version", u.Version),
				zap.String("collection", coll.Name),
			)
			continue // skip units with no build data rather than serving empty files
		}

		files := make([]ContentManifestFile, len(build.Files))
		for j, f := range build.Files {
			files[j] = ContentManifestFile{
				Path: f.Path,
				Size: f.Size,
			}
		}
		units = append(units, ContentManifestUnit{
			ID:              u.UnitID,
			Title:           u.Title,
			Version:         u.Version,
			BuildIdentifier: u.BuildIdentifier,
			DataFile:        build.DataFile,
			FrameworkFile:   build.FrameworkFile,
			CodeFile:        build.CodeFile,
			Files:           files,
			TotalSize:       build.TotalSize,
		})
	}
	return ContentManifest{
		CDNBaseURL: h.CDNBaseURL,
		Units:      units,
	}
}

// ServeContentManifest returns the content manifest as JSON.
func (h *Handler) ServeContentManifest(w http.ResponseWriter, r *http.Request) {
	manifest, ok := h.resolveManifest(r)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ContentManifest{CDNBaseURL: h.CDNBaseURL})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(manifest)
}
