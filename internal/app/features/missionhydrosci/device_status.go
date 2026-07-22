// internal/app/features/missionhydrosci/device_status.go
package missionhydrosci

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

// deviceStatusRequest is the JSON body for POST /api/device-status.
type deviceStatusRequest struct {
	DeviceID      string            `json:"device_id"`
	DeviceType    string            `json:"device_type"`
	DeviceDetails map[string]string `json:"device_details,omitempty"`
	PWAInstalled  bool              `json:"pwa_installed"`
	SWRegistered  bool              `json:"sw_registered"`
	UnitStatus    map[string]string `json:"unit_status"`
	StorageQuota  int64             `json:"storage_quota"`
	StorageUsage  int64             `json:"storage_usage"`
}

// HandleDeviceStatus receives a device status report from the client.
func (h *Handler) HandleDeviceStatus(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.CurrentUser(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req deviceStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.DeviceID == "" {
		http.Error(w, "device_id is required", http.StatusBadRequest)
		return
	}

	wsID := workspace.IDFromRequest(r)
	userID, err := primitive.ObjectIDFromHex(user.ID)
	if err != nil {
		http.Error(w, "invalid user", http.StatusBadRequest)
		return
	}

	unitStatus := req.UnitStatus
	if unitStatus == nil {
		unitStatus = map[string]string{}
	}

	status := models.MHSDeviceStatus{
		WorkspaceID:   wsID,
		UserID:        userID,
		DeviceID:      req.DeviceID,
		DeviceType:    req.DeviceType,
		DeviceDetails: req.DeviceDetails,
		PWAInstalled:  req.PWAInstalled,
		SWRegistered:  req.SWRegistered,
		UnitStatus:    unitStatus,
		StorageQuota:  req.StorageQuota,
		StorageUsage:  req.StorageUsage,
	}

	if err := h.DeviceStatusStore.Upsert(r.Context(), status); err != nil {
		h.Log.Error("failed to upsert device status", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// downloadErrorRequest is the JSON body for POST /api/download-error.
type downloadErrorRequest struct {
	DeviceID     string `json:"device_id"`
	DeviceType   string `json:"device_type"`
	Unit         string `json:"unit"`
	Version      string `json:"version"`
	ErrorClass   string `json:"error_class"`
	Message      string `json:"message"`
	Path         string `json:"path"` // "bg" | "fallback" | ""
	StorageQuota int64  `json:"storage_quota"`
	StorageUsage int64  `json:"storage_usage"`
	UserAgent    string `json:"user_agent"`
}

// HandleDownloadError records a client-side unit-download failure. It is
// log-only (no stored schema): each event becomes one structured log line so
// failure prevalence across devices/workspaces is greppable server-side — the
// diagnostics gap MHS-008 called out (a raw cache/network error was previously
// visible only to the tester on their screen). Bounded so a scripted client
// can't flood the logs with oversized strings.
func (h *Handler) HandleDownloadError(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.CurrentUser(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req downloadErrorRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	clip := func(s string, n int) string {
		if len(s) > n {
			return s[:n]
		}
		return s
	}

	h.Log.Warn("mhs download error",
		zap.String("workspace_id", workspace.IDFromRequest(r).Hex()),
		zap.String("user_id", user.ID),
		zap.String("device_id", clip(req.DeviceID, 64)),
		zap.String("device_type", clip(req.DeviceType, 32)),
		zap.String("unit", clip(req.Unit, 32)),
		zap.String("version", clip(req.Version, 32)),
		zap.String("error_class", clip(req.ErrorClass, 48)),
		zap.String("path", clip(req.Path, 16)),
		zap.Int64("storage_quota", req.StorageQuota),
		zap.Int64("storage_usage", req.StorageUsage),
		zap.String("message", clip(req.Message, 500)),
		zap.String("user_agent", clip(req.UserAgent, 300)),
	)

	w.WriteHeader(http.StatusNoContent)
}
