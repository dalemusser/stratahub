// Package resourceurl builds the identity query parameters that StrataHub
// appends to a resource's launch URL when a member opens it. Which parameters
// are emitted is selected by the resource's identification mode
// (models.URLIdentity*).
//
// See docs/resource-identification/ for the parameter vocabulary (the permanent
// contract) and the mode definitions.
package resourceurl

import (
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/urlutil"
)

// IdentityContext carries the resolved values for the member opening a resource.
// Name fields are human-readable; *ID fields are 24-char hex ObjectIDs; LoginID
// is the member's login (email). Any field may be empty — empty values are
// omitted from the URL.
type IdentityContext struct {
	WorkspaceSubdomain string
	WorkspaceID        string
	OrgName            string
	OrgID              string
	GroupName          string
	GroupID            string
	UserName           string
	UserID             string
	LoginID            string
}

// Default returns the default mode for new resources.
func Default() string { return models.DefaultURLIdentityMode }

// HasPII reports whether a mode emits a high-PII field (the user's name and/or
// login). Used to drive the creation-time PII warning. Only "none" and "hex"
// are PII-free.
func HasPII(mode string) bool {
	switch mode {
	case models.URLIdentityHuman, models.URLIdentityBoth, models.URLIdentityLegacy:
		return true
	}
	return false
}

// BuildLaunchURL appends the identity parameters for mode to launchURL. An empty
// or unrecognized mode is treated as "none" (no parameters added). Empty context
// values are omitted.
func BuildLaunchURL(launchURL, mode string, ctx IdentityContext) string {
	params := paramsForMode(mode, ctx)
	if len(params) == 0 {
		return launchURL
	}
	return urlutil.AddOrSetQueryParams(launchURL, params)
}

func paramsForMode(mode string, ctx IdentityContext) map[string]string {
	switch mode {
	case models.URLIdentityHex:
		return map[string]string{
			"ws_id":    ctx.WorkspaceID,
			"org_id":   ctx.OrgID,
			"group_id": ctx.GroupID,
			"user_id":  ctx.UserID,
		}
	case models.URLIdentityHuman:
		return map[string]string{
			"ws":       ctx.WorkspaceSubdomain,
			"org":      ctx.OrgName,
			"group":    ctx.GroupName,
			"user":     ctx.UserName,
			"login_id": ctx.LoginID,
		}
	case models.URLIdentityBoth:
		return map[string]string{
			"ws":       ctx.WorkspaceSubdomain,
			"ws_id":    ctx.WorkspaceID,
			"org":      ctx.OrgName,
			"org_id":   ctx.OrgID,
			"group":    ctx.GroupName,
			"group_id": ctx.GroupID,
			"user":     ctx.UserName,
			"user_id":  ctx.UserID,
			"login_id": ctx.LoginID,
		}
	case models.URLIdentityLegacy:
		return map[string]string{
			"id":    ctx.LoginID, // pre-2026 contract: login under the param named "id"
			"org":   ctx.OrgName,
			"group": ctx.GroupName,
		}
	default: // "none", "", or unrecognized
		return nil
	}
}
