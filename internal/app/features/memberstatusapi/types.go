// internal/app/features/memberstatusapi/types.go
package memberstatusapi

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import "time"

// statusRequest is the JSON body of POST /api/member-status. The same shape
// is decoded for /ping, which uses only Key.
type statusRequest struct {
	Key        string `json:"key"`         // the workspace's shared key (required)
	UserID     string `json:"user_id"`     // member's 24-char hex ObjectID
	Entity     string `json:"entity"`      // entity name, e.g. "MHS Engagement"
	State      string `json:"state"`       // "started" | "completed"
	OccurredAt string `json:"occurred_at"` // optional RFC 3339 timestamp from the provider
}

// statusResponse is the JSON body returned for an accepted status event.
// EventID identifies the log entry for this request (Survey Events viewer).
type statusResponse struct {
	OK          bool       `json:"ok"`
	EventID     string     `json:"event_id,omitempty"`
	UserID      string     `json:"user_id"`
	Entity      string     `json:"entity"`
	EntityKey   string     `json:"entity_key"`
	KnownEntity bool       `json:"known_entity"`
	State       string     `json:"state"`
	OpenedAt    *time.Time `json:"opened_at,omitempty"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// pingResponse is the JSON body returned by POST /api/member-status/ping.
type pingResponse struct {
	OK        bool     `json:"ok"`
	Workspace string   `json:"workspace"`
	Entities  []string `json:"entities"`
}

// errorResponse is the JSON body returned for every non-2xx result. EventID
// is present when the request was authenticated (and therefore logged).
type errorResponse struct {
	OK      bool   `json:"ok"`
	Error   string `json:"error"`
	Message string `json:"message"`
	EventID string `json:"event_id,omitempty"`
}

// Error codes (the "error" field of errorResponse). Stable strings a
// provider can branch on; see docs/member-status-api/.
const (
	errBadJSON           = "bad_json"
	errWorkspaceRequired = "workspace_required"
	errNotConfigured     = "not_configured"
	errUnauthorized      = "unauthorized"
	errRateLimited       = "rate_limited"
	errInvalidUserID     = "invalid_user_id"
	errUnknownUser       = "unknown_user"
	errInvalidEntity     = "invalid_entity"
	errInvalidState      = "invalid_state"
	errInvalidOccurredAt = "invalid_occurred_at"
	errServer            = "server_error"
)
