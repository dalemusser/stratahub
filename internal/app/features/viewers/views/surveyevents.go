// Package views holds the concrete data viewers registered with the viewers
// framework at bootstrap. Each viewer is a small type: a query over its store
// plus row and detail mapping. See docs/viewers/plan.md.
package views

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/dalemusser/stratahub/internal/app/features/viewers"
	"github.com/dalemusser/stratahub/internal/app/store/memberstatuslog"
	"github.com/dalemusser/stratahub/internal/app/system/memberstatuscfg"
	"github.com/dalemusser/stratahub/internal/app/system/viewscope"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/text"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// SurveyEventsSlug is the viewer's URL segment: /views/survey-events.
const SurveyEventsSlug = "survey-events"

const (
	surveyEventsLiveSeconds = 10
	surveyEventsTimeLayout  = "Jan 2, 2006 3:04:05 PM MST"
	nameSearchLimit         = 200
)

// SurveyEvents lists every member-status event received — from the survey
// provider via the Member Status API and from survey-resource launches — so
// the provider's developer and staff can confirm an event arrived, was
// stored, and carried the right content.
type SurveyEvents struct {
	db  *mongo.Database
	log *memberstatuslog.Store
	cfg *memberstatuscfg.Config
}

// NewSurveyEvents constructs the viewer. It loads the survey configuration up
// front (it drives the Survey filter and column labels).
func NewSurveyEvents(db *mongo.Database) (*SurveyEvents, error) {
	cfg, err := memberstatuscfg.Load()
	if err != nil {
		return nil, err
	}
	return &SurveyEvents{db: db, log: memberstatuslog.New(db), cfg: cfg}, nil
}

func (v *SurveyEvents) Slug() string { return SurveyEventsSlug }

// Title derives from the configured tab title ("Surveys" → "Survey Events").
func (v *SurveyEvents) Title() string {
	base := strings.TrimSpace(v.cfg.TabTitle)
	if base == "" {
		base = "Surveys"
	}
	return strings.TrimSuffix(base, "s") + " Events"
}

func (v *SurveyEvents) Description() string {
	return "Every survey status event received — from the survey provider and from survey links opened in StrataHub — with what arrived, what it resolved to, and whether it was accepted."
}

func (v *SurveyEvents) Roles() []string { return []string{"admin", "analyst", "coordinator", "leader"} }

func (v *SurveyEvents) LiveIntervalSeconds() int { return surveyEventsLiveSeconds }

// Filter keys.
const (
	fWhen    = "when"
	fSource  = "source"
	fEntity  = "entity"
	fState   = "state"
	fOutcome = "outcome"
	fStudent = "student"
	fEvent   = "event"
)

func (v *SurveyEvents) Filters() []viewers.FilterSpec {
	entity := []viewers.Option{}
	for _, item := range v.cfg.Items {
		entity = append(entity, viewers.Option{Value: item.ID, Label: item.Title})
	}
	entity = append(entity, viewers.Option{Value: memberstatuslog.EntityUnrecognized, Label: "Unrecognized name"})

	return []viewers.FilterSpec{
		{Key: fWhen, Label: "Received", Type: viewers.FilterDateRange, Default: viewers.RangeWeek},
		{Key: fSource, Label: "Source", Type: viewers.FilterSelect, Options: []viewers.Option{
			{Value: models.MemberStatusSourceAPI, Label: "Provider (API)"},
			{Value: models.MemberStatusSourceLaunch, Label: "Launch in StrataHub"},
		}},
		{Key: fEntity, Label: "Survey", Type: viewers.FilterSelect, Options: entity},
		{Key: fState, Label: "State sent", Type: viewers.FilterSelect, Options: []viewers.Option{
			{Value: models.MemberStatusOpened, Label: "Opened"},
			{Value: models.MemberStatusStarted, Label: "Started"},
			{Value: models.MemberStatusCompleted, Label: "Completed"},
		}},
		{Key: fOutcome, Label: "Result", Type: viewers.FilterSelect, Options: []viewers.Option{
			{Value: memberstatuslog.OutcomeAccepted, Label: "Accepted"},
			{Value: memberstatuslog.OutcomeRejected, Label: "Rejected (any)"},
			{Value: "unknown_user", Label: "Rejected: unknown user"},
			{Value: "invalid_user_id", Label: "Rejected: invalid user id"},
			{Value: "invalid_entity", Label: "Rejected: invalid survey name"},
			{Value: "invalid_state", Label: "Rejected: invalid state"},
			{Value: "invalid_occurred_at", Label: "Rejected: invalid timestamp"},
			{Value: "server_error", Label: "Rejected: server error"},
		}},
		{Key: fStudent, Label: "Student", Type: viewers.FilterText, Placeholder: "name or 24-char id"},
		{Key: fEvent, Label: "Event id", Type: viewers.FilterID, Placeholder: "24-char event id"},
	}
}

