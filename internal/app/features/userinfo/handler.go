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
//	{ "isAuthenticated": bool, "name": "...", "user_id": "...", "email": "...", "login_id": "..." }
//
// Three identity fields are provided for transition support:
//   - "user_id"  — canonical identity field; currently carries login_id,
//     will carry the user's MongoDB ObjectID in a future phase.
//   - "login_id" — the human-readable login string.
//   - "email"    — legacy alias for login_id (backwards compat with older games).
func (h *Handler) ServeUserInfo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	user, ok := auth.CurrentUser(r)
	if !ok {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"isAuthenticated": false,
			"name":            "",
			"user_id":         "",
			"email":           "",
			"login_id":        "",
		})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"isAuthenticated": true,
		"name":            user.Name,
		"user_id":         user.LoginID, // Phase 1: login_id; Phase 2: user.ID.Hex()
		"email":           user.LoginID, // Legacy compat
		"login_id":        user.LoginID,
	})
}
