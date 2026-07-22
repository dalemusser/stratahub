// internal/app/features/missionhydrosci/progress.go
package missionhydrosci

import (
	"encoding/json"
	"net/http"

	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

// progressResponse is the JSON response for GET /api/progress.
type progressResponse struct {
	CurrentUnit    string   `json:"current_unit"`
	CompletedUnits []string `json:"completed_units"`
	IsComplete     bool     `json:"is_complete"`
}

// completeRequest is the JSON body for POST /api/progress/complete.
type completeRequest struct {
	Unit string `json:"unit"`
}

// completeResponse is the JSON response for POST /api/progress/complete.
type completeResponse struct {
	NextUnit string `json:"next_unit"`
	IsFinal  bool   `json:"is_final"`
}

// ServeProgress returns the current user's progress through Mission HydroSci units.
func (h *Handler) ServeProgress(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.CurrentUser(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	wsID := workspace.IDFromRequest(r)
	userID, err := primitive.ObjectIDFromHex(user.ID)
	if err != nil {
		http.Error(w, "invalid user", http.StatusBadRequest)
		return
	}

	progress, err := h.ProgressStore.GetOrCreate(r.Context(), wsID, userID)
	if err != nil {
		h.Log.Error("failed to get progress", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	completed := progress.CompletedUnits
	if completed == nil {
		completed = []string{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(progressResponse{
		CurrentUnit:    progress.CurrentUnit,
		CompletedUnits: completed,
		IsComplete:     progress.CurrentUnit == "complete",
	})
}


// setToUnitRequest is the JSON body for POST /api/progress/set-unit.
type setToUnitRequest struct {
	Unit      string `json:"unit"`
	AuthToken string `json:"auth_token,omitempty"` // Staff auth token (when staffauth mode)
	Keyword   string `json:"keyword,omitempty"`    // Keyword (when keyword mode)
}

// HandleSetToUnit sets the user's progress to a specific unit.
func (h *Handler) HandleSetToUnit(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.CurrentUser(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req setToUnitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Validate the requested unit against this user's resolved manifest rather
	// than a fixed unit1..unitN list. Collections are data-driven, so the unit
	// set can change (e.g. a future unit6) — the manifest is the source of truth.
	manifest, _ := h.resolveManifest(r)
	validUnit := false
	for _, u := range manifest.Units {
		if u.ID == req.Unit {
			validUnit = true
			break
		}
	}
	if !validUnit {
		http.Error(w, "invalid unit", http.StatusBadRequest)
		return
	}

	wsID := workspace.IDFromRequest(r)

	// Server-side authorization for members (credentials come from the JSON
	// body here; the shared core also serves the form-based endpoints)
	if status, msg := h.checkMemberAuth(r, user.Role, req.AuthToken, req.Keyword); status != 0 {
		http.Error(w, msg, status)
		return
	}

	userID, err := primitive.ObjectIDFromHex(user.ID)
	if err != nil {
		http.Error(w, "invalid user", http.StatusBadRequest)
		return
	}

	if err := h.ProgressStore.SetToUnit(r.Context(), wsID, userID, req.Unit); err != nil {
		h.Log.Error("failed to set progress to unit", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true, "unit": req.Unit})
}

// HandleCompleteUnit marks a unit as completed and advances to the next one.
func (h *Handler) HandleCompleteUnit(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.CurrentUser(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req completeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Unit == "" {
		http.Error(w, "unit is required", http.StatusBadRequest)
		return
	}

	wsID := workspace.IDFromRequest(r)
	userID, err := primitive.ObjectIDFromHex(user.ID)
	if err != nil {
		http.Error(w, "invalid user", http.StatusBadRequest)
		return
	}

	// Get total units from the active manifest for this user
	manifest, _ := h.resolveManifest(r)
	totalUnits := len(manifest.Units)
	if totalUnits == 0 {
		totalUnits = 5 // fallback
	}

	progress, err := h.ProgressStore.CompleteUnit(r.Context(), wsID, userID, req.Unit, totalUnits)
	if err != nil {
		h.Log.Error("failed to complete unit", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	var nextUnit string
	isFinal := progress.CurrentUnit == "complete"
	if !isFinal {
		nextUnit = progress.CurrentUnit
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(completeResponse{
		NextUnit: nextUnit,
		IsFinal:  isFinal,
	})
}
