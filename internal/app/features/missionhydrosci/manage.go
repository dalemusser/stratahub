// internal/app/features/missionhydrosci/manage.go
package missionhydrosci

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/app/system/format"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"github.com/dalemusser/waffle/pantry/templates"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

// ManageData is the view model for the manage page (and its auth gate).
type ManageData struct {
	viewdata.BaseVM

	// Gate state. Gated = member in a keyword/staffauth workspace: the page
	// content renders only while an unlock is active; otherwise the gate view.
	Gated             bool
	Unlocked          bool   // active staff unlock exists (only meaningful when Gated)
	UnlockGrantedBy   string // staff name or "keyword"
	UnlockExpiresAtMs int64  // unix ms, for the countdown
	ServerNowMs       int64  // unix ms at render; the countdown measures against this, not the device clock (skew-proof)
	MHSMemberAuth     string // "keyword" or "staffauth" — selects the gate flow

	// Page content (populated when the page renders, not for the gate)
	Units          []UnitVM
	CurrentUnit    string
	CompletedUnits []string
	IsComplete     bool
	NextUnitID     string // current/next are auto-managed; manage JS needs them for manual-download tracking

	CollectionOverride   bool
	ActiveCollectionName string
	ActiveCollectionID   string
	ActiveCollectionDesc string
}

// ServeManage renders the manage page, or its staff-authorization gate for
// members in keyword/staffauth workspaces without an active unlock. The gate
// is server-enforced: page content is never sent to a gated, locked session.
func (h *Handler) ServeManage(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.CurrentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	wsID := workspace.IDFromRequest(r)
	mhsMemberAuth := "staffauth"
	if settings, err := h.SettingsStore.Get(r.Context(), wsID); err == nil {
		mhsMemberAuth = settings.GetMHSMemberAuth()
	}

	data := ManageData{
		BaseVM:        viewdata.LoadBase(r, h.DB),
		Gated:         user.Role == "member" && mhsMemberAuth != "trust",
		MHSMemberAuth: mhsMemberAuth,
	}
	data.Title = "Manage Mission HydroSci"

	if data.Gated {
		if key, _, _, keyOK := h.unlockKey(r); keyOK {
			if unlock, err := h.UnlockStore.GetActive(r.Context(), key); err != nil {
				h.Log.Error("failed to check staff unlock for manage page", zap.Error(err))
			} else if unlock != nil {
				data.Unlocked = true
				data.UnlockGrantedBy = unlock.GrantedBy
				data.UnlockExpiresAtMs = unlock.ExpiresAt.UnixMilli()
				data.ServerNowMs = time.Now().UnixMilli()
			}
		}
		if !data.Unlocked {
			// Gate view only — no page content for a locked member session.
			templates.Render(w, r, "missionhydrosci_manage", data)
			return
		}
	}

	// Page content
	manifest, _ := h.resolveManifest(r)

	var currentUnit string
	var completedUnits []string
	var isComplete bool
	userID, err := primitive.ObjectIDFromHex(user.ID)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "invalid user id on MHS manage page", err, "Something went wrong loading Mission HydroSci.", "/missionhydrosci/units")
		return
	}
	// A DB failure here must not silently render an empty/default page and burn
	// the member's unlock time — surface it.
	progress, err := h.ProgressStore.GetOrCreate(r.Context(), wsID, userID)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "load MHS progress failed (manage)", err, "Couldn't load your Mission HydroSci data. Please try again.", "/missionhydrosci/units")
		return
	}
	currentUnit = progress.CurrentUnit
	completedUnits = progress.CompletedUnits
	isComplete = progress.CurrentUnit == "complete"
	if currentUnit == "" {
		currentUnit = "unit1"
	}
	if completedUnits == nil {
		completedUnits = []string{}
	}

	completedSet := make(map[string]bool, len(completedUnits))
	for _, u := range completedUnits {
		completedSet[u] = true
	}

	units := make([]UnitVM, len(manifest.Units))
	var nextUnitID string
	for i, u := range manifest.Units {
		var status string
		switch {
		case completedSet[u.ID]:
			status = "completed"
		case u.ID == currentUnit:
			status = "current"
		default:
			status = "future"
		}
		units[i] = UnitVM{
			ID:              u.ID,
			Title:           u.Title,
			Version:         u.Version,
			BuildIdentifier: u.BuildIdentifier,
			TotalSize:       u.TotalSize,
			SizeLabel:       format.Bytes(u.TotalSize),
			Status:          status,
		}
		if !isComplete && u.ID == currentUnit && i+1 < len(manifest.Units) {
			nextUnitID = manifest.Units[i+1].ID
		}
	}

	collInfo := h.resolveEffectiveCollectionInfo(r)

	data.Units = units
	data.CurrentUnit = currentUnit
	data.CompletedUnits = completedUnits
	data.IsComplete = isComplete
	data.NextUnitID = nextUnitID
	data.CollectionOverride = collInfo.IsOverride
	data.ActiveCollectionName = collInfo.Name
	data.ActiveCollectionID = collInfo.ID
	data.ActiveCollectionDesc = collInfo.Description

	templates.Render(w, r, "missionhydrosci_manage", data)
}

