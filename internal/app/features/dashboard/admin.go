// internal/app/features/dashboard/admin.go
package dashboard

import (
	"context"
	"net/http"

	metricsstore "github.com/dalemusser/stratahub/internal/app/store/metrics"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/waffle/pantry/templates"
	"go.uber.org/zap"
)

func (h *Handler) ServeAdmin(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()

	counts := metricsstore.FetchDashboardCounts(ctx, h.DB)

	base := viewdata.NewBaseVM(r, h.DB, "Admin Dashboard", "/")
	data := dashboardWithCounts{
		baseDashboardData: baseDashboardData{
			BaseVM: base,
		},
		OrganizationsCount: counts.Organizations,
		LeadersCount:       counts.Leaders,
		GroupsCount:        counts.Groups,
		MembersCount:       counts.Members,
		ResourcesCount:     counts.Resources,
	}

	h.Log.Debug("admin dashboard served", zap.String("user", base.UserName))

	templates.Render(w, r, "admin_dashboard", data)
}
