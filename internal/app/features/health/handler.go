package health

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/dalemusser/stratahub/internal/app/system/certcheck"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"go.uber.org/zap"
)

// Handler holds dependencies needed for health checks.
type Handler struct {
	Client  *mongo.Client
	BaseURL string
	Log     *zap.Logger
}

// NewHandler constructs a health Handler with the Mongo client and logger.
func NewHandler(client *mongo.Client, baseURL string, logger *zap.Logger) *Handler {
	return &Handler{
		Client:  client,
		BaseURL: baseURL,
		Log:     logger,
	}
}

// healthResponse is the JSON structure for the health check response.
type healthResponse struct {
	Status   string       `json:"status"`
	Database string       `json:"database"`
	Message  string       `json:"message,omitempty"`
	Error    string       `json:"error,omitempty"`
	Cert     *certStatus  `json:"cert,omitempty"`
}

// certStatus is a simplified cert status for the health endpoint.
type certStatus struct {
	DaysLeft int  `json:"days_left"`
	Valid    bool `json:"valid"`
}

// Serve handles GET /health.
//
// On success: 200 and
//
//	{ "status":"ok", "database":"connected", "cert":{"days_left":30,"valid":true} }
//
// On DB failure: 503 and
//
//	{ "status":"error", "message":"Database unavailable", "error":"…"}
func (h *Handler) Serve(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Ping())
	defer cancel()

	w.Header().Set("Content-Type", "application/json")

	resp := healthResponse{
		Status:   "ok",
		Database: "connected",
	}

	// Check database
	if err := h.Client.Ping(ctx, readpref.Primary()); err != nil {
		h.Log.Error("health-check: mongo ping failed", zap.Error(err))
		w.WriteHeader(http.StatusServiceUnavailable)
		resp.Status = "error"
		resp.Database = "disconnected"
		resp.Message = "Database unavailable"
		resp.Error = err.Error()
		_ = json.NewEncoder(w).Encode(resp)
		return
	}

	// Check certificate (non-blocking, informational only)
	if h.BaseURL != "" {
		certInfo := certcheck.Check(h.BaseURL)
		resp.Cert = &certStatus{
			DaysLeft: certInfo.DaysLeft,
			Valid:    certInfo.IsValid,
		}
	}

	_ = json.NewEncoder(w).Encode(resp)
}