// manageUnlockRequest is the JSON body for POST /api/manage/unlock.
type manageUnlockRequest struct {
	AuthToken string `json:"auth_token,omitempty"`
	Keyword   string `json:"keyword,omitempty"`
}

// HandleManageUnlock starts a staff unlock for a gated member session from
// the manage page's gate view. checkMemberAuth validates the credentials and
// grants the unlock as a side effect.
func (h *Handler) HandleManageUnlock(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.CurrentUser(r)
	if !ok {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req manageUnlockRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if status, msg := h.checkMemberAuth(r, user.Role, req.AuthToken, req.Keyword); status != 0 {
		jsonError(w, msg, status)
		return
	}

	resp := map[string]any{"ok": true}
	if key, _, _, keyOK := h.unlockKey(r); keyOK {
		if unlock, err := h.UnlockStore.GetActive(r.Context(), key); err == nil && unlock != nil {
			resp["granted_by"] = unlock.GrantedBy
			resp["expires_at_ms"] = unlock.ExpiresAt.UnixMilli()
			resp["server_now_ms"] = time.Now().UnixMilli()
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// HandleManageLock revokes the staff unlock for the current session
// ("Lock now"). Safe to call when no unlock exists.
func (h *Handler) HandleManageLock(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.CurrentUser(r)
	if !ok {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	if key, wsOID, userOID, keyOK := h.unlockKey(r); keyOK {
		if err := h.UnlockStore.Revoke(r.Context(), key); err != nil {
			h.Log.Error("failed to revoke staff unlock", zap.Error(err))
			jsonError(w, "failed to lock", http.StatusInternalServerError)
			return
		}
		h.Log.Info("staff unlock revoked",
			zap.String("workspace_id", wsOID.Hex()),
			zap.String("member_user_id", userOID.Hex()),
			zap.String("role", user.Role),
		)
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"ok":true}`))
}

// ServeManageStatus reports the gate/unlock state for the current session.
// The manage page uses it to keep the countdown accurate after actions.
func (h *Handler) ServeManageStatus(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.CurrentUser(r)
	if !ok {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	wsID := workspace.IDFromRequest(r)
	mode := "staffauth"
	if settings, err := h.SettingsStore.Get(r.Context(), wsID); err == nil {
		mode = settings.GetMHSMemberAuth()
	}

	resp := map[string]any{
		"gated":    user.Role == "member" && mode != "trust",
		"unlocked": false,
	}
	if resp["gated"] == true {
		if key, _, _, keyOK := h.unlockKey(r); keyOK {
			if unlock, err := h.UnlockStore.GetActive(r.Context(), key); err == nil && unlock != nil {
				resp["unlocked"] = true
				resp["granted_by"] = unlock.GrantedBy
				resp["expires_at_ms"] = unlock.ExpiresAt.UnixMilli()
				resp["server_now_ms"] = time.Now().UnixMilli()
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
