// internal/app/features/dashboard/common.go
package dashboard

import "github.com/dalemusser/stratahub/internal/app/system/viewdata"

// baseDashboardData contains fields common to all dashboard views.
type baseDashboardData struct {
	viewdata.BaseVM
}

// dashboardWithCounts extends baseDashboardData with entity counts
// for admin and analyst dashboards.
type dashboardWithCounts struct {
	baseDashboardData
	OrganizationsCount int64
	LeadersCount       int64
	GroupsCount        int64
	MembersCount       int64
	ResourcesCount     int64
}

// coordinatorDashboardData extends dashboardWithCounts with coordinator-specific permissions.
type coordinatorDashboardData struct {
	dashboardWithCounts
	CanManageResources bool
	CanManageMaterials bool
	AssignedOrgNames   []string // Names of organizations this coordinator can manage
}