func (v *SurveyEvents) Columns() []viewers.ColumnSpec {
	return []viewers.ColumnSpec{
		{Key: "received", Label: "Received", Class: "whitespace-nowrap"},
		{Key: "source", Label: "Source"},
		{Key: "student", Label: "Student"},
		{Key: "survey", Label: "Survey"},
		{Key: "state", Label: "State sent"},
		{Key: "result", Label: "Result", Class: "whitespace-nowrap"},
	}
}

// --- query -------------------------------------------------------------------

// listQuery translates scope + filters into a store query. It returns nil
// when the filters can match nothing (a student search with no hits).
func (v *SurveyEvents) listQuery(ctx context.Context, scope *viewscope.Scope, f viewers.Filters) (*memberstatuslog.ListQuery, error) {
	sf, err := scope.Filter(ctx, "resolved.organization_id", "resolved.user_id")
	if err != nil {
		return nil, err
	}
	q := &memberstatuslog.ListQuery{
		WorkspaceID: scope.WorkspaceID,
		Scope:       sf,
		Source:      f.Get(fSource),
		EntityKey:   f.Get(fEntity),
		StateSent:   f.Get(fState),
		Outcome:     f.Get(fOutcome),
	}
	if from, to, ok := f.DateRange(fWhen, time.Now().UTC()); ok {
		q.From, q.To = from, to
	}
	if id, ok := f.ID(fEvent); ok {
		q.EventID = &id
	} else if f.Has(fEvent) {
		return nil, nil // not a valid id: nothing can match
	}
	if s := strings.TrimSpace(f.Get(fStudent)); s != "" {
		if _, err := primitive.ObjectIDFromHex(s); err == nil {
			// Exact id, as sent: finds accepted and rejected entries alike.
			q.RawUserID = strings.ToLower(s)
		} else {
			ids, err := v.searchMembersByName(ctx, scope.WorkspaceID, s)
			if err != nil {
				return nil, err
			}
			if len(ids) == 0 {
				return nil, nil
			}
			q.UserIDs = ids
		}
	}
	return q, nil
}

