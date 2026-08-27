// Package viewscope resolves what data the current user may see in a
// role-scoped list ("viewer") and turns that into Mongo filters.
//
// It consolidates the group/org lookups that the MHS Dashboard, Activity, and
// the audit feature each implement separately:
//
//	superadmin, admin, analyst → the whole workspace
//	coordinator                → members of the organizations assigned to them
//	leader                     → members of the groups they lead
//	anything else              → no scope (ErrForbidden)
//
// A viewer narrows the scope with an optional organization and/or group
// selection (validated against the role's reach), then asks for a single
// Filter that is correct for the role — so a viewer cannot accidentally
// under-scope a leader by filtering on organization alone.
//
// See docs/viewers/plan.md.
package viewscope

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import (
	"context"
	"errors"
	"net/http"
	"sort"

	"github.com/dalemusser/stratahub/internal/app/store/coordinatorassign"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/normalize"
	"github.com/dalemusser/stratahub/internal/app/system/status"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

var (
	// ErrForbidden means the current user has no data scope at all (not
	// signed in, or a role — e.g. member — that viewers never serve).
	ErrForbidden = errors.New("viewscope: no data scope for this user")
	// ErrNoWorkspace means the request carries no workspace context.
	ErrNoWorkspace = errors.New("viewscope: workspace required")
	// ErrOutOfScope means the requested organization/group selection is not
	// within what the role may see (or does not exist in the workspace).
	ErrOutOfScope = errors.New("viewscope: selection is outside the user's scope")
)

// Options narrows a scope to a chosen organization and/or group. Zero values
// mean "no selection". Selections are validated against the role's reach.
type Options struct {
	OrgID   primitive.ObjectID
	GroupID primitive.ObjectID
}

// OrgOption is an organization the user may select in a viewer's filter bar.
type OrgOption struct {
	ID   primitive.ObjectID
	Name string
}

// GroupOption is a group the user may select in a viewer's filter bar.
type GroupOption struct {
	ID    primitive.ObjectID
	Name  string
	OrgID primitive.ObjectID
}

// Scope is the resolved reach of the current user, plus any narrowing
// selection. Build it with Resolve; use Filter to constrain queries.
type Scope struct {
	WorkspaceID primitive.ObjectID
	Role        string
	UserID      primitive.ObjectID

	// All is true for roles that may see the whole workspace (before any
	// selection narrows it).
	All bool
	// OrgIDs is the organization reach: a coordinator's assigned orgs, or the
	// selected organization. Nil means "not restricted by organization".
	OrgIDs []primitive.ObjectID
	// GroupIDs is the group reach: a leader's groups, or the selected group.
	// Nil means "not restricted by group".
	GroupIDs []primitive.ObjectID

	// The selections applied, if any.
	SelectedOrg   primitive.ObjectID
	SelectedGroup primitive.ObjectID

	db       *mongo.Database
	empty    bool // the role reaches nothing (coordinator without orgs, leader without groups)
	userIDs  []primitive.ObjectID
	resolved bool
}

// Resolve builds the Scope for the current request's user and workspace.
func Resolve(ctx context.Context, db *mongo.Database, r *http.Request, opts Options) (*Scope, error) {
	role, _, userID, ok := authz.UserCtx(r)
	if !ok {
		return nil, ErrForbidden
	}
	wsID := workspace.IDFromRequest(r)
	if wsID.IsZero() {
		return nil, ErrNoWorkspace
	}

	s := &Scope{WorkspaceID: wsID, Role: normalize.Role(role), UserID: userID, db: db}

	switch s.Role {
	case "superadmin", "admin", "analyst":
		s.All = true

	case "coordinator":
		orgIDs, err := coordinatorassign.New(db).OrgIDsByUser(ctx, userID)
		if err != nil {
			return nil, err
		}
		s.OrgIDs = orgIDs
		s.empty = len(orgIDs) == 0

	case "leader":
		groupIDs, err := leaderGroupIDs(ctx, db, wsID, userID)
		if err != nil {
			return nil, err
		}
		s.GroupIDs = groupIDs
		s.empty = len(groupIDs) == 0

	default:
		return nil, ErrForbidden
	}

	if err := s.applySelection(ctx, opts); err != nil {
		return nil, err
	}
	return s, nil
}

// applySelection narrows the scope to the chosen organization and/or group,
// rejecting anything the role cannot reach.
func (s *Scope) applySelection(ctx context.Context, opts Options) error {
	if !opts.OrgID.IsZero() {
		switch s.Role {
		case "leader":
			return ErrOutOfScope // leaders select groups, not organizations
		case "coordinator":
			if !contains(s.OrgIDs, opts.OrgID) {
				return ErrOutOfScope
			}
		default:
			ok, err := s.orgInWorkspace(ctx, opts.OrgID)
			if err != nil {
				return err
			}
			if !ok {
				return ErrOutOfScope
			}
		}
		s.OrgIDs = []primitive.ObjectID{opts.OrgID}
		s.SelectedOrg = opts.OrgID
		s.All = false
	}

	if !opts.GroupID.IsZero() {
		var g struct {
			OrganizationID primitive.ObjectID `bson:"organization_id"`
		}
		err := s.db.Collection("groups").FindOne(ctx, bson.M{"_id": opts.GroupID, "workspace_id": s.WorkspaceID}).Decode(&g)
		if err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return ErrOutOfScope
			}
			return err
		}
		switch s.Role {
		case "leader":
			if !contains(s.GroupIDs, opts.GroupID) {
				return ErrOutOfScope
			}
		default:
			// Coordinators: the group's org must be assigned to them; anyone
			// who selected an org: the group must belong to it.
			if s.OrgIDs != nil && !contains(s.OrgIDs, g.OrganizationID) {
				return ErrOutOfScope
			}
		}
		s.GroupIDs = []primitive.ObjectID{opts.GroupID}
		s.SelectedGroup = opts.GroupID
		s.All = false
		s.userIDs, s.resolved = nil, false
	}
	return nil
}

