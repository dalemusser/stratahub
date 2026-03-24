package missionhydrosci

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/dalemusser/stratahub/internal/app/store/emailverify"
	"github.com/dalemusser/stratahub/internal/app/system/staffauth"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

type staffAuthStartRequest struct {
	LoginID string `json:"login_id"`
}

type staffAuthStartResponse struct {
	Method      string `json:"method"`
	ChallengeID string `json:"challenge_id,omitempty"`
	Token       string `json:"token,omitempty"`
	StaffName   string `json:"staff_name"`
}

type staffAuthVerifyRequest struct {
	ChallengeID string `json:"challenge_id"`
	Credential  string `json:"credential"`
}

type staffAuthResendRequest struct {
	ChallengeID string `json:"challenge_id"`
}

type staffAuthKeywordRequest struct {
	Keyword string `json:"keyword"`
}

// HandleStaffAuthStart begins a staff authentication flow.
func (h *Handler) HandleStaffAuthStart(w http.ResponseWriter, r *http.Request) {
	var req staffAuthStartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.LoginID == "" {
		jsonError(w, "login_id is required", http.StatusBadRequest)
		return
	}

	wsID := workspace.IDFromRequest(r)
	result, err := h.StaffAuthVerifier.StartAuth(r.Context(), wsID, req.LoginID)
	if err != nil {
		switch err {
		case staffauth.ErrUserNotFound:
			jsonError(w, "User not found. Check the login ID and try again.", http.StatusNotFound)
		case staffauth.ErrUserDisabled:
			jsonError(w, "This user account is disabled.", http.StatusForbidden)
		case staffauth.ErrUserNotStaff:
			jsonError(w, "This user is not a leader, coordinator, or admin.", http.StatusForbidden)
		case staffauth.ErrUserWrongWorkspace:
			jsonError(w, "This user does not belong to this workspace.", http.StatusForbidden)
		case staffauth.ErrNoEmail:
			jsonError(w, "This user has email authentication but no email address is available. The account may need to be fixed.", http.StatusBadRequest)
		default:
			h.Log.Error("staff auth start failed", zap.Error(err))
			jsonError(w, "Authentication failed. Please try again.", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(staffAuthStartResponse{
		Method:      result.Method,
		ChallengeID: result.ChallengeID,
		Token:       result.Token,
		StaffName:   result.StaffName,
	})
}

// HandleStaffAuthVerify verifies a credential against a pending challenge.
func (h *Handler) HandleStaffAuthVerify(w http.ResponseWriter, r *http.Request) {
	var req staffAuthVerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.ChallengeID == "" || req.Credential == "" {
		jsonError(w, "challenge_id and credential are required", http.StatusBadRequest)
		return
	}

	result, err := h.StaffAuthVerifier.VerifyAuth(r.Context(), req.ChallengeID, req.Credential)
	if err != nil {
		switch err {
		case staffauth.ErrChallengeNotFound:
			jsonError(w, "Challenge not found or expired. Please start over.", http.StatusNotFound)
		case staffauth.ErrChallengeExpired:
			jsonError(w, "Challenge expired. Please start over.", http.StatusGone)
		case staffauth.ErrCodeExpired:
			jsonError(w, "Verification code expired. Please click Resend to get a new code.", http.StatusGone)
		case staffauth.ErrTooManyAttempts:
			jsonError(w, "Too many incorrect attempts. Please start over.", http.StatusTooManyRequests)
		case staffauth.ErrInvalidCredential:
			jsonError(w, "Incorrect credential. Please try again.", http.StatusUnauthorized)
		default:
			h.Log.Error("staff auth verify failed", zap.Error(err))
			jsonError(w, "Verification failed. Please try again.", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"ok":    true,
		"token": result.Token,
	})
}

// HandleStaffAuthResend resends a verification email for an email challenge.
func (h *Handler) HandleStaffAuthResend(w http.ResponseWriter, r *http.Request) {
	var req staffAuthResendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.ChallengeID == "" {
		jsonError(w, "challenge_id is required", http.StatusBadRequest)
		return
	}

	if err := h.StaffAuthVerifier.ResendEmail(r.Context(), req.ChallengeID); err != nil {
		switch err {
		case staffauth.ErrChallengeNotFound:
			jsonError(w, "Challenge not found or expired. Please start over.", http.StatusNotFound)
		case staffauth.ErrChallengeExpired:
			jsonError(w, "Challenge expired. Please start over.", http.StatusGone)
		case emailverify.ErrTooManyResends:
			jsonError(w, "Too many resend requests. Please wait before trying again.", http.StatusTooManyRequests)
		default:
			h.Log.Error("staff auth resend failed", zap.Error(err))
			jsonError(w, "Failed to resend. Please try again.", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"ok":true}`))
}

// HandleKeywordVerify validates a keyword against the workspace setting.
func (h *Handler) HandleKeywordVerify(w http.ResponseWriter, r *http.Request) {
	var req staffAuthKeywordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	wsID := workspace.IDFromRequest(r)
	settings, err := h.loadSettings(r.Context(), wsID)
	if err != nil {
		h.Log.Error("failed to load settings for keyword verify", zap.Error(err))
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	if settings.GetMHSMemberAuth() != "keyword" {
		jsonError(w, "keyword mode is not active", http.StatusBadRequest)
		return
	}
	if req.Keyword != settings.MHSMemberAuthKeyword {
		jsonError(w, "Incorrect keyword.", http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"ok":true}`))
}

// loadSettings loads workspace site settings.
func (h *Handler) loadSettings(ctx context.Context, wsID primitive.ObjectID) (models.SiteSettings, error) {
	return h.SettingsStore.Get(ctx, wsID)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
