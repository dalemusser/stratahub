// internal/app/features/dashboard/analyst.go
package dashboard

import (
	"context"
	"net/http"

	metricsstore "github.com/dalemusser/stratahub/internal/app/store/metrics"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/waffle/pantry/templates"
)

func (h *Handler) ServeAnalyst(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()

	counts := metricsstore.FetchDashboardCounts(ctx, h.DB)

	base := viewdata.NewBaseVM(r, h.DB, "Analyst Dashboard", "/")
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

	templates.Render(w, r, "analyst_dashboard", data)
}