// Empty reports whether the role reaches nothing at all (a coordinator with no
// assigned organizations, a leader with no groups). Filter then matches
// nothing.
func (s *Scope) Empty() bool { return s.empty }

// Unrestricted reports whether queries need no scope constraint at all.
func (s *Scope) Unrestricted() bool { return s.All && s.OrgIDs == nil && s.GroupIDs == nil }

// Filter returns the Mongo constraint a viewer must AND into its query.
//
// orgField is the document field holding the organization id (may be "" if
// the viewer's documents carry none); userField holds the member's user id.
// The constraint is chosen by what the scope is restricted to:
//
//   - unrestricted → nil (no constraint)
//   - restricted to organizations (coordinator, or an org selection with no
//     group selection) → {orgField ∈ OrgIDs}, or, when orgField is "", the
//     members of those organizations by userField
//   - restricted to groups (leader, or a group selection) → {userField ∈
//     members of those groups}
//   - empty reach → a constraint that matches nothing
func (s *Scope) Filter(ctx context.Context, orgField, userField string) (bson.M, error) {
	if s.empty {
		return bson.M{userField: bson.M{"$in": []primitive.ObjectID{}}}, nil
	}
	if s.Unrestricted() {
		return nil, nil
	}
	if s.GroupIDs == nil && orgField != "" {
		return bson.M{orgField: bson.M{"$in": s.OrgIDs}}, nil
	}
	ids, err := s.UserIDs(ctx)
	if err != nil {
		return nil, err
	}
	return bson.M{userField: bson.M{"$in": ids}}, nil
}

// UserIDs returns the member user ids within the scope, resolving them once:
// members of GroupIDs when groups restrict the scope, otherwise members of
// OrgIDs. It returns nil for an unrestricted scope and an empty (non-nil)
// slice for an empty reach.
func (s *Scope) UserIDs(ctx context.Context) ([]primitive.ObjectID, error) {
	if s.resolved {
		return s.userIDs, nil
	}
	s.resolved = true
	switch {
	case s.empty:
		s.userIDs = []primitive.ObjectID{}
	case s.Unrestricted():
		s.userIDs = nil
	case s.GroupIDs != nil:
		ids, err := membersOfGroups(ctx, s.db, s.WorkspaceID, s.GroupIDs)
		if err != nil {
			return nil, err
		}
		s.userIDs = ids
	default:
		ids, err := membersOfOrgs(ctx, s.db, s.WorkspaceID, s.OrgIDs)
		if err != nil {
			return nil, err
		}
		s.userIDs = ids
	}
	return s.userIDs, nil
}

