// internal/app/features/dashboard/coordinator.go
package dashboard

import (
	"context"
	"net/http"
	"sort"

	metricsstore "github.com/dalemusser/stratahub/internal/app/store/metrics"
	organizationstore "github.com/dalemusser/stratahub/internal/app/store/organizations"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/waffle/pantry/templates"
	"go.mongodb.org/mongo-driver/bson"
)

func (h *Handler) ServeCoordinator(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()

	// TODO: Filter counts to coordinator's assigned organizations
	counts := metricsstore.FetchDashboardCounts(ctx, h.DB)

	// Fetch organization names for coordinator's assigned orgs
	orgIDs := authz.UserOrgIDs(r)
	var orgNames []string
	if len(orgIDs) > 0 {
		orgStore := organizationstore.New(h.DB)
		orgs, err := orgStore.Find(ctx, bson.M{"_id": bson.M{"$in": orgIDs}})
		if err == nil {
			for _, org := range orgs {
				orgNames = append(orgNames, org.Name)
			}
			sort.Strings(orgNames)
		}
	}

	base := viewdata.NewBaseVM(r, h.DB, "Coordinator Dashboard", "/")
	data := coordinatorDashboardData{
		dashboardWithCounts: dashboardWithCounts{
			baseDashboardData: baseDashboardData{
				BaseVM: base,
			},
			OrganizationsCount: counts.Organizations,
			LeadersCount:       counts.Leaders,
			GroupsCount:        counts.Groups,
			MembersCount:       counts.Members,
			ResourcesCount:     counts.Resources,
		},
		CanManageResources: authz.CanManageResources(r),
		CanManageMaterials: authz.CanManageMaterials(r),
		AssignedOrgNames:   orgNames,
	}

	templates.Render(w, r, "coordinator_dashboard", data)
}
