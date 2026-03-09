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

	progress, err := h.ProgressStore.GetOrCreate(r.Context(), wsID, userID, user.LoginID)
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

// HandleResetProgress deletes the user's progress record and returns JSON {"ok": true}.
// Only non-member roles may reset progress.
func (h *Handler) HandleResetProgress(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.CurrentUser(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	if user.Role == "member" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	wsID := workspace.IDFromRequest(r)
	userID, err := primitive.ObjectIDFromHex(user.ID)
	if err != nil {
		http.Error(w, "invalid user", http.StatusBadRequest)
		return
	}

	if err := h.ProgressStore.Delete(r.Context(), wsID, userID); err != nil {
		h.Log.Error("failed to reset progress", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"ok":true}`))
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

	progress, err := h.ProgressStore.CompleteUnit(r.Context(), wsID, userID, req.Unit)
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
