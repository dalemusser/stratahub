// internal/app/features/gameconfig/handler.go
package gameconfig

import (
	"encoding/json"
	"net/http"

	"github.com/dalemusser/stratahub/internal/app/system/auth"
)

// ServiceEntry is a single service endpoint with URL and auth.
type ServiceEntry struct {
	URL  string `json:"url"`
	Auth string `json:"auth"`
}

// GameConfigResponse is the JSON response for /api/game-config.
type GameConfigResponse struct {
	Game     string                  `json:"game"`
	Services map[string]ServiceEntry `json:"services"`
}

// Handler serves game configuration for authenticated sessions.
type Handler struct {
	// configs maps game ID → service config.
	configs map[string]GameConfigResponse
}

// NewHandler creates a game config handler with the given per-game configs.
func NewHandler(configs map[string]GameConfigResponse) *Handler {
	return &Handler{configs: configs}
}

// ServeGameConfig returns JSON with service endpoints for the requested game.
//
// Query parameter:
//
//	game (required) — the game identifier (e.g., "mhs")
//
// Response format:
//
//	{ "game": "mhs", "services": { "log": { "url": "...", "auth": "..." }, "save": { "url": "...", "auth": "..." } } }
func (h *Handler) ServeGameConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Require authentication
	_, ok := auth.CurrentUser(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": "authentication required",
		})
		return
	}

	game := r.URL.Query().Get("game")
	if game == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": "missing required query parameter: game",
		})
		return
	}

	cfg, exists := h.configs[game]
	if !exists {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": "no configuration found for game: " + game,
		})
		return
	}

	_ = json.NewEncoder(w).Encode(cfg)
}
