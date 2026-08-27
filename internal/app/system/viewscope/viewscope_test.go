package viewscope_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/app/system/viewscope"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// world is a small workspace: two orgs, three active groups (+ one inactive),
// members spread across them, a leader of A1 and A2, a coordinator for org A.
type world struct {
	db                    *mongo.Database
	ws                    primitive.ObjectID
	orgA, orgB            primitive.ObjectID
	a1, a2, a3, b1        primitive.ObjectID // a3 is inactive
	a1m1, a1m2, a2m1      primitive.ObjectID // a1m2 is in A1 and A2
	b1m1, aLoose          primitive.ObjectID // aLoose: org A member in no group
	leader, coord, admin  primitive.ObjectID
	analyst, member, none primitive.ObjectID // none: leader with no groups
}

func newWorld(t *testing.T) *world {
	t.Helper()
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()
	now := time.Now().UTC()
	oid := primitive.NewObjectID

	w := &world{db: db, ws: oid(), orgA: oid(), orgB: oid(), a1: oid(), a2: oid(), a3: oid(), b1: oid(),
		a1m1: oid(), a1m2: oid(), a2m1: oid(), b1m1: oid(), aLoose: oid(),
		leader: oid(), coord: oid(), admin: oid(), analyst: oid(), member: oid(), none: oid()}

	must := func(_ interface{}, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(db.Collection("organizations").InsertMany(ctx, []interface{}{
		bson.M{"_id": w.orgA, "workspace_id": w.ws, "name": "Alpha School", "name_ci": "alpha school", "status": "active", "created_at": now},
		bson.M{"_id": w.orgB, "workspace_id": w.ws, "name": "Beta School", "name_ci": "beta school", "status": "active", "created_at": now},
	}))
	must(db.Collection("groups").InsertMany(ctx, []interface{}{
		bson.M{"_id": w.a1, "workspace_id": w.ws, "organization_id": w.orgA, "name": "A1", "name_ci": "a1", "status": "active", "created_at": now},
		bson.M{"_id": w.a2, "workspace_id": w.ws, "organization_id": w.orgA, "name": "A2", "name_ci": "a2", "status": "active", "created_at": now},
		bson.M{"_id": w.a3, "workspace_id": w.ws, "organization_id": w.orgA, "name": "A3 old", "name_ci": "a3 old", "status": "disabled", "created_at": now},
		bson.M{"_id": w.b1, "workspace_id": w.ws, "organization_id": w.orgB, "name": "B1", "name_ci": "b1", "status": "active", "created_at": now},
	}))
	user := func(id, org primitive.ObjectID, role, name string) bson.M {
		login := name + "@example.org"
		return bson.M{"_id": id, "workspace_id": w.ws, "organization_id": org, "role": role, "status": "active",
			"full_name": name, "full_name_ci": name, "login_id": login, "login_id_ci": login, "auth_method": "trust", "created_at": now, "updated_at": now}
	}
	must(db.Collection("users").InsertMany(ctx, []interface{}{
		user(w.a1m1, w.orgA, "member", "a1m1"), user(w.a1m2, w.orgA, "member", "a1m2"), user(w.a2m1, w.orgA, "member", "a2m1"),
		user(w.b1m1, w.orgB, "member", "b1m1"), user(w.aLoose, w.orgA, "member", "aloose"),
		user(w.leader, w.orgA, "leader", "leader"), user(w.none, w.orgA, "leader", "none"),
		user(w.coord, w.orgA, "coordinator", "coord"), user(w.admin, w.orgA, "admin", "admin"),
		user(w.analyst, w.orgA, "analyst", "analyst"), user(w.member, w.orgA, "member", "member"),
	}))
	membership := func(group, org, uid primitive.ObjectID, role string) bson.M {
		return bson.M{"_id": oid(), "workspace_id": w.ws, "group_id": group, "org_id": org, "user_id": uid, "role": role, "created_at": now}
	}
	must(db.Collection("group_memberships").InsertMany(ctx, []interface{}{
		membership(w.a1, w.orgA, w.a1m1, "member"), membership(w.a1, w.orgA, w.a1m2, "member"),
		membership(w.a2, w.orgA, w.a1m2, "member"), membership(w.a2, w.orgA, w.a2m1, "member"),
		membership(w.b1, w.orgB, w.b1m1, "member"),
		membership(w.a1, w.orgA, w.leader, "leader"), membership(w.a2, w.orgA, w.leader, "leader"),
		membership(w.a3, w.orgA, w.leader, "leader"), // inactive group: must not count
	}))
	must(db.Collection("coordinator_assignments").InsertOne(ctx, bson.M{"_id": oid(), "user_id": w.coord, "organization_id": w.orgA, "created_at": now}))
	return w
}

func (w *world) req(userID primitive.ObjectID, role string) *http.Request {
	r := httptest.NewRequest("GET", "/views/x", nil)
	r = workspace.WithTestWorkspace(r, w.ws, "mhs", "MHS")
	if role != "" {
		r = auth.WithTestUser(r, &auth.SessionUser{ID: userID.Hex(), Name: role, LoginID: role + "@example.org", Role: role})
	}
	return r
}

func (w *world) resolve(t *testing.T, userID primitive.ObjectID, role string, opts viewscope.Options) *viewscope.Scope {
	t.Helper()
	ctx, cancel := testutil.TestContext()
	defer cancel()
	s, err := viewscope.Resolve(ctx, w.db, w.req(userID, role), opts)
	if err != nil {
		t.Fatalf("Resolve(%s): %v", role, err)
	}
	return s
}

func hexSet(ids []primitive.ObjectID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, id.Hex())
	}
	sort.Strings(out)
	return out
}