// searchMembersByName finds member ids whose folded full name starts with
// the folded query (prefix match, index-friendly).
func (v *SurveyEvents) searchMembersByName(ctx context.Context, wsID primitive.ObjectID, name string) ([]primitive.ObjectID, error) {
	prefix := "^" + regexp.QuoteMeta(text.Fold(name))
	cur, err := v.db.Collection("users").Find(ctx, bson.M{
		"workspace_id": wsID,
		"role":         "member",
		"full_name_ci": bson.M{"$regex": prefix},
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

func (v *SurveyEvents) Query(ctx context.Context, scope *viewscope.Scope, f viewers.Filters, after *viewers.Cursor, limit int) (viewers.Page, error) {
	q, err := v.listQuery(ctx, scope, f)
	if err != nil || q == nil {
		return viewers.Page{}, err
	}
	if after != nil {
		at, err := viewers.ParseTimeKey(after.Key)
		if err != nil {
			return viewers.Page{}, err
		}
		q.After = &memberstatuslog.Position{ReceivedAt: at, ID: after.ID}
	}
	q.Limit = limit

	entries, more, err := v.log.List(ctx, *q)
	if err != nil {
		return viewers.Page{}, err
	}
	ctxInfo := v.lookups(ctx, entries)

	page := viewers.Page{}
	for _, e := range entries {
		page.Rows = append(page.Rows, v.row(e, ctxInfo))
	}
	if more && len(entries) > 0 {
		last := entries[len(entries)-1]
		page.Next = &viewers.Cursor{Key: viewers.TimeKey(last.ReceivedAt), ID: last.ID}
	}
	return page, nil
}

// rowContext is the per-page lookup of student names and organization time
// zones referenced by the entries.
type rowContext struct {
	names map[primitive.ObjectID]string
	locs  map[primitive.ObjectID]*time.Location
}

func (v *SurveyEvents) lookups(ctx context.Context, entries []models.MemberStatusLogEntry) rowContext {
	rc := rowContext{names: map[primitive.ObjectID]string{}, locs: map[primitive.ObjectID]*time.Location{}}
	var userIDs, orgIDs []primitive.ObjectID
	seenU, seenO := map[primitive.ObjectID]bool{}, map[primitive.ObjectID]bool{}
	for _, e := range entries {
		if e.Resolved.UserID != nil && !seenU[*e.Resolved.UserID] {
			seenU[*e.Resolved.UserID] = true
			userIDs = append(userIDs, *e.Resolved.UserID)
		}
		if e.Resolved.OrganizationID != nil && !seenO[*e.Resolved.OrganizationID] {
			seenO[*e.Resolved.OrganizationID] = true
			orgIDs = append(orgIDs, *e.Resolved.OrganizationID)
		}
	}
	if len(userIDs) > 0 {
		cur, err := v.db.Collection("users").Find(ctx, bson.M{"_id": bson.M{"$in": userIDs}}, options.Find().SetProjection(bson.M{"full_name": 1}))
		if err == nil {
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
	if len(orgIDs) > 0 {
		cur, err := v.db.Collection("organizations").Find(ctx, bson.M{"_id": bson.M{"$in": orgIDs}}, options.Find().SetProjection(bson.M{"time_zone": 1}))
		if err == nil {
			for cur.Next(ctx) {
				var o struct {
					ID       primitive.ObjectID `bson:"_id"`
					TimeZone string             `bson:"time_zone"`
				}
				if cur.Decode(&o) == nil && o.TimeZone != "" {
					if loc, err := time.LoadLocation(o.TimeZone); err == nil {
						rc.locs[o.ID] = loc
					}
				}
			}
			cur.Close(ctx)
		}
	}
	return rc
}

func (rc rowContext) location(e models.MemberStatusLogEntry) *time.Location {
	if e.Resolved.OrganizationID != nil {
		if loc, ok := rc.locs[*e.Resolved.OrganizationID]; ok {
			return loc
		}
	}
	return time.UTC
}

// --- rows --------------------------------------------------------------------

var stateLabels = map[string]string{
	models.MemberStatusOpened:    "Opened",
	models.MemberStatusStarted:   "Started",
	models.MemberStatusCompleted: "Completed",
}

func stateLabel(s string) string {
	if l, ok := stateLabels[strings.ToLower(strings.TrimSpace(s))]; ok {
		return l
	}
	return s
}

func statePill(state string) string {
	switch state {
	case models.MemberStatusCompleted:
		return viewers.PillGreen
	case models.MemberStatusStarted:
		return viewers.PillIndigo
	case models.MemberStatusOpened:
		return viewers.PillBlue
	}
	return viewers.PillGray
}

func (v *SurveyEvents) row(e models.MemberStatusLogEntry, rc rowContext) viewers.Row {
	loc := rc.location(e)
	received := viewers.Cell{
		Text:  e.ReceivedAt.In(loc).Format(surveyEventsTimeLayout),
		Title: e.ReceivedAt.UTC().Format(time.RFC3339) + " UTC · event " + e.ID.Hex(),
	}

	source := viewers.Cell{Text: "Provider", Class: viewers.PillIndigo, Title: "Reported by the survey provider via the API"}
	if e.Source == models.MemberStatusSourceLaunch {
		source = viewers.Cell{Text: "Launch", Class: viewers.PillBlue, Title: "Student opened the survey link in StrataHub"}
	}

	var student viewers.Cell
	if e.Resolved.UserID != nil {
		name := rc.names[*e.Resolved.UserID]
		if name == "" {
			name = e.Resolved.UserID.Hex()
		}
		student = viewers.Cell{Text: name, Title: e.Resolved.UserID.Hex(), Href: "/views/" + SurveyEventsSlug + "?student=" + e.Resolved.UserID.Hex()}
	} else {
		raw := e.Request.UserID
		if raw == "" {
			raw = "—"
		}
		student = viewers.Cell{Text: raw, Class: viewers.PillGray, Title: "Not matched to a member in this workspace"}
	}

	var survey viewers.Cell
	switch {
	case e.Resolved.EntityKey != "" && e.Resolved.KnownEntity:
		survey = viewers.Cell{Text: e.Resolved.EntityTitle, Title: "Sent as: " + e.Request.Entity}
	case e.Resolved.EntityKey != "":
		survey = viewers.Cell{Text: "Unrecognized: " + e.Request.Entity, Class: viewers.PillAmber, Title: "Name not in the survey configuration; stored under " + e.Resolved.EntityKey}
	default:
		survey = viewers.Cell{Text: e.Request.Entity, Title: "As sent"}
	}

	state := viewers.Cell{Text: stateLabel(e.Request.State), Title: "Sent as: " + e.Request.State}

	var result viewers.Cell
	if e.Accepted() {
		rs := e.Resolved.ResultingState
		result = viewers.Cell{Text: "Accepted · " + stateLabel(rs), Class: statePill(rs), Title: "Student's status after this event: " + stateLabel(rs)}
	} else {
		result = viewers.Cell{Text: "Rejected · " + e.Outcome.Error, Class: viewers.PillRed, Title: fmt.Sprintf("%d %s", e.Outcome.HTTPStatus, e.Outcome.Message)}
	}

	return viewers.Row{ID: e.ID.Hex(), Cells: []viewers.Cell{received, source, student, survey, state, result}}
}

// --- detail ------------------------------------------------------------------

func (v *SurveyEvents) Detail(ctx context.Context, scope *viewscope.Scope, id string) (*viewers.Detail, error) {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, viewers.ErrNotFound
	}
	// Re-apply scope: list with the id as the only filter.
	sf, err := scope.Filter(ctx, "resolved.organization_id", "resolved.user_id")
	if err != nil {
		return nil, err
	}
	entries, _, err := v.log.List(ctx, memberstatuslog.ListQuery{WorkspaceID: scope.WorkspaceID, Scope: sf, EventID: &oid, Limit: 1})
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, viewers.ErrNotFound
	}
	e := entries[0]
	rc := v.lookups(ctx, entries)
	loc := rc.location(e)

	fields := []viewers.Field{
		{Label: "Event id", Value: e.ID.Hex(), Mono: true},
		{Label: "Received", Value: e.ReceivedAt.In(loc).Format(surveyEventsTimeLayout) + "  (" + e.ReceivedAt.UTC().Format(time.RFC3339) + " UTC)"},
		{Label: "Source", Value: map[string]string{models.MemberStatusSourceAPI: "Provider (API)", models.MemberStatusSourceLaunch: "Launch in StrataHub"}[e.Source]},
	}
	if e.RemoteIP != "" {
		fields = append(fields, viewers.Field{Label: "From IP", Value: e.RemoteIP, Mono: true})
	}
	fields = append(fields, []viewers.Field{
		{Label: "Sent user_id", Value: e.Request.UserID, Mono: true},
		{Label: "Sent entity", Value: e.Request.Entity},
		{Label: "Sent state", Value: e.Request.State},
	}...)
	if e.Request.OccurredAt != "" {
		fields = append(fields, viewers.Field{Label: "Sent occurred_at", Value: e.Request.OccurredAt, Mono: true})
	}
	if e.Request.ResourceID != "" {
		fields = append(fields, viewers.Field{Label: "Resource", Value: e.Request.ResourceID, Mono: true})
	}
	if e.Resolved.UserID != nil {
		name := rc.names[*e.Resolved.UserID]
		fields = append(fields, viewers.Field{Label: "Student", Value: strings.TrimSpace(name + "  " + e.Resolved.UserID.Hex())})
	}
	if e.Resolved.EntityKey != "" {
		known := "recognized"
		if !e.Resolved.KnownEntity {
			known = "NOT in the survey configuration"
		}
		fields = append(fields, viewers.Field{Label: "Survey", Value: fmt.Sprintf("%s (%s, key %s)", e.Resolved.EntityTitle, known, e.Resolved.EntityKey)})
	}
	if e.Resolved.StateApplied != "" {
		fields = append(fields, viewers.Field{Label: "State applied", Value: stateLabel(e.Resolved.StateApplied)})
	}
	if e.Resolved.ResultingState != "" {
		fields = append(fields, viewers.Field{Label: "Student's status after", Value: stateLabel(e.Resolved.ResultingState)})
	}
	outcome := fmt.Sprintf("%d accepted", e.Outcome.HTTPStatus)
	if !e.Accepted() {
		outcome = fmt.Sprintf("%d %s — %s", e.Outcome.HTTPStatus, e.Outcome.Error, e.Outcome.Message)
	}
	fields = append(fields, viewers.Field{Label: "Outcome", Value: outcome})

	raw, _ := json.MarshalIndent(e, "", "  ")
	title := "Event " + e.ID.Hex()
	subtitle := "Accepted"
	if !e.Accepted() {
		subtitle = "Rejected: " + e.Outcome.Error
	}
	return &viewers.Detail{Title: title, Subtitle: subtitle, Fields: fields, RawJSON: string(raw)}, nil
}

// --- summary -----------------------------------------------------------------

func (v *SurveyEvents) Summary(ctx context.Context, scope *viewscope.Scope, f viewers.Filters) ([]viewers.Chip, error) {
	q, err := v.listQuery(ctx, scope, f)
	if err != nil {
		return nil, err
	}
	if q == nil {
		return []viewers.Chip{{Label: "Events", Value: "0"}}, nil
	}
	sum, err := v.log.Summarize(ctx, *q)
	if err != nil {
		return nil, err
	}
	chips := []viewers.Chip{
		{Label: "Events", Value: fmt.Sprint(sum.Total)},
		{Label: "Accepted", Value: fmt.Sprint(sum.Accepted), Class: viewers.TextGreen},
	}
	rejected := viewers.Chip{Label: "Rejected", Value: fmt.Sprint(sum.Rejected)}
	if sum.Rejected > 0 {
		rejected.Class = viewers.TextRed
	}
	chips = append(chips, rejected)
	last := "—"
	if sum.Last != nil {
		last = relativeTime(time.Since(*sum.Last))
	}
	chips = append(chips, viewers.Chip{Label: "Last received", Value: last, Class: viewers.TextMuted})
	return chips, nil
}

// relativeTime renders "just now", "4 min ago", "3 h ago", "2 d ago".
func relativeTime(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%d min ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%d h ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%d d ago", int(d.Hours()/24))
	}
}
