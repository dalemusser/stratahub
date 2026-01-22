// internal/app/features/dashboard/handler.go
package dashboard

import (
	"net/http"
	"strings"

	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
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

	// Check workspace context for superadmins
	// If superadmin is on a workspace subdomain (not apex), show admin dashboard
	// so they can manage that workspace with the standard admin tools
	ws := workspace.FromRequest(r)
	if strings.ToLower(strings.TrimSpace(role)) == "superadmin" {
		if ws != nil && !ws.IsApex && ws.ID.IsZero() == false {
			// Superadmin on workspace subdomain - show admin view
			h.ServeAdmin(w, r)
			return
		}
		// Superadmin on apex domain - show superadmin view
		h.ServeSuperAdmin(w, r)
		return
	}

	switch strings.ToLower(strings.TrimSpace(role)) {
	case "admin":
		h.ServeAdmin(w, r)
	case "analyst":
		h.ServeAnalyst(w, r)
	case "coordinator":
		h.ServeCoordinator(w, r)
	case "leader":
		http.Redirect(w, r, "/mhsdashboard", http.StatusSeeOther)
	case "member":
		h.ServeMember(w, r)
	default:
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}