func sameSet(t *testing.T, label string, got []primitive.ObjectID, want ...primitive.ObjectID) {
	t.Helper()
	g, w := hexSet(got), hexSet(want)
	if len(g) != len(w) {
		t.Errorf("%s: got %d ids %v, want %d %v", label, len(g), g, len(w), w)
		return
	}
	for i := range g {
		if g[i] != w[i] {
			t.Errorf("%s: got %v, want %v", label, g, w)
			return
		}
	}
}

// userIn extracts the $in list from a {field: {$in: [...]}} filter.
func userIn(t *testing.T, f bson.M, field string) []primitive.ObjectID {
	t.Helper()
	m, ok := f[field].(bson.M)
	if !ok {
		t.Fatalf("filter %v has no %q $in clause", f, field)
	}
	ids, ok := m["$in"].([]primitive.ObjectID)
	if !ok {
		t.Fatalf("filter %v: %q $in is %T", f, field, m["$in"])
	}
	return ids
}

func TestResolve_WholeWorkspaceRoles(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()

	for _, tc := range []struct {
		id   primitive.ObjectID
		role string
	}{{w.admin, "admin"}, {w.analyst, "analyst"}, {primitive.NewObjectID(), "superadmin"}} {
		s := w.resolve(t, tc.id, tc.role, viewscope.Options{})
		if !s.All || !s.Unrestricted() || s.Empty() {
			t.Errorf("%s: All=%v Unrestricted=%v Empty=%v", tc.role, s.All, s.Unrestricted(), s.Empty())
		}
		f, err := s.Filter(ctx, "org_id", "user_id")
		if err != nil || f != nil {
			t.Errorf("%s: unrestricted Filter should be nil, got %v %v", tc.role, f, err)
		}
		ids, _ := s.UserIDs(ctx)
		if ids != nil {
			t.Errorf("%s: unrestricted UserIDs should be nil", tc.role)
		}
		orgs, _ := s.Orgs(ctx)
		if len(orgs) != 2 || orgs[0].Name != "Alpha School" || orgs[1].Name != "Beta School" {
			t.Errorf("%s: orgs %v", tc.role, orgs)
		}
		groups, _ := s.Groups(ctx)
		if len(groups) != 3 || groups[0].Name != "A1" || groups[2].Name != "B1" {
			t.Errorf("%s: groups should be the 3 active ones, got %v", tc.role, groups)
		}
	}
}

