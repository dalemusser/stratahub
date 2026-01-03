// internal/app/features/userinfo/handler.go
package userinfo

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
//	{ "isAuthenticated": bool, "name": "...", "email": "...", "login_id": "..." }
//
// The "email" field is set to login_id for backwards compatibility with existing games.
// New integrations should use "login_id" instead.
func (h *Handler) ServeUserInfo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	user, ok := auth.CurrentUser(r)
	if !ok {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"isAuthenticated": false,
			"name":            "",
			"email":           "",
			"login_id":        "",
		})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"isAuthenticated": true,
		"name":            user.Name,
		"email":           user.LoginID, // For backwards compatibility with existing games
		"login_id":        user.LoginID,
	})
}
