// Package reportpolicy provides authorization policies for report access.
//
// Authorization rules:
//   - Admins and Analysts can view reports for all organizations
//   - Leaders can only view reports for their own organization
//   - Other roles (member) cannot access reports
package reportpolicy

import (
	"net/http"

	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ReportScope represents the scope of data a user can access in reports.
type ReportScope struct {
	// CanView indicates whether the user can view reports at all.
	CanView bool
	// AllOrgs indicates whether the user can see data from all organizations.
	// If false, OrgID specifies the single org they can see.
	AllOrgs bool
	// OrgID is the organization ID the user is restricted to (when AllOrgs is false).
	OrgID primitive.ObjectID
}

// CanViewMembersReport determines what scope of data the current user can access
// in the members report.
//
// Authorization:
//   - Admin/Analyst: can view report data from all organizations
//   - Leader: can only view report data from their own organization
//   - Others: cannot view reports
func CanViewMembersReport(r *http.Request) ReportScope {
	role, _, _, ok := authz.UserCtx(r)
	if !ok {
		return ReportScope{CanView: false}
	}

	switch role {
	case "admin", "analyst":
		return ReportScope{CanView: true, AllOrgs: true}
	case "leader":
		orgID := authz.UserOrgID(r)
		if orgID == primitive.NilObjectID {
			return ReportScope{CanView: false}
		}
		return ReportScope{CanView: true, AllOrgs: false, OrgID: orgID}
	default:
		return ReportScope{CanView: false}
	}
}