func TestResolve_AdminSelections(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()

	// Organization selection: org filter when the documents carry one …
	s := w.resolve(t, w.admin, "admin", viewscope.Options{OrgID: w.orgA})
	if s.All || s.Unrestricted() || s.SelectedOrg != w.orgA {
		t.Errorf("after org selection: All=%v Unrestricted=%v selected=%s", s.All, s.Unrestricted(), s.SelectedOrg.Hex())
	}
	f, _ := s.Filter(ctx, "org_id", "user_id")
	sameSet(t, "org filter", userIn(t, f, "org_id"), w.orgA)
	// … and the org's members when they don't.
	f, _ = s.Filter(ctx, "", "user_id")
	sameSet(t, "org members", userIn(t, f, "user_id"), w.a1m1, w.a1m2, w.a2m1, w.aLoose, w.member)
	groups, _ := s.Groups(ctx)
	if len(groups) != 2 || groups[0].Name != "A1" || groups[1].Name != "A2" {
		t.Errorf("groups within selected org: %v", groups)
	}

	// Group selection narrows to that group's members regardless of orgField.
	s = w.resolve(t, w.admin, "admin", viewscope.Options{GroupID: w.a1})
	f, _ = s.Filter(ctx, "org_id", "user_id")
	sameSet(t, "group members", userIn(t, f, "user_id"), w.a1m1, w.a1m2)
	if s.SelectedGroup != w.a1 {
		t.Errorf("SelectedGroup: %s", s.SelectedGroup.Hex())
	}

	// Org + group must agree.
	if _, err := viewscope.Resolve(ctx, w.db, w.req(w.admin, "admin"), viewscope.Options{OrgID: w.orgA, GroupID: w.b1}); !errors.Is(err, viewscope.ErrOutOfScope) {
		t.Errorf("group outside selected org: got %v, want ErrOutOfScope", err)
	}
	// Unknown / other-workspace ids are out of scope.
	if _, err := viewscope.Resolve(ctx, w.db, w.req(w.admin, "admin"), viewscope.Options{OrgID: primitive.NewObjectID()}); !errors.Is(err, viewscope.ErrOutOfScope) {
		t.Errorf("unknown org: got %v, want ErrOutOfScope", err)
	}
	if _, err := viewscope.Resolve(ctx, w.db, w.req(w.admin, "admin"), viewscope.Options{GroupID: primitive.NewObjectID()}); !errors.Is(err, viewscope.ErrOutOfScope) {
		t.Errorf("unknown group: got %v, want ErrOutOfScope", err)
	}
}

func TestResolve_Coordinator(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()

	s := w.resolve(t, w.coord, "coordinator", viewscope.Options{})
	if s.All || s.Unrestricted() || s.Empty() {
		t.Errorf("coordinator: All=%v Unrestricted=%v Empty=%v", s.All, s.Unrestricted(), s.Empty())
	}
	sameSet(t, "OrgIDs", s.OrgIDs, w.orgA)
	f, _ := s.Filter(ctx, "org_id", "user_id")
	sameSet(t, "org filter", userIn(t, f, "org_id"), w.orgA)
	f, _ = s.Filter(ctx, "", "user_id")
	sameSet(t, "org members", userIn(t, f, "user_id"), w.a1m1, w.a1m2, w.a2m1, w.aLoose, w.member)
	orgs, _ := s.Orgs(ctx)
	if len(orgs) != 1 || orgs[0].ID != w.orgA {
		t.Errorf("orgs: %v", orgs)
	}
	groups, _ := s.Groups(ctx)
	if len(groups) != 2 || groups[0].ID != w.a1 || groups[1].ID != w.a2 {
		t.Errorf("groups: %v", groups)
	}

	// Selecting within reach works; outside reach is rejected.
	s = w.resolve(t, w.coord, "coordinator", viewscope.Options{GroupID: w.a2})
	f, _ = s.Filter(ctx, "org_id", "user_id")
	sameSet(t, "A2 members", userIn(t, f, "user_id"), w.a1m2, w.a2m1)
	for name, opts := range map[string]viewscope.Options{
		"other org":   {OrgID: w.orgB},
		"other group": {GroupID: w.b1},
	} {
		if _, err := viewscope.Resolve(ctx, w.db, w.req(w.coord, "coordinator"), opts); !errors.Is(err, viewscope.ErrOutOfScope) {
			t.Errorf("%s: got %v, want ErrOutOfScope", name, err)
		}
	}

	// A coordinator with no assignments reaches nothing.
	loner := primitive.NewObjectID()
	s = w.resolve(t, loner, "coordinator", viewscope.Options{})
	if !s.Empty() {
		t.Error("coordinator without orgs should be Empty")
	}
	f, _ = s.Filter(ctx, "org_id", "user_id")
	if ids := userIn(t, f, "user_id"); len(ids) != 0 {
		t.Errorf("empty scope filter should match nothing, got %v", ids)
	}
	if orgs, _ := s.Orgs(ctx); orgs != nil {
		t.Errorf("empty scope orgs: %v", orgs)
	}
}

