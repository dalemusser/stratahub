// internal/domain/models/memberstatuslog.go
package models

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// MemberStatusLogEntry is one received member-status event, exactly as it
// arrived and what StrataHub made of it — the record behind the Survey
// Events viewer, so the provider's developer (and staff) can confirm an
// event arrived, was stored, and carried the right content.
//
// One entry is appended per authenticated API request, accepted OR rejected
// (a wrong or missing key is not logged here — it stays a server-log
// warning), and per launch-hook "opened" write. The shared key is never
// stored. Entries are kept for MemberStatusLogRetention, then expire.
// See docs/member-status-api/plan.md §8.
type MemberStatusLogEntry struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"event_id"` // doubles as the event_id returned to the provider
	WorkspaceID primitive.ObjectID `bson:"workspace_id" json:"-"`
	ReceivedAt  time.Time          `bson:"received_at" json:"received_at"` // StrataHub receipt time (UTC)
	Source      string             `bson:"source" json:"source"`           // MemberStatusSourceAPI | MemberStatusSourceLaunch
	RemoteIP    string             `bson:"remote_ip,omitempty" json:"remote_ip,omitempty"`

	Request  MemberStatusLogRequest  `bson:"request" json:"request"`
	Resolved MemberStatusLogResolved `bson:"resolved" json:"resolved"`
	Outcome  MemberStatusLogOutcome  `bson:"outcome" json:"outcome"`
}

// MemberStatusLogRequest is the event as sent, clipped to sane lengths.
// Strings are kept raw (even a malformed user_id) so a rejected request can
// be diagnosed; StateNorm is the normalized state used for filtering.
type MemberStatusLogRequest struct {
	UserID     string `bson:"user_id,omitempty" json:"user_id,omitempty"`
	Entity     string `bson:"entity,omitempty" json:"entity,omitempty"`
	State      string `bson:"state,omitempty" json:"state,omitempty"`
	StateNorm  string `bson:"state_norm,omitempty" json:"-"`
	OccurredAt string `bson:"occurred_at,omitempty" json:"occurred_at,omitempty"`
	ResourceID string `bson:"resource_id,omitempty" json:"resource_id,omitempty"` // launch source only
}

// MemberStatusLogResolved is what StrataHub made of the request; zero values
// when the request was rejected before that step.
type MemberStatusLogResolved struct {
	UserID         *primitive.ObjectID `bson:"user_id,omitempty" json:"user_id,omitempty"`
	OrganizationID *primitive.ObjectID `bson:"organization_id,omitempty" json:"organization_id,omitempty"`
	EntityKey      string              `bson:"entity_key,omitempty" json:"entity_key,omitempty"`
	EntityTitle    string              `bson:"entity_title,omitempty" json:"entity_title,omitempty"`
	KnownEntity    bool                `bson:"known_entity" json:"known_entity"`
	StateApplied   string              `bson:"state_applied,omitempty" json:"state_applied,omitempty"`     // normalized state written
	ResultingState string              `bson:"resulting_state,omitempty" json:"resulting_state,omitempty"` // the member's state after the write
}

// MemberStatusLogOutcome is the response the caller received.
type MemberStatusLogOutcome struct {
	HTTPStatus int    `bson:"http_status" json:"http_status"`
	Error      string `bson:"error,omitempty" json:"error,omitempty"` // "" when accepted; else the API error code
	Message    string `bson:"message,omitempty" json:"message,omitempty"`
}

// Accepted reports whether the event was recorded.
func (e *MemberStatusLogEntry) Accepted() bool { return e.Outcome.Error == "" }

// MemberStatusLogRetention is how long log entries are kept (TTL index).
const MemberStatusLogRetention = 400 * 24 * time.Hour

// Clip limits for stored request strings.
const (
	MemberStatusLogMaxField = 120
)

// ClipForLog shortens a request string to MemberStatusLogMaxField runes so a
// hostile or malformed payload cannot bloat the log.
func ClipForLog(s string) string {
	r := []rune(s)
	if len(r) <= MemberStatusLogMaxField {
		return s
	}
	return string(r[:MemberStatusLogMaxField]) + "…"
}
