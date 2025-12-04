// internal/app/features/dashboard/handler.go
package dashboard

import (
	"net/http"
	"strings"

	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

type Handler struct {
	DB  *mongo.Database
	Log *zap.Logger
}

func NewHandler(db *mongo.Database, logger *zap.Logger) *Handler {
	return &Handler{
		DB:  db,
		Log: logger,
	}
}

func (h *Handler) ServeDashboard(w http.ResponseWriter, r *http.Request) {
	role, _, _, ok := authz.UserCtx(r)
	if !ok {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	switch strings.ToLower(strings.TrimSpace(role)) {
	case "admin":
		h.ServeAdmin(w, r)
	case "analyst":
		h.ServeAnalyst(w, r)
	case "leader":
		h.ServeLeader(w, r)
	case "member":
		h.ServeMember(w, r)
	default:
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}