func TestResolve_Leader(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()

	s := w.resolve(t, w.leader, "leader", viewscope.Options{})
	if s.All || s.Empty() {
		t.Errorf("leader: All=%v Empty=%v", s.All, s.Empty())
	}
	sameSet(t, "GroupIDs (active only)", s.GroupIDs, w.a1, w.a2)

	// A leader is never scoped by organization, even when an org field exists:
	// the filter must be on the members of their groups, de-duplicated.
	f, _ := s.Filter(ctx, "org_id", "user_id")
	if _, hasOrg := f["org_id"]; hasOrg {
		t.Errorf("leader filter must not be org-based: %v", f)
	}
	sameSet(t, "group members", userIn(t, f, "user_id"), w.a1m1, w.a1m2, w.a2m1)
	if orgs, _ := s.Orgs(ctx); orgs != nil {
		t.Errorf("leader should get no org options, got %v", orgs)
	}
	groups, _ := s.Groups(ctx)
	if len(groups) != 2 || groups[0].ID != w.a1 || groups[1].ID != w.a2 {
		t.Errorf("leader groups: %v", groups)
	}

	// Selecting one of their groups narrows; anything else is rejected.
	s = w.resolve(t, w.leader, "leader", viewscope.Options{GroupID: w.a1})
	f, _ = s.Filter(ctx, "org_id", "user_id")
	sameSet(t, "A1 members", userIn(t, f, "user_id"), w.a1m1, w.a1m2)
	for name, opts := range map[string]viewscope.Options{
		"group they don't lead": {GroupID: w.b1},
		"inactive group":        {GroupID: w.a3},
		"any org":               {OrgID: w.orgA},
	} {
		if _, err := viewscope.Resolve(ctx, w.db, w.req(w.leader, "leader"), opts); !errors.Is(err, viewscope.ErrOutOfScope) {
			t.Errorf("%s: got %v, want ErrOutOfScope", name, err)
		}
	}

	// A leader with no groups reaches nothing.
	s = w.resolve(t, w.none, "leader", viewscope.Options{})
	if !s.Empty() {
		t.Error("leader without groups should be Empty")
	}
	ids, _ := s.UserIDs(ctx)
	if ids == nil || len(ids) != 0 {
		t.Errorf("empty scope UserIDs should be an empty slice, got %v", ids)
	}
}

func TestResolve_Denied(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()

	if _, err := viewscope.Resolve(ctx, w.db, w.req(w.member, "member"), viewscope.Options{}); !errors.Is(err, viewscope.ErrForbidden) {
		t.Errorf("member: got %v, want ErrForbidden", err)
	}
	if _, err := viewscope.Resolve(ctx, w.db, w.req(primitive.NilObjectID, ""), viewscope.Options{}); !errors.Is(err, viewscope.ErrForbidden) {
		t.Errorf("anonymous: got %v, want ErrForbidden", err)
	}
	r := httptest.NewRequest("GET", "/views/x", nil)
	r = workspace.WithTestApex(r)
	r = auth.WithTestUser(r, &auth.SessionUser{ID: w.admin.Hex(), Role: "admin"})
	if _, err := viewscope.Resolve(ctx, w.db, r, viewscope.Options{}); !errors.Is(err, viewscope.ErrNoWorkspace) {
		t.Errorf("apex: got %v, want ErrNoWorkspace", err)
	}
}

func TestUserIDs_ResolvedOnce(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	s := w.resolve(t, w.leader, "leader", viewscope.Options{})
	first, _ := s.UserIDs(ctx)
	// Adding a member afterwards must not change the cached result.
	_, _ = w.db.Collection("group_memberships").InsertOne(ctx, bson.M{"_id": primitive.NewObjectID(), "workspace_id": w.ws, "group_id": w.a1, "org_id": w.orgA, "user_id": primitive.NewObjectID(), "role": "member"})
	second, _ := s.UserIDs(ctx)
	if len(first) != len(second) {
		t.Errorf("UserIDs should be cached per scope: %d then %d", len(first), len(second))
	}
}
