// internal/app/features/missionhydrosci/steplog_member.go
//
// Plan step A2: a signed-in member's page sends its step log when a download
// or launch fails or finishes, and it is stored beside the device-test runs
// (kind "member") so a field report can be read step by step in the Device
// Tests view without asking the student for anything.
package missionhydrosci

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

// Outcomes a member page reports. Each becomes one stored record.
const (
	memberStepLogDownloadFailed   = "download-failed"
	memberStepLogDownloadComplete = "download-complete"
	memberStepLogLaunchFailed     = "launch-failed"
	memberStepLogLaunchOK         = "launch-ok"
	memberStepLogCrash            = "crash"
)

// memberStepLogRequest is the page's body: the outcome, the unit, the log's
// context and its entries (the page sends at most the last 300).
type memberStepLogRequest struct {
	Outcome string `json:"outcome"`
	Unit    string `json:"unit"`
	Version string `json:"version"`
	deviceTestStepsRequest
}

// HandleMemberStepLog stores a member's step log as a kind "member" record.
func (h *Handler) HandleMemberStepLog(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.CurrentUser(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req memberStepLogRequest
	if err := decodeDeviceTestBody(r, &req); err != nil {
		writeDeviceTestJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	switch req.Outcome {
	case memberStepLogDownloadFailed, memberStepLogDownloadComplete, memberStepLogLaunchFailed, memberStepLogLaunchOK, memberStepLogCrash:
	default:
		writeDeviceTestJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_outcome"})
		return
	}
	if len(req.Entries) == 0 {
		writeDeviceTestJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "no_entries"})
		return
	}
	if len(req.Entries) > models.MHSDeviceTestMaxSteps {
		req.Entries = req.Entries[len(req.Entries)-models.MHSDeviceTestMaxSteps:]
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()

	userOID, err := primitive.ObjectIDFromHex(user.ID)
	if err != nil {
		http.Error(w, "invalid user", http.StatusBadRequest)
		return
	}
	now := time.Now().UTC()
	steps, derived := deriveFromSteps(req.deviceTestStepsRequest, models.MHSDeviceTestStageRun, now)

	ctxMap := boundDiagnostics(req.Context)
	str := func(k string) string {
		if v, ok := ctxMap[k].(string); ok {
			return v
		}
		return ""
	}
	rec := models.MHSDeviceTest{
		WorkspaceID:  workspace.IDFromRequest(r),
		Kind:         models.MHSDeviceTestKindMember,
		UserID:       &userOID,
		UnitID:       models.ClipRunes(strings.TrimSpace(req.Unit), 32),
		UnitVersion:  models.ClipRunes(strings.TrimSpace(req.Version), 32),
		DeviceID:     models.ClipRunes(str("deviceId"), 64),
		DeviceType:   models.ClipRunes(str("deviceType"), 40),
		RemoteIP:     clientIPForLog(r),
		UserAgent:    models.ClipRunes(r.UserAgent(), 400),
		StartedAt:    now,
		ExpiresAt:    now, // a stored record accepts no further writes
		Stage:        derived.stage,
		ReachedStage: derived.reached,
		FailedStep:   derived.failedStep,
		FailedReason: derived.failedReason,
		Diagnostics:  map[string]interface{}{"steplog_context": ctxMap, "outcome": req.Outcome},
		Steps:        steps,
	}
	if derived.gameplayAt != nil {
		rec.GameplayReachedAt = derived.gameplayAt
	}
	if user.OrganizationID != "" {
		if oid, err := primitive.ObjectIDFromHex(user.OrganizationID); err == nil {
			rec.OrganizationID = &oid
		}
	}
	if _, err := h.DeviceTestStore.Create(ctx, rec); err != nil {
		h.Log.Error("member step log: store failed", zap.Error(err))
		writeDeviceTestJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "server_error"})
		return
	}
	h.Log.Info("member step log stored",
		zap.String("workspace_id", rec.WorkspaceID.Hex()),
		zap.String("user_id", user.ID),
		zap.String("outcome", req.Outcome),
		zap.String("unit", rec.UnitID+" v"+rec.UnitVersion),
		zap.String("stage", rec.Stage),
		zap.String("failed_step", rec.FailedStep))
	writeDeviceTestJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func clientIPForLog(r *http.Request) string {
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		if i := strings.Index(ip, ","); i > 0 {
			ip = ip[:i]
		}
		return strings.TrimSpace(ip)
	}
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i > 0 {
		host = host[:i]
	}
	return host
}
