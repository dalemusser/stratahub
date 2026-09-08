package views

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/dalemusser/stratahub/internal/app/features/viewers"
	"github.com/dalemusser/stratahub/internal/app/store/audit"
	"github.com/dalemusser/stratahub/internal/app/system/viewscope"
	"github.com/dalemusser/waffle/pantry/text"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// AuditLogSlug is the viewer's URL segment: /views/audit-log.
const AuditLogSlug = "audit-log"

const auditTimeLayout = "Jan 2, 2006 3:04:05 PM MST"

// AuditLog lists security and administration events (logins, password
// changes, user/group/org/resource changes, …) for admins and coordinators —
// the second viewer on the framework, replacing the former /audit feature.
type AuditLog struct {
	db    *mongo.Database
	store *audit.Store
}

// NewAuditLog constructs the viewer.
func NewAuditLog(db *mongo.Database) *AuditLog {
	return &AuditLog{db: db, store: audit.New(db)}
}

func (v *AuditLog) Slug() string  { return AuditLogSlug }
func (v *AuditLog) Title() string { return "Audit Log" }
func (v *AuditLog) Description() string {
	return "Security and administration events: sign-ins and sign-in failures, password and verification activity, and changes to users, groups, organizations, resources, and materials."
}
func (v *AuditLog) Roles() []string { return []string{"admin", "coordinator"} }

// Filter keys.
const (
	afWhen     = "when"
	afCategory = "category"
	afEvent    = "event"
	afResult   = "result"
	afPerson   = "person"
	afIP       = "ip"
	afEventID  = "id"
)

// auditEventTypes lists every known event type by category, in display order.
var auditEventTypes = map[string][]string{
	audit.CategoryAuth: {
		audit.EventLoginSuccess, audit.EventLoginFailedUserNotFound, audit.EventLoginFailedWrongPassword,
		audit.EventLoginFailedUserDisabled, audit.EventLoginFailedAuthMethodDisabled, audit.EventLoginFailedRateLimit,
		audit.EventLogout, audit.EventPasswordChanged, audit.EventVerificationCodeSent,
		audit.EventVerificationCodeResent, audit.EventVerificationCodeFailed, audit.EventMagicLinkUsed,
	},
	audit.CategoryAdmin: {
		audit.EventUserCreated, audit.EventUserUpdated, audit.EventUserDisabled, audit.EventUserEnabled, audit.EventUserDeleted,
		audit.EventGroupCreated, audit.EventGroupUpdated, audit.EventGroupDeleted,
		audit.EventMemberAddedToGroup, audit.EventMemberRemovedFromGroup,
		audit.EventOrgCreated, audit.EventOrgUpdated, audit.EventOrgDeleted,
		audit.EventResourceCreated, audit.EventResourceUpdated, audit.EventResourceDeleted,
		audit.EventMaterialCreated, audit.EventMaterialUpdated, audit.EventMaterialDeleted,
		audit.EventResourceAssignedToGroup, audit.EventResourceAssignmentUpdated, audit.EventResourceUnassignedFromGroup,
		audit.EventCoordinatorAssignedToOrg, audit.EventCoordinatorUnassignedFromOrg,
		audit.EventMaterialAssigned, audit.EventMaterialAssignmentUpdated, audit.EventMaterialUnassigned,
		audit.EventGroupAppEnabled, audit.EventGroupAppDisabled,
		audit.EventMemberStatusKeyChanged, audit.EventMemberStatusOriginsChanged,
	},
}

var auditCategoryLabels = map[string]string{
	audit.CategoryAuth:     "Authentication",
	audit.CategoryAdmin:    "Administration",
	audit.CategorySecurity: "Security",
}

