// internal/app/features/missionhydrosci/savedata.go
package missionhydrosci

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"go.uber.org/zap"
)

// mhsGameID is the game identifier under which Mission HydroSci save data is
// stored in stratasave. player_states and player_settings are keyed by
// {user_id, game}; verified against production data, MHS state and settings are
// both stored with game = "mhs". A delete must use this exact value or it
// silently matches nothing.
const mhsGameID = "mhs"

// stratasaveClient is the HTTP client used for server-side calls to the
// stratasave delete API. Deletes are small and should complete quickly; the
// timeout bounds a stalled upstream so the request handler can't hang.
var stratasaveClient = &http.Client{Timeout: 15 * time.Second}

// deleteSaveDataRequest is the JSON body for the save-data delete endpoints.
// It carries the same member-auth proof as the other gated MHS actions.
type deleteSaveDataRequest struct {
	AuthToken string `json:"auth_token,omitempty"` // Staff auth token (staffauth mode)
	Keyword   string `json:"keyword,omitempty"`    // Keyword (keyword mode)
}

// HandleDeleteSavedState deletes the current user's saved MHS game state from
// stratasave. State is stored as append-only history, so every saved state for
// this user in the MHS game is removed. Members must satisfy the workspace's
// MHS member auth method (staffauth/keyword/trust), the same as Set-a-Unit.
func (h *Handler) HandleDeleteSavedState(w http.ResponseWriter, r *http.Request) {
	h.handleDeleteSaveData(w, r, "state", h.Services.StateDeleteURL)
}

// HandleDeleteSavedSettings deletes the current user's saved MHS settings from
// stratasave (one document per user/game). Same member-auth rules as state.
func (h *Handler) HandleDeleteSavedSettings(w http.ResponseWriter, r *http.Request) {
	h.handleDeleteSaveData(w, r, "settings", h.Services.SettingsDeleteURL)
}

// handleDeleteSaveData is the shared implementation for the two delete handlers.
// It authorizes the request, then calls the stratasave delete endpoint with the
// shared save auth header and returns the number of records removed.
func (h *Handler) handleDeleteSaveData(w http.ResponseWriter, r *http.Request, target, endpoint string) {
	user, ok := auth.CurrentUser(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req deleteSaveDataRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Server-side authorization for members. The client auth modal alone can be
	// bypassed with a direct POST, so every gated member action must run this.
	if status, msg := h.checkMemberAuth(r, user.Role, req.AuthToken, req.Keyword); status != 0 {
		http.Error(w, msg, status)
		return
	}

	if endpoint == "" {
		h.Log.Error("MHS save-data delete endpoint not configured", zap.String("target", target))
		http.Error(w, "save data delete is not configured", http.StatusServiceUnavailable)
		return
	}

	deleted, err := h.callStratasaveDelete(r.Context(), endpoint, user.ID)
	if err != nil {
		h.Log.Error("failed to delete MHS save data via stratasave",
			zap.String("target", target),
			zap.String("user_id", user.ID),
			zap.Error(err),
		)
		http.Error(w, "failed to delete save data", http.StatusBadGateway)
		return
	}

	h.Log.Info("deleted MHS save data",
		zap.String("target", target),
		zap.String("user_id", user.ID),
		zap.Int64("deleted", deleted),
	)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true, "target": target, "deleted": deleted})
}

// callStratasaveDelete POSTs {user_id, game:"mhs"} to a stratasave delete
// endpoint using the shared save auth header, and returns the deleted count
// stratasave reports.
func (h *Handler) callStratasaveDelete(ctx context.Context, endpoint, userIDHex string) (int64, error) {
	body, err := json.Marshal(map[string]string{"user_id": userIDHex, "game": mhsGameID})
	if err != nil {
		return 0, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if h.Services.SaveAuth != "" {
		httpReq.Header.Set("Authorization", h.Services.SaveAuth)
	}

	resp, err := stratasaveClient.Do(httpReq)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errResp struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errResp)
		return 0, fmt.Errorf("stratasave delete returned %d: %s", resp.StatusCode, errResp.Error)
	}

	var result struct {
		Deleted int64 `json:"deleted"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("decode stratasave response: %w", err)
	}
	return result.Deleted, nil
}
