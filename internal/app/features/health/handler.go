package health

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"go.uber.org/zap"
)

// Handler holds dependencies needed for health checks.
type Handler struct {
	Client *mongo.Client
	Log    *zap.Logger
}

// NewHandler constructs a health Handler with the Mongo client and logger.
func NewHandler(client *mongo.Client, logger *zap.Logger) *Handler {
	return &Handler{
		Client: client,
		Log:    logger,
	}
}

// Serve handles GET /health.
//
// On success: 200 and
//
//	{ "status":"ok", "database":"connected" }
//
// On DB failure: 503 and
//
//	{ "status":"error", "message":"Database unavailable", "error":"…"}
func (h *Handler) Serve(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Ping())
	defer cancel()

	w.Header().Set("Content-Type", "application/json")

	if err := h.Client.Ping(ctx, readpref.Primary()); err != nil {
		h.Log.Error("health-check: mongo ping failed", zap.Error(err))
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "error",
			"message": "Database unavailable",
			"error":   err.Error(),
		})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":   "ok",
		"database": "connected",
	})
}
