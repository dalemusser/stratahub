// internal/domain/models/resourceurlmodes.go
package models

// Canonical URL identity modes for Resource.URLIdentityMode.
//
// The mode selects which identity query parameters are appended to a resource's
// LaunchURL when a member opens it. See docs/resource-identification/ for the
// parameter vocabulary (the permanent contract) and the mode definitions.
//
// An empty URLIdentityMode is treated as URLIdentityNone.
const (
	URLIdentityNone   = "none"   // emit no identity parameters (default)
	URLIdentityHex    = "hex"    // ws_id, org_id, group_id, user_id (de-identified)
	URLIdentityHuman  = "human"  // ws, org, group, user, login_id (human-readable; PII)
	URLIdentityBoth   = "both"   // hex + human (debugging; PII)
	URLIdentityLegacy = "legacy" // id(=login_id), org, group (pre-2026 contract; deprecated)
)

// URLIdentityModes is the full set of valid identity modes. Treat as the single
// source of truth for validation and schema enums. Empty string is also accepted
// at input time and means URLIdentityNone.
var URLIdentityModes = []string{
	URLIdentityNone,
	URLIdentityHex,
	URLIdentityHuman,
	URLIdentityBoth,
	URLIdentityLegacy,
}

// DefaultURLIdentityMode is used when no mode is set.
const DefaultURLIdentityMode = URLIdentityNone
