// internal/app/features/dashboard/analyst.go
package dashboard

import (
	"context"
	"net/http"

	metricsstore "github.com/dalemusser/stratahub/internal/app/store/metrics"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/waffle/templates"
	"github.com/dalemusser/waffle/toolkit/ui/nav"
	"go.uber.org/zap"
)

type analystData struct {
	Title      string
	IsLoggedIn bool
	Role       string
	UserName   string

	OrganizationsCount int64
	LeadersCount       int64
	GroupsCount        int64
	MembersCount       int64
	ResourcesCount     int64

	CurrentPath string
}

func (h *Handler) ServeAnalyst(w http.ResponseWriter, r *http.Request) {
	role, uname, _, signedIn := authz.UserCtx(r)

	ctx, cancel := context.WithTimeout(r.Context(), dashboardTimeout)
	defer cancel()

	counts := metricsstore.FetchDashboardCounts(ctx, h.DB)

	data := analystData{
		Title:              "Analyst Dashboard",
		IsLoggedIn:         signedIn,
		Role:               role,
		UserName:           uname,
		OrganizationsCount: counts.Organizations,
		LeadersCount:       counts.Leaders,
		GroupsCount:        counts.Groups,
		MembersCount:       counts.Members,
		ResourcesCount:     counts.Resources,
		CurrentPath:        nav.CurrentPath(r),
	}

	h.Log.Debug("analyst dashboard served", zap.String("user", uname))

	templates.Render(w, r, "analyst_dashboard", data)
}
