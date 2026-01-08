// internal/app/features/dashboard/superadmin.go
package dashboard

import (
	"context"
	"net/http"

	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/waffle/pantry/templates"
	"go.mongodb.org/mongo-driver/bson"
)

// superadminDashboardData contains data for the superadmin dashboard.
type superadminDashboardData struct {
	baseDashboardData
	WorkspacesCount int64
	UsersCount      int64
	OrgsCount       int64
}

func (h *Handler) ServeSuperAdmin(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()

	// Fetch cross-workspace stats
	workspaceCount, _ := h.DB.Collection("workspaces").CountDocuments(ctx, bson.M{})
	userCount, _ := h.DB.Collection("users").CountDocuments(ctx, bson.M{})
	orgCount, _ := h.DB.Collection("organizations").CountDocuments(ctx, bson.M{})

	base := viewdata.NewBaseVM(r, h.DB, "SuperAdmin Dashboard", "/")
	data := superadminDashboardData{
		baseDashboardData: baseDashboardData{
			BaseVM: base,
		},
		WorkspacesCount: workspaceCount,
		UsersCount:      userCount,
		OrgsCount:       orgCount,
	}

	templates.Render(w, r, "superadmin_dashboard", data)
}
