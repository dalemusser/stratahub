// internal/app/system/search/search.go
package webutil

import "strings"

// EmailPivotOK reports whether it’s safe & useful to pivot a paged search
// from name-based sorting to email-based sorting.
//
// We consider it safe to pivot when:
//   - The user is clearly searching by email (the query contains '@'), and
//   - The result set is constrained by status (e.g., active/disabled). For entity
//     types that live inside an organization (e.g., members/leaders), we also
//     require an org constraint to keep the indexed path selective enough.
//
// Typical usage in org-scoped lists (Members/Leaders):
//
//	pivot := webutil.EmailPivotOK(query, status, scopeOrg != nil)
//	sortField := "full_name_ci"
//	if pivot {
//	    sortField = "email"
//	}
//
// For unscoped lists (e.g., System Users across all orgs), use EmailPivotNoOrgOK.
//
//	pivot := webutil.EmailPivotNoOrgOK(query, status)
func EmailPivotOK(search, status string, hasOrg bool) bool {
	qHasAt := strings.Contains(search, "@")
	statusFixed := equalsAnyFold(status, "active", "disabled")
	return qHasAt && statusFixed && hasOrg
}

// EmailPivotNoOrgOK is a variant for global lists with no org constraint
// (e.g., System Users). Requires that the query looks like an email and the
// status filter is constrained, then pivots to sort by email.
func EmailPivotNoOrgOK(search, status string) bool {
	qHasAt := strings.Contains(search, "@")
	statusFixed := equalsAnyFold(status, "active", "disabled")
	return qHasAt && statusFixed
}

func equalsAnyFold(s string, vals ...string) bool {
	s = strings.TrimSpace(strings.ToLower(s))
	for _, v := range vals {
		if s == strings.ToLower(v) {
			return true
		}
	}
	return false
}
