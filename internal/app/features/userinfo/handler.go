// internal/app/features/userinfo/handler.go
package userinfo

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import (
	"encoding/json"
	"net/http"

	"github.com/dalemusser/stratahub/internal/app/system/auth"
)

// Handler serves user information for authenticated sessions.
type Handler struct{}

// NewHandler creates a new userinfo handler.
func NewHandler() *Handler {
	return &Handler{}
}

// ServeUserInfo returns JSON with the current user's authentication status and identity.
//
// Response format:
//
//	{ "isAuthenticated": bool, "name": "...", "user_id": "..." }
//
// user_id is the 24-character hex string of the user's MongoDB ObjectID.
// It is the only identity field — login_id and email are intentionally not
// exposed here so that downstream services (stratalog, stratasave, mhsgrader)
// receive a non-identifiable identifier.
func (h *Handler) ServeUserInfo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	user, ok := auth.CurrentUser(r)
	if !ok {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"isAuthenticated": false,
			"name":            "",
			"user_id":         "",
		})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"isAuthenticated": true,
		"name":            user.Name,
		"user_id":         user.ID,
	})
}