// Orgs returns the organizations the user may choose from in a filter bar,
// sorted by name: every active org in the workspace for whole-workspace
// roles, the assigned orgs for a coordinator, none for a leader.
func (s *Scope) Orgs(ctx context.Context) ([]OrgOption, error) {
	filter := bson.M{"workspace_id": s.WorkspaceID, "status": status.Active}
	switch s.Role {
	case "leader":
		return nil, nil
	case "coordinator":
		if len(s.OrgIDs) == 0 {
			return nil, nil
		}
		filter["_id"] = bson.M{"$in": s.OrgIDs}
	}
	cur, err := s.db.Collection("organizations").Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []OrgOption
	for cur.Next(ctx) {
		var o struct {
			ID   primitive.ObjectID `bson:"_id"`
			Name string             `bson:"name"`
		}
		if err := cur.Decode(&o); err != nil {
			return nil, err
		}
		out = append(out, OrgOption{ID: o.ID, Name: o.Name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, cur.Err()
}

// Groups returns the groups the user may choose from in a filter bar, sorted
// by name and limited to the current organization reach (and to the selected
// organization, if any): all active groups for whole-workspace roles, groups
// in assigned orgs for a coordinator, the leader's own groups for a leader.
func (s *Scope) Groups(ctx context.Context) ([]GroupOption, error) {
	filter := bson.M{"workspace_id": s.WorkspaceID, "status": status.Active}
	switch {
	case s.Role == "leader":
		if len(s.GroupIDs) == 0 {
			return nil, nil
		}
		// A leader's reach is their groups regardless of any selection.
		ids, err := leaderGroupIDs(ctx, s.db, s.WorkspaceID, s.UserID)
		if err != nil {
			return nil, err
		}
		if len(ids) == 0 {
			return nil, nil
		}
		filter["_id"] = bson.M{"$in": ids}
	case s.OrgIDs != nil:
		filter["organization_id"] = bson.M{"$in": s.OrgIDs}
	}
	cur, err := s.db.Collection("groups").Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []GroupOption
	for cur.Next(ctx) {
		var g struct {
			ID    primitive.ObjectID `bson:"_id"`
			Name  string             `bson:"name"`
			OrgID primitive.ObjectID `bson:"organization_id"`
		}
		if err := cur.Decode(&g); err != nil {
			return nil, err
		}
		out = append(out, GroupOption{ID: g.ID, Name: g.Name, OrgID: g.OrgID})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, cur.Err()
}

// --- lookups ---------------------------------------------------------------

// leaderGroupIDs returns the active groups in which the user holds the
// leader role within the workspace.
func leaderGroupIDs(ctx context.Context, db *mongo.Database, wsID, userID primitive.ObjectID) ([]primitive.ObjectID, error) {
	cur, err := db.Collection("group_memberships").Find(ctx, bson.M{
		"workspace_id": wsID,
		"user_id":      userID,
		"role":         "leader",
	})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var candidates []primitive.ObjectID
	for cur.Next(ctx) {
		var m struct {
			GroupID primitive.ObjectID `bson:"group_id"`
		}
		if err := cur.Decode(&m); err != nil {
			return nil, err
		}
		candidates = append(candidates, m.GroupID)
	}
	if err := cur.Err(); err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	// Keep only active groups, mirroring the dashboards.
	gcur, err := db.Collection("groups").Find(ctx, bson.M{"_id": bson.M{"$in": candidates}, "status": status.Active})
	if err != nil {
		return nil, err
	}
	defer gcur.Close(ctx)
	var ids []primitive.ObjectID
	for gcur.Next(ctx) {
		var g struct {
			ID primitive.ObjectID `bson:"_id"`
		}
		if err := gcur.Decode(&g); err != nil {
			return nil, err
		}
		ids = append(ids, g.ID)
	}
	return ids, gcur.Err()
}

// membersOfGroups returns the distinct member user ids of the given groups.
func membersOfGroups(ctx context.Context, db *mongo.Database, wsID primitive.ObjectID, groupIDs []primitive.ObjectID) ([]primitive.ObjectID, error) {
	if len(groupIDs) == 0 {
		return []primitive.ObjectID{}, nil
	}
	cur, err := db.Collection("group_memberships").Find(ctx, bson.M{
		"workspace_id": wsID,
		"group_id":     bson.M{"$in": groupIDs},
		"role":         "member",
	})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	return distinctUserIDs(ctx, cur)
}

// membersOfOrgs returns the member user ids belonging to the given
// organizations.
func membersOfOrgs(ctx context.Context, db *mongo.Database, wsID primitive.ObjectID, orgIDs []primitive.ObjectID) ([]primitive.ObjectID, error) {
	if len(orgIDs) == 0 {
		return []primitive.ObjectID{}, nil
	}
	cur, err := db.Collection("users").Find(ctx, bson.M{
		"workspace_id":    wsID,
		"organization_id": bson.M{"$in": orgIDs},
		"role":            "member",
	})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	ids := []primitive.ObjectID{}
	for cur.Next(ctx) {
		var u struct {
			ID primitive.ObjectID `bson:"_id"`
		}
		if err := cur.Decode(&u); err != nil {
			return nil, err
		}
		ids = append(ids, u.ID)
	}
	return ids, cur.Err()
}

func distinctUserIDs(ctx context.Context, cur *mongo.Cursor) ([]primitive.ObjectID, error) {
	seen := map[primitive.ObjectID]struct{}{}
	ids := []primitive.ObjectID{}
	for cur.Next(ctx) {
		var m struct {
			UserID primitive.ObjectID `bson:"user_id"`
		}
		if err := cur.Decode(&m); err != nil {
			return nil, err
		}
		if _, dup := seen[m.UserID]; dup {
			continue
		}
		seen[m.UserID] = struct{}{}
		ids = append(ids, m.UserID)
	}
	return ids, cur.Err()
}

func (s *Scope) orgInWorkspace(ctx context.Context, orgID primitive.ObjectID) (bool, error) {
	n, err := s.db.Collection("organizations").CountDocuments(ctx, bson.M{"_id": orgID, "workspace_id": s.WorkspaceID})
	return n > 0, err
}

func contains(ids []primitive.ObjectID, id primitive.ObjectID) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}