// humanize turns "login_failed_wrong_password" into "Login failed wrong password".
func humanize(eventType string) string {
	s := strings.ReplaceAll(eventType, "_", " ")
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func (v *AuditLog) Filters() []viewers.FilterSpec {
	var events []viewers.Option
	for _, cat := range []string{audit.CategoryAuth, audit.CategoryAdmin} {
		for _, et := range auditEventTypes[cat] {
			events = append(events, viewers.Option{Value: et, Label: humanize(et) + " (" + cat + ")"})
		}
	}
	return []viewers.FilterSpec{
		{Key: afWhen, Label: "When", Type: viewers.FilterDateRange},
		{Key: afCategory, Label: "Category", Type: viewers.FilterSelect, Options: []viewers.Option{
			{Value: audit.CategoryAuth, Label: auditCategoryLabels[audit.CategoryAuth]},
			{Value: audit.CategoryAdmin, Label: auditCategoryLabels[audit.CategoryAdmin]},
		}},
		{Key: afEvent, Label: "Event", Type: viewers.FilterSelect, Options: events},
		{Key: afResult, Label: "Result", Type: viewers.FilterSelect, Options: []viewers.Option{
			{Value: "success", Label: "Succeeded"},
			{Value: "failure", Label: "Failed"},
		}},
		{Key: afPerson, Label: "Person", Type: viewers.FilterText, Placeholder: "name or 24-char id"},
		{Key: afIP, Label: "IP address", Type: viewers.FilterText, Placeholder: "exact"},
		{Key: afEventID, Label: "Event id", Type: viewers.FilterID, Placeholder: "24-char event id"},
	}
}

func (v *AuditLog) Columns() []viewers.ColumnSpec {
	return []viewers.ColumnSpec{
		{Key: "time", Label: "Time", Class: "whitespace-nowrap"},
		{Key: "category", Label: "Category"},
		{Key: "event", Label: "Event"},
		{Key: "actor", Label: "Actor"},
		{Key: "target", Label: "Target"},
		{Key: "org", Label: "Organization"},
		{Key: "result", Label: "Result"},
		{Key: "ip", Label: "IP", Mono: true},
	}
}

// --- query -------------------------------------------------------------------

func (v *AuditLog) listQuery(ctx context.Context, scope *viewscope.Scope, f viewers.Filters) (*audit.ListQuery, error) {
	sf, err := scope.Filter(ctx, "organization_id", "user_id")
	if err != nil {
		return nil, err
	}
	q := &audit.ListQuery{
		WorkspaceID: scope.WorkspaceID,
		Scope:       sf,
		Category:    f.Get(afCategory),
		EventType:   f.Get(afEvent),
		IP:          strings.TrimSpace(f.Get(afIP)),
	}
	if from, to, ok := f.DateRange(afWhen, time.Now().UTC()); ok {
		q.From, q.To = from, to
	}
	switch f.Get(afResult) {
	case "success":
		t := true
		q.Success = &t
	case "failure":
		fl := false
		q.Success = &fl
	}
	if id, ok := f.ID(afEventID); ok {
		q.EventID = &id
	} else if f.Has(afEventID) {
		return nil, nil
	}
	if s := strings.TrimSpace(f.Get(afPerson)); s != "" {
		if id, err := primitive.ObjectIDFromHex(s); err == nil {
			q.PersonIDs = []primitive.ObjectID{id}
		} else {
			ids, err := v.searchPeople(ctx, scope.WorkspaceID, s)
			if err != nil {
				return nil, err
			}
			if len(ids) == 0 {
				return nil, nil
			}
			q.PersonIDs = ids
		}
	}
	return q, nil
}

// searchPeople finds users of any role whose folded full name starts with the
// folded query. Actors are staff; targets are usually members.
func (v *AuditLog) searchPeople(ctx context.Context, wsID primitive.ObjectID, name string) ([]primitive.ObjectID, error) {
	cur, err := v.db.Collection("users").Find(ctx, bson.M{
		"workspace_id": wsID,
		"full_name_ci": bson.M{"$regex": "^" + regexp.QuoteMeta(text.Fold(name))},
	}, options.Find().SetProjection(bson.M{"_id": 1}).SetLimit(nameSearchLimit))
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

func (v *AuditLog) Query(ctx context.Context, scope *viewscope.Scope, f viewers.Filters, after *viewers.Cursor, limit int) (viewers.Page, error) {
	q, err := v.listQuery(ctx, scope, f)
	if err != nil || q == nil {
		return viewers.Page{}, err
	}
	if after != nil {
		at, err := viewers.ParseTimeKey(after.Key)
		if err != nil {
			return viewers.Page{}, err
		}
		q.After = &audit.Position{Timestamp: at, ID: after.ID}
	}
	q.Limit = limit

	events, more, err := v.store.List(ctx, *q)
	if err != nil {
		return viewers.Page{}, err
	}
	rc := v.lookups(ctx, events)
	page := viewers.Page{}
	for _, e := range events {
		page.Rows = append(page.Rows, v.row(e, rc))
	}
	if more && len(events) > 0 {
		last := events[len(events)-1]
		page.Next = &viewers.Cursor{Key: viewers.TimeKey(last.Timestamp), ID: last.ID}
	}
	return page, nil
}

// auditContext is the per-page lookup of names and organization time zones.
type auditContext struct {
	names    map[primitive.ObjectID]string
	orgNames map[primitive.ObjectID]string
	locs     map[primitive.ObjectID]*time.Location
}

func (v *AuditLog) lookups(ctx context.Context, events []audit.Event) auditContext {
	rc := auditContext{names: map[primitive.ObjectID]string{}, orgNames: map[primitive.ObjectID]string{}, locs: map[primitive.ObjectID]*time.Location{}}
	userSet, orgSet := map[primitive.ObjectID]bool{}, map[primitive.ObjectID]bool{}
	for _, e := range events {
		if e.ActorID != nil {
			userSet[*e.ActorID] = true
		}
		if e.UserID != nil {
			userSet[*e.UserID] = true
		}
		if e.OrganizationID != nil {
			orgSet[*e.OrganizationID] = true
		}
	}
	if len(userSet) > 0 {
		ids := make([]primitive.ObjectID, 0, len(userSet))
		for id := range userSet {
			ids = append(ids, id)
		}
		if cur, err := v.db.Collection("users").Find(ctx, bson.M{"_id": bson.M{"$in": ids}}, options.Find().SetProjection(bson.M{"full_name": 1})); err == nil {
			for cur.Next(ctx) {
				var u struct {
					ID       primitive.ObjectID `bson:"_id"`
					FullName string             `bson:"full_name"`
				}
				if cur.Decode(&u) == nil {
					rc.names[u.ID] = u.FullName
				}
			}
			cur.Close(ctx)
		}
	}
	if len(orgSet) > 0 {
		ids := make([]primitive.ObjectID, 0, len(orgSet))
		for id := range orgSet {
			ids = append(ids, id)
		}
		if cur, err := v.db.Collection("organizations").Find(ctx, bson.M{"_id": bson.M{"$in": ids}}, options.Find().SetProjection(bson.M{"name": 1, "time_zone": 1})); err == nil {
			for cur.Next(ctx) {
				var o struct {
					ID       primitive.ObjectID `bson:"_id"`
					Name     string             `bson:"name"`
					TimeZone string             `bson:"time_zone"`
				}
				if cur.Decode(&o) == nil {
					rc.orgNames[o.ID] = o.Name
					if o.TimeZone != "" {
						if loc, err := time.LoadLocation(o.TimeZone); err == nil {
							rc.locs[o.ID] = loc
						}
					}
				}
			}
			cur.Close(ctx)
		}
	}
	return rc
}

func (rc auditContext) location(e audit.Event) *time.Location {
	if e.OrganizationID != nil {
		if loc, ok := rc.locs[*e.OrganizationID]; ok {
			return loc
		}
	}
	return time.UTC
}

// actorAndTarget mirrors the retired feature's rules: admin events name the
// actor and the affected user; auth events are the user's own action, so the
// user is the actor and there is no target. Failed logins for unknown users
// show the attempted login id, muted. Deleted users show nothing rather than
// a raw id.
func (rc auditContext) actorAndTarget(e audit.Event) (actor viewers.Cell, target viewers.Cell) {
	switch {
	case e.ActorID != nil:
		actor = viewers.Cell{Text: rc.names[*e.ActorID], Title: e.ActorID.Hex()}
	case e.UserID != nil && e.Category == audit.CategoryAuth:
		actor = viewers.Cell{Text: rc.names[*e.UserID], Title: e.UserID.Hex()}
	case e.Details["attempted_login_id"] != "":
		actor = viewers.Cell{Text: e.Details["attempted_login_id"], Class: viewers.TextMuted, Title: "Login attempted; no such user"}
	}
	if e.UserID != nil && e.Category != audit.CategoryAuth {
		target = viewers.Cell{Text: rc.names[*e.UserID], Title: e.UserID.Hex()}
	}
	return actor, target
}

func (v *AuditLog) row(e audit.Event, rc auditContext) viewers.Row {
	loc := rc.location(e)
	timeCell := viewers.Cell{Text: e.Timestamp.In(loc).Format(auditTimeLayout), Title: e.Timestamp.UTC().Format(time.RFC3339) + " UTC · event " + e.ID.Hex()}

	catClass := viewers.PillGray
	switch e.Category {
	case audit.CategoryAuth:
		catClass = viewers.PillBlue
	case audit.CategoryAdmin:
		catClass = viewers.PillIndigo
	case audit.CategorySecurity:
		catClass = viewers.PillAmber
	}
	category := viewers.Cell{Text: auditCategoryLabels[e.Category], Class: catClass}
	if category.Text == "" {
		category.Text = e.Category
	}
	event := viewers.Cell{Text: humanize(e.EventType), Title: e.EventType}

	actor, target := rc.actorAndTarget(e)

	var org viewers.Cell
	if e.OrganizationID != nil {
		org = viewers.Cell{Text: rc.orgNames[*e.OrganizationID], Title: e.OrganizationID.Hex()}
	}

	result := viewers.Cell{Text: "OK", Class: viewers.PillGreen}
	if !e.Success {
		result = viewers.Cell{Text: "Failed", Class: viewers.PillRed, Title: e.FailureReason}
	}
	ip := viewers.Cell{Text: e.IP}

	return viewers.Row{ID: e.ID.Hex(), Cells: []viewers.Cell{timeCell, category, event, actor, target, org, result, ip}}
}

// --- detail ------------------------------------------------------------------

func (v *AuditLog) Detail(ctx context.Context, scope *viewscope.Scope, id string) (*viewers.Detail, error) {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, viewers.ErrNotFound
	}
	sf, err := scope.Filter(ctx, "organization_id", "user_id")
	if err != nil {
		return nil, err
	}
	events, _, err := v.store.List(ctx, audit.ListQuery{WorkspaceID: scope.WorkspaceID, Scope: sf, EventID: &oid, Limit: 1})
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, viewers.ErrNotFound
	}
	e := events[0]
	rc := v.lookups(ctx, events)
	loc := rc.location(e)
	actor, target := rc.actorAndTarget(e)

	fields := []viewers.Field{
		{Label: "Event id", Value: e.ID.Hex(), Mono: true},
		{Label: "Time", Value: e.Timestamp.In(loc).Format(auditTimeLayout) + "  (" + e.Timestamp.UTC().Format(time.RFC3339) + " UTC)"},
		{Label: "Category", Value: auditCategoryLabels[e.Category]},
		{Label: "Event", Value: humanize(e.EventType) + "  (" + e.EventType + ")"},
	}
	if actor.Text != "" {
		fields = append(fields, viewers.Field{Label: "Actor", Value: strings.TrimSpace(actor.Text + "  " + actor.Title)})
	}
	if target.Text != "" {
		fields = append(fields, viewers.Field{Label: "Target", Value: strings.TrimSpace(target.Text + "  " + target.Title)})
	}
	if e.OrganizationID != nil {
		fields = append(fields, viewers.Field{Label: "Organization", Value: strings.TrimSpace(rc.orgNames[*e.OrganizationID] + "  " + e.OrganizationID.Hex())})
	}
	if e.IP != "" {
		fields = append(fields, viewers.Field{Label: "IP address", Value: e.IP, Mono: true})
	}
	if e.UserAgent != "" {
		fields = append(fields, viewers.Field{Label: "User agent", Value: e.UserAgent, Mono: true})
	}
	outcome := "Succeeded"
	if !e.Success {
		outcome = "Failed"
		if e.FailureReason != "" {
			outcome += " — " + e.FailureReason
		}
	}
	fields = append(fields, viewers.Field{Label: "Result", Value: outcome})

	// Details, in a stable order.
	keys := make([]string, 0, len(e.Details))
	for k := range e.Details {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fields = append(fields, viewers.Field{Label: humanize(k), Value: e.Details[k]})
	}

	raw, _ := json.MarshalIndent(struct {
		ID             string            `json:"id"`
		Timestamp      time.Time         `json:"timestamp"`
		Category       string            `json:"category"`
		EventType      string            `json:"event_type"`
		ActorID        string            `json:"actor_id,omitempty"`
		UserID         string            `json:"user_id,omitempty"`
		OrganizationID string            `json:"organization_id,omitempty"`
		IP             string            `json:"ip,omitempty"`
		UserAgent      string            `json:"user_agent,omitempty"`
		Success        bool              `json:"success"`
		FailureReason  string            `json:"failure_reason,omitempty"`
		Details        map[string]string `json:"details,omitempty"`
	}{
		ID: e.ID.Hex(), Timestamp: e.Timestamp.UTC(), Category: e.Category, EventType: e.EventType,
		ActorID: hexOrEmpty(e.ActorID), UserID: hexOrEmpty(e.UserID), OrganizationID: hexOrEmpty(e.OrganizationID),
		IP: e.IP, UserAgent: e.UserAgent, Success: e.Success, FailureReason: e.FailureReason, Details: e.Details,
	}, "", "  ")

	return &viewers.Detail{
		Title:    humanize(e.EventType),
		Subtitle: fmt.Sprintf("%s · %s", auditCategoryLabels[e.Category], outcome),
		Fields:   fields,
		RawJSON:  string(raw),
	}, nil
}

func hexOrEmpty(id *primitive.ObjectID) string {
	if id == nil {
		return ""
	}
	return id.Hex()
}

// --- summary -----------------------------------------------------------------

func (v *AuditLog) Summary(ctx context.Context, scope *viewscope.Scope, f viewers.Filters) ([]viewers.Chip, error) {
	q, err := v.listQuery(ctx, scope, f)
	if err != nil {
		return nil, err
	}
	if q == nil {
		return []viewers.Chip{{Label: "Events", Value: "0"}}, nil
	}
	total, err := v.store.Count(ctx, *q)
	if err != nil {
		return nil, err
	}
	fq := *q
	failed := false
	fq.Success = &failed
	failures := int64(0)
	if q.Success == nil || !*q.Success {
		if failures, err = v.store.Count(ctx, fq); err != nil {
			return nil, err
		}
	}
	chips := []viewers.Chip{{Label: "Events", Value: fmt.Sprint(total)}}
	fc := viewers.Chip{Label: "Failed", Value: fmt.Sprint(failures)}
	if failures > 0 {
		fc.Class = viewers.TextRed
	}
	return append(chips, fc), nil
}
