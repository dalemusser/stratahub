// internal/app/features/missionhydrosci/device_status.go
package missionhydrosci

import (
	"encoding/json"
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
		LoginID:       user.LoginID,
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
