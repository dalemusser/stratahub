// internal/domain/models/memberstatus.go
package models

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Member status states, in ladder order. A member's status on an entity only
// ever moves up the ladder: a lower state reported after a higher one is
// recorded in history but never changes State. "not_started" is implicit —
// it means no document exists — and is never stored.
const (
	MemberStatusNotStarted = "not_started"
	MemberStatusOpened     = "opened"    // the member launched the linked resource in StrataHub
	MemberStatusStarted    = "started"   // reported by the external provider
	MemberStatusCompleted  = "completed" // reported by the external provider
)

// Sources identify which writer produced a status event.
const (
	MemberStatusSourceAPI    = "api"    // the Member Status API (external provider)
	MemberStatusSourceLaunch = "launch" // a resource launch inside StrataHub
)

// MemberStatusHistoryLimit caps the per-document event history. Older events
// are dropped from the front; the first-wins timestamps are kept separately
// so nothing that matters is lost when the cap is reached.
const MemberStatusHistoryLimit = 25

// memberStatusRanks orders the stored states so the store can raise a state
// with a single $max and never regress it.
var memberStatusRanks = map[string]int{
	MemberStatusOpened:    1,
	MemberStatusStarted:   2,
	MemberStatusCompleted: 3,
}

// MemberStatusRank returns the ladder position of a stored state, or 0 for an
// unknown state (including "not_started", which is never stored).
func MemberStatusRank(state string) int {
	return memberStatusRanks[state]
}

// MemberStatusStateForRank is the inverse of MemberStatusRank. Rank 0 (or any
// unknown rank) maps to "not_started".
func MemberStatusStateForRank(rank int) string {
	for state, r := range memberStatusRanks {
		if r == rank {
			return state
		}
	}
	return MemberStatusNotStarted
}

// IsValidMemberStatusState reports whether state is one that can be stored.
func IsValidMemberStatusState(state string) bool {
	return MemberStatusRank(state) > 0
}

// MemberStatusEvent is one entry in a MemberStatus document's history.
type MemberStatusEvent struct {
	State      string     `bson:"state" json:"state"`
	Source     string     `bson:"source" json:"source"`
	ReceivedAt time.Time  `bson:"received_at" json:"received_at"`                     // when StrataHub recorded it
	OccurredAt *time.Time `bson:"occurred_at,omitempty" json:"occurred_at,omitempty"` // the writer's own timestamp, if it supplied one
	RemoteIP   string     `bson:"remote_ip,omitempty" json:"-"`
}

// MemberStatus records a member's progress on one externally-tracked entity
// (a survey, for example). One document per (workspace, user, entity_key).
//
// EntityKey is the canonical key both writers resolve to: a configured item's
// id when the entity name is recognized, or "name:<folded name>" for a name
// the configuration does not (yet) know. Entity holds the display name as
// first received. See docs/member-status-api/plan.md.
type MemberStatus struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	WorkspaceID primitive.ObjectID `bson:"workspace_id" json:"workspace_id"`
	UserID      primitive.ObjectID `bson:"user_id" json:"user_id"`
	EntityKey   string             `bson:"entity_key" json:"entity_key"`
	Entity      string             `bson:"entity" json:"entity"`

	State     string `bson:"state" json:"state"`  // highest state reached; see MemberStatusRank
	StateRank int    `bson:"state_rank" json:"-"` // numeric form of State, raised with $max

	// First-wins timestamps: the moment StrataHub first recorded each state.
	OpenedAt    *time.Time `bson:"opened_at,omitempty" json:"opened_at,omitempty"`
	StartedAt   *time.Time `bson:"started_at,omitempty" json:"started_at,omitempty"`
	CompletedAt *time.Time `bson:"completed_at,omitempty" json:"completed_at,omitempty"`

	Source          string    `bson:"source" json:"source"` // writer of the most recent event
	FirstReceivedAt time.Time `bson:"first_received_at" json:"first_received_at"`
	LastReceivedAt  time.Time `bson:"last_received_at" json:"last_received_at"`

	History []MemberStatusEvent `bson:"history,omitempty" json:"history,omitempty"`

	CreatedAt time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time `bson:"updated_at" json:"updated_at"`
}

// TimestampFor returns the first-wins timestamp recorded for a state, or nil
// if that state has never been reached.
func (m *MemberStatus) TimestampFor(state string) *time.Time {
	switch state {
	case MemberStatusOpened:
		return m.OpenedAt
	case MemberStatusStarted:
		return m.StartedAt
	case MemberStatusCompleted:
		return m.CompletedAt
	}
	return nil
}
