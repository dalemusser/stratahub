// internal/app/features/dashboard/admin.go
package dashboard

import (
	"context"
	"net/http"

	metricsstore "github.com/dalemusser/stratahub/internal/app/store/metrics"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/dalemusser/waffle/pantry/httpnav"
	"go.uber.org/zap"
)

func (h *Handler) ServeAdmin(w http.ResponseWriter, r *http.Request) {
	role, uname, _, signedIn := authz.UserCtx(r)

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()

	counts := metricsstore.FetchDashboardCounts(ctx, h.DB)

	data := dashboardWithCounts{
		baseDashboardData: baseDashboardData{
			Title:       "Admin Dashboard",
			IsLoggedIn:  signedIn,
			Role:        role,
			UserName:    uname,
			CurrentPath: httpnav.CurrentPath(r),
		},
		OrganizationsCount: counts.Organizations,
		LeadersCount:       counts.Leaders,
		GroupsCount:        counts.Groups,
		MembersCount:       counts.Members,
		ResourcesCount:     counts.Resources,
	}

	h.Log.Debug("admin dashboard served", zap.String("user", uname))

	templates.Render(w, r, "admin_dashboard", data)
}
