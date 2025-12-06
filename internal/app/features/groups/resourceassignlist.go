// internal/app/features/groups/resourceassignlist.go
package groups

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/policy/grouppolicy"
	groupstore "github.com/dalemusser/stratahub/internal/app/store/groups"
	orgstore "github.com/dalemusser/stratahub/internal/app/store/organizations"
	resourceassignstore "github.com/dalemusser/stratahub/internal/app/store/resourceassign"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/paging"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/templates"
	mongodb "github.com/dalemusser/waffle/toolkit/db/mongodb"
	textfold "github.com/dalemusser/waffle/toolkit/text/textfold"
	nav "github.com/dalemusser/waffle/toolkit/ui/nav"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

// assignedResourceItem represents a single assigned resource in the list view.
type assignedResourceItem struct {
	AssignmentID      string
	ResourceID        string
	Title             string
	Subject           string
	Type              string
	Status            string
	Availability      string
	AvailabilityTitle string
}

// availableResourceItem represents a single available resource that can be assigned.
type availableResourceItem struct {
	ResourceID string
	Title      string
	Subject    string
	Type       string
	Status     string
}

// assignmentListData is the view model for the Assign Resources page.
type assignmentListData struct {
	Title      string
	IsLoggedIn bool
	Role       string
	UserName   string

	GroupID   string
	GroupName string

	Assigned  []assignedResourceItem
	Available []availableResourceItem

	AvailableShown int
	AvailableTotal int64

	Query       string
	TypeFilter  string
	TypeOptions []string

	CurrentAfter  string
	CurrentBefore string
	NextCursor    string
	PrevCursor    string
	HasNext       bool
	HasPrev       bool

	Flash       template.HTML
	BackURL     string
	CurrentPath string
}

// manageAssignmentModalVM is used to render the modal for managing an existing assignment.
type manageAssignmentModalVM struct {
	AssignmentID  string
	GroupID       string
	GroupName     string
	ResourceID    string
	ResourceTitle string
	ResourceType  string

	BackURL string
}

// ServeAssignResources renders the full Assign Resources page for a group.
func (h *Handler) ServeAssignResources(w http.ResponseWriter, r *http.Request) {
	role, uname, _, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}

	gid := chi.URLParam(r, "id")
	groupOID, err := primitive.ObjectIDFromHex(gid)
	if err != nil {
		uierrors.RenderForbidden(w, r, "Bad group id.", nav.ResolveBackURL(r, "/groups"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), metaShortTimeout)
	defer cancel()
	db := h.DB

	group, err := groupstore.New(db).GetByID(ctx, groupOID)
	if err == mongo.ErrNoDocuments {
		uierrors.RenderForbidden(w, r, "Group not found.", nav.ResolveBackURL(r, "/groups"))
		return
	}
	if err != nil {
		h.Log.Warn("group GetByID(assign)", zap.Error(err))
		uierrors.RenderForbidden(w, r, "A database error occurred.", nav.ResolveBackURL(r, "/groups"))
		return
	}

	if !grouppolicy.CanManageGroup(ctx, db, r, group.ID) {
		uierrors.RenderForbidden(w, r, "You do not have access to this group.", nav.ResolveBackURL(r, "/groups"))
		return
	}

	q := r.URL.Query().Get("q")
	typeFilter := r.URL.Query().Get("type")
	after := r.URL.Query().Get("after")
	before := r.URL.Query().Get("before")

	assigned, avail, shown, total, nextCur, prevCur, hasNext, hasPrev, err := h.buildAssignments(ctx, group, q, typeFilter, after, before)
	if err != nil {
		h.Log.Warn("buildAssignments", zap.Error(err))
		uierrors.RenderForbidden(w, r, "A database error occurred.", nav.ResolveBackURL(r, "/groups/"+group.ID.Hex()+"/manage"))
		return
	}

	back := r.FormValue("return")
	if back == "" {
		back = r.URL.Query().Get("return")
	}
	if back == "" {
		back = "/groups"
	}

	templates.RenderAutoMap(w, r, "group_manage_resources", nil, assignmentListData{
		Title:          "Assign Resources",
		IsLoggedIn:     true,
		Role:           role,
		UserName:       uname,
		GroupID:        group.ID.Hex(),
		GroupName:      group.Name,
		Assigned:       assigned,
		Available:      avail,
		AvailableShown: shown,
		AvailableTotal: total,
		Query:          q,
		TypeFilter:     typeFilter,
		TypeOptions:    models.ResourceTypes,
		CurrentAfter:   after,
		CurrentBefore:  before,
		NextCursor:     nextCur,
		PrevCursor:     prevCur,
		HasNext:        hasNext,
		HasPrev:        hasPrev,
		BackURL:        back,
		CurrentPath:    nav.CurrentPath(r),
	})
}

// HandleSearchResources serves only the Available Resources block (for HTMX search/paging).
func (h *Handler) HandleSearchResources(w http.ResponseWriter, r *http.Request) {
	// If this is a normal (non-HTMX) request, render the full page instead of a bare snippet.
	if r.Header.Get("HX-Request") != "true" {
		h.ServeAssignResources(w, r)
		return
	}

	gid := chi.URLParam(r, "id")
	q := r.URL.Query().Get("q")
	typeFilter := r.URL.Query().Get("type")
	after := r.URL.Query().Get("after")
	before := r.URL.Query().Get("before")

	ctx, cancel := context.WithTimeout(r.Context(), metaShortTimeout)
	defer cancel()
	db := h.DB

	groupOID, err := primitive.ObjectIDFromHex(gid)
	if err != nil {
		uierrors.RenderForbidden(w, r, "Bad group id.", nav.ResolveBackURL(r, "/groups"))
		return
	}

	group, err := groupstore.New(db).GetByID(ctx, groupOID)
	if err != nil {
		uierrors.RenderForbidden(w, r, "Group not found.", nav.ResolveBackURL(r, "/groups"))
		return
	}
	if !grouppolicy.CanManageGroup(ctx, db, r, group.ID) {
		uierrors.RenderForbidden(w, r, "You do not have access to this group.", nav.ResolveBackURL(r, "/groups"))
		return
	}

	assigned, avail, shown, total, nextCur, prevCur, hasNext, hasPrev, err := h.buildAssignments(ctx, group, q, typeFilter, after, before)
	if err != nil {
		uierrors.RenderForbidden(w, r, "A database error occurred.", nav.ResolveBackURL(r, "/groups/"+group.ID.Hex()+"/manage"))
		return
	}

	back := r.FormValue("return")
	if back == "" {
		back = r.URL.Query().Get("return")
	}
	if back == "" {
		back = "/groups"
	}

	data := assignmentListData{
		Title:          "Assign Resources",
		IsLoggedIn:     true,
		GroupID:        group.ID.Hex(),
		GroupName:      group.Name,
		Assigned:       assigned,
		Available:      avail,
		AvailableShown: shown,
		AvailableTotal: total,
		Query:          q,
		TypeFilter:     typeFilter,
		TypeOptions:    models.ResourceTypes,
		CurrentAfter:   after,
		CurrentBefore:  before,
		NextCursor:     nextCur,
		PrevCursor:     prevCur,
		HasNext:        hasNext,
		HasPrev:        hasPrev,
		BackURL:        back,
	}

	templates.RenderSnippet(w, "group_available_resources_block", data)
}

// ServeAssignResourceModal renders the modal fragment used to configure a
// single resource assignment before it is added to the group.
func (h *Handler) ServeAssignResourceModal(w http.ResponseWriter, r *http.Request) {
	_, _, _, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}

	gid := chi.URLParam(r, "id")
	groupOID, err := primitive.ObjectIDFromHex(gid)
	if err != nil {
		uierrors.RenderForbidden(w, r, "Bad group id.", nav.ResolveBackURL(r, "/groups"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), metaShortTimeout)
	defer cancel()
	db := h.DB

	group, err := groupstore.New(db).GetByID(ctx, groupOID)
	if err == mongo.ErrNoDocuments {
		uierrors.RenderForbidden(w, r, "Group not found.", nav.ResolveBackURL(r, "/groups"))
		return
	}
	if err != nil {
		h.Log.Warn("group GetByID(assign modal)", zap.Error(err))
		uierrors.RenderForbidden(w, r, "A database error occurred.", nav.ResolveBackURL(r, "/groups"))
		return
	}

	if !grouppolicy.CanManageGroup(ctx, db, r, group.ID) {
		uierrors.RenderForbidden(w, r, "You do not have access to this group.", nav.ResolveBackURL(r, "/groups"))
		return
	}

	back := r.FormValue("return")
	if back == "" {
		back = nav.ResolveBackURL(r, "/groups/"+group.ID.Hex()+"/assign_resources")
	}

	mode := r.FormValue("mode")
	if mode != "manage" {
		uierrors.RenderForbidden(w, r, "Unsupported operation.", back)
		return
	}

	// Manage an existing assignment: load by assignmentID.
	assignHex := r.FormValue("assignmentID")
	assignID, err := primitive.ObjectIDFromHex(assignHex)
	if err != nil {
		uierrors.RenderForbidden(w, r, "Bad assignment id.", back)
		return
	}

	asn, err := resourceassignstore.New(db).GetByID(ctx, assignID)
	if err == mongo.ErrNoDocuments {
		uierrors.RenderForbidden(w, r, "Assignment not found.", back)
		return
	}
	if err != nil {
		h.Log.Warn("assignment GetByID(manage modal)", zap.Error(err))
		uierrors.RenderForbidden(w, r, "A database error occurred.", back)
		return
	}
	if asn.GroupID != group.ID {
		uierrors.RenderForbidden(w, r, "Assignment does not belong to this group.", back)
		return
	}

	var res models.Resource
	if err := db.Collection("resources").FindOne(ctx, bson.M{"_id": asn.ResourceID}).Decode(&res); err != nil {
		if err == mongo.ErrNoDocuments {
			uierrors.RenderForbidden(w, r, "Resource not found.", back)
			return
		}
		h.Log.Warn("resource FindOne(manage modal)", zap.Error(err))
		uierrors.RenderForbidden(w, r, "A database error occurred.", back)
		return
	}

	manageVM := manageAssignmentModalVM{
		AssignmentID:  asn.ID.Hex(),
		GroupID:       group.ID.Hex(),
		GroupName:     group.Name,
		ResourceID:    res.ID.Hex(),
		ResourceTitle: res.Title,
		ResourceType:  res.Type,
		BackURL:       back,
	}

	templates.RenderSnippet(w, "group_manage_assignment_modal", manageVM)
}

// HandleAssignResource handles POST /groups/{id}/assign_resources/add.
func (h *Handler) HandleAssignResource(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		uierrors.RenderForbidden(w, r, "Bad request.", nav.ResolveBackURL(r, "/groups"))
		return
	}

	gid := chi.URLParam(r, "id")
	groupOID, err := primitive.ObjectIDFromHex(gid)
	if err != nil {
		uierrors.RenderForbidden(w, r, "Bad group id.", nav.ResolveBackURL(r, "/groups"))
		return
	}
	resourceHex := r.FormValue("resourceID")
	resourceOID, err := primitive.ObjectIDFromHex(resourceHex)
	if err != nil {
		uierrors.RenderForbidden(w, r, "Bad resource id.", nav.ResolveBackURL(r, "/groups/"+gid+"/assign_resources"))
		return
	}

	visibleFromStr := strings.TrimSpace(r.FormValue("visible_from"))
	visibleUntilStr := strings.TrimSpace(r.FormValue("visible_until"))
	instructions := strings.TrimSpace(r.FormValue("instructions"))

	ctx, cancel := context.WithTimeout(r.Context(), metaMedTimeout)
	defer cancel()
	db := h.DB

	group, err := groupstore.New(db).GetByID(ctx, groupOID)
	if err == mongo.ErrNoDocuments {
		uierrors.RenderForbidden(w, r, "Group not found.", nav.ResolveBackURL(r, "/groups"))
		return
	}
	if err != nil {
		h.Log.Warn("group GetByID(assign add)", zap.Error(err))
		uierrors.RenderForbidden(w, r, "A database error occurred.", nav.ResolveBackURL(r, "/groups"))
		return
	}

	if !grouppolicy.CanManageGroup(ctx, db, r, group.ID) {
		uierrors.RenderForbidden(w, r, "You do not have access to this group.", nav.ResolveBackURL(r, "/groups"))
		return
	}

	_, uname, _, _ := authz.UserCtx(r)

	// Resolve the organization's timezone so we can interpret the submitted
	// datetime strings in the correct local time before converting to UTC.
	loc := time.Local
	if !group.OrganizationID.IsZero() {
		if org, err := orgstore.New(db).GetByID(ctx, group.OrganizationID); err == nil {
			if tz := strings.TrimSpace(org.TimeZone); tz != "" {
				if l, err := time.LoadLocation(tz); err == nil {
					loc = l
				}
			}
		}
	}

	var visibleFrom *time.Time
	if visibleFromStr != "" {
		if t, err := time.ParseInLocation("2006-01-02T15:04", visibleFromStr, loc); err == nil {
			utc := t.UTC()
			visibleFrom = &utc
		}
	}

	var visibleUntil *time.Time
	if visibleUntilStr != "" {
		if t, err := time.ParseInLocation("2006-01-02T15:04", visibleUntilStr, loc); err == nil {
			utc := t.UTC()
			visibleUntil = &utc
		}
	}

	a := models.GroupResourceAssignment{
		GroupID:        group.ID,
		OrganizationID: group.OrganizationID,
		ResourceID:     resourceOID,
		VisibleFrom:    visibleFrom,
		VisibleUntil:   visibleUntil,
		Instructions:   instructions,
		CreatedByName:  uname,
	}

	if _, err := resourceassignstore.New(db).Create(ctx, a); err != nil {
		h.Log.Warn("resource assign create", zap.Error(err))
		uierrors.RenderForbidden(w, r, "A database error occurred.", nav.ResolveBackURL(r, "/groups/"+gid+"/assign_resources"))
		return
	}

	h.redirectAssignResources(w, r, gid)
}

// HandleRemoveAssignment handles POST /groups/{id}/assign_resources/remove.
func (h *Handler) HandleRemoveAssignment(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		uierrors.RenderForbidden(w, r, "Bad request.", nav.ResolveBackURL(r, "/groups"))
		return
	}

	gid := chi.URLParam(r, "id")
	groupOID, err := primitive.ObjectIDFromHex(gid)
	if err != nil {
		uierrors.RenderForbidden(w, r, "Bad group id.", nav.ResolveBackURL(r, "/groups"))
		return
	}
	assignHex := r.FormValue("assignmentID")
	assignID, err := primitive.ObjectIDFromHex(assignHex)
	if err != nil {
		uierrors.RenderForbidden(w, r, "Bad assignment id.", nav.ResolveBackURL(r, "/groups/"+gid+"/assign_resources"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), metaMedTimeout)
	defer cancel()
	db := h.DB

	group, err := groupstore.New(db).GetByID(ctx, groupOID)
	if err == mongo.ErrNoDocuments {
		uierrors.RenderForbidden(w, r, "Group not found.", nav.ResolveBackURL(r, "/groups"))
		return
	}
	if err != nil {
		h.Log.Warn("group GetByID(assign remove)", zap.Error(err))
		uierrors.RenderForbidden(w, r, "A database error occurred.", nav.ResolveBackURL(r, "/groups"))
		return
	}

	if !grouppolicy.CanManageGroup(ctx, db, r, group.ID) {
		uierrors.RenderForbidden(w, r, "You do not have access to this group.", nav.ResolveBackURL(r, "/groups"))
		return
	}

	asn, err := resourceassignstore.New(db).GetByID(ctx, assignID)
	if err == mongo.ErrNoDocuments {
		uierrors.RenderForbidden(w, r, "Assignment not found.", nav.ResolveBackURL(r, "/groups/"+gid+"/assign_resources"))
		return
	}
	if err != nil {
		h.Log.Warn("assignment GetByID(remove)", zap.Error(err))
		uierrors.RenderForbidden(w, r, "A database error occurred.", nav.ResolveBackURL(r, "/groups/"+gid+"/assign_resources"))
		return
	}
	if asn.GroupID != group.ID {
		uierrors.RenderForbidden(w, r, "Assignment does not belong to this group.", nav.ResolveBackURL(r, "/groups/"+gid+"/assign_resources"))
		return
	}

	if err := resourceassignstore.New(db).Delete(ctx, assignID); err != nil {
		h.Log.Warn("resource assign delete", zap.Error(err))
		uierrors.RenderForbidden(w, r, "A database error occurred.", nav.ResolveBackURL(r, "/groups/"+gid+"/assign_resources"))
		return
	}

	h.redirectAssignResources(w, r, gid)
}

// buildAssignments composes the assigned + available resources lists, including
// availability summaries, paging state, and filters.
func (h *Handler) buildAssignments(ctx context.Context, g models.Group, q, typeFilter, after, before string) (
	assigned []assignedResourceItem,
	avail []availableResourceItem,
	shown int,
	total int64,
	nextCursor, prevCursor string,
	hasNext, hasPrev bool,
	err error,
) {
	db := h.DB

	// assigned resource ids
	assns, e := resourceassignstore.New(db).ListByGroup(ctx, g.ID)
	if e != nil {
		err = e
		return
	}

	assignedIDs := make([]primitive.ObjectID, 0, len(assns))
	for _, a := range assns {
		assignedIDs = append(assignedIDs, a.ResourceID)
	}

	// Use organization-specific timezone for assignment availability
	loc, _ := resolveGroupLocation(ctx, db, g)
	now := time.Now().In(loc)

	// load assigned resources
	if len(assignedIDs) > 0 {
		cg, _ := db.Collection("resources").Find(ctx, bson.M{"_id": bson.M{"$in": assignedIDs}})
		defer cg.Close(ctx)

		resByID := make(map[primitive.ObjectID]models.Resource)
		for cg.Next(ctx) {
			var rdoc models.Resource
			if err = cg.Decode(&rdoc); err != nil {
				h.Log.Warn("decode assigned resource", zap.Error(err))
				continue
			}
			resByID[rdoc.ID] = rdoc
		}

		for _, a := range assns {
			res, ok := resByID[a.ResourceID]
			if !ok {
				continue
			}

			availStr, availTitle := summarizeAssignmentAvailability(now, a.VisibleFrom, a.VisibleUntil)

			assigned = append(assigned, assignedResourceItem{
				AssignmentID:      a.ID.Hex(),
				ResourceID:        res.ID.Hex(),
				Title:             res.Title,
				Subject:           res.Subject,
				Type:              res.Type,
				Status:            res.Status,
				Availability:      availStr,
				AvailabilityTitle: availTitle,
			})
		}
	}

	// sort assigned by Title (case-insensitive)
	sort.SliceStable(assigned, func(i, j int) bool {
		ti := strings.ToLower(assigned[i].Title)
		tj := strings.ToLower(assigned[j].Title)
		if ti == tj {
			return assigned[i].Title < assigned[j].Title
		}
		return ti < tj
	})

	// load available resources with paging/search (status:active only; do not exclude assigned)
	avail, shown, total, nextCursor, prevCursor, hasNext, hasPrev, err = h.fetchAvailableResourcesPaged(ctx, q, typeFilter, after, before)
	return
}

// fetchAvailableResourcesPaged returns a page of available resources with optional
// search and type filters, using keyset pagination on (title_ci, _id).
func (h *Handler) fetchAvailableResourcesPaged(
	ctx context.Context,
	qRaw, typeFilter, after, before string,
) (resources []availableResourceItem, shown int, total int64, nextCursor, prevCursor string, hasNext, hasPrev bool, err error) {
	db := h.DB
	coll := db.Collection("resources")

	filter := bson.M{"status": "active"}
	if strings.TrimSpace(typeFilter) != "" {
		filter["type"] = strings.TrimSpace(typeFilter)
	}

	q := textfold.Fold(qRaw)
	if q != "" {
		high := q + "\uffff"
		filter["title_ci"] = bson.M{"$gte": q, "$lt": high}
	}

	total, _ = coll.CountDocuments(ctx, filter)

	findOpts := options.Find()
	limit := paging.LimitPlusOne()
	if before != "" {
		if c, ok := mongodb.DecodeCursor(before); ok {
			filter["$or"] = []bson.M{
				{"title_ci": bson.M{"$lt": c.CI}},
				{"title_ci": c.CI, "_id": bson.M{"$lt": c.ID}},
			}
		}
		findOpts.SetSort(bson.D{{Key: "title_ci", Value: -1}, {Key: "_id", Value: -1}}).SetLimit(limit)
	} else {
		if after != "" {
			if c, ok := mongodb.DecodeCursor(after); ok {
				filter["$or"] = []bson.M{
					{"title_ci": bson.M{"$gt": c.CI}},
					{"title_ci": c.CI, "_id": bson.M{"$gt": c.ID}},
				}
			}
		}
		findOpts.SetSort(bson.D{{Key: "title_ci", Value: 1}, {Key: "_id", Value: 1}}).SetLimit(limit)
	}

	cur, e := coll.Find(ctx, filter, findOpts)
	if e != nil {
		err = e
		return
	}
	defer cur.Close(ctx)

	var rows []struct {
		ID      primitive.ObjectID `bson:"_id"`
		Title   string             `bson:"title"`
		TitleCI string             `bson:"title_ci"`
		Subject string             `bson:"subject"`
		Type    string             `bson:"type"`
		Status  string             `bson:"status"`
	}
	if err = cur.All(ctx, &rows); err != nil {
		return
	}

	if before != "" {
		for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
			rows[i], rows[j] = rows[j], rows[i]
		}
	}

	orig := len(rows)
	if before != "" {
		if orig > paging.PageSize {
			rows = rows[1:]
			hasPrev = true
		} else {
			hasPrev = false
		}
		hasNext = true
	} else {
		if orig > paging.PageSize {
			rows = rows[:paging.PageSize]
			hasNext = true
		} else {
			hasNext = false
		}
		hasPrev = after != ""
	}

	resources = make([]availableResourceItem, 0, len(rows))
	for _, r := range rows {
		resources = append(resources, availableResourceItem{
			ResourceID: r.ID.Hex(),
			Title:      r.Title,
			Subject:    r.Subject,
			Type:       r.Type,
			Status:     r.Status,
		})
	}
	shown = len(resources)
	if shown > 0 {
		first := rows[0]
		last := rows[shown-1]
		prevCursor = mongodb.EncodeCursor(first.TitleCI, first.ID)
		nextCursor = mongodb.EncodeCursor(last.TitleCI, last.ID)
	}

	return
}

// summarizeAssignmentAvailability builds a compact availability summary and
// a tooltip from optional VisibleFrom/VisibleUntil timestamps.
func summarizeAssignmentAvailability(now time.Time, from, until *time.Time) (string, string) {
	// Build the tooltip with full datetimes
	layout := "2006-01-02 15:04"
	startLabel := "No start date"
	endLabel := "No end date"

	var fromVal, untilVal time.Time
	if from != nil {
		fromVal = *from
		if !fromVal.IsZero() {
			startLabel = "Starts " + fromVal.Format(layout)
		}
	}
	if until != nil {
		untilVal = *until
		if !untilVal.IsZero() {
			endLabel = "Ends " + untilVal.Format(layout)
		}
	}

	tooltip := fmt.Sprintf("%s — %s", startLabel, endLabel)

	// Build the compact relative summary
	var startPart string
	if from == nil || fromVal.IsZero() {
		startPart = "Unscheduled"
	} else if !now.Before(fromVal) {
		startPart = "Now"
	} else {
		startPart = "In " + humanizeDuration(fromVal.Sub(now))
	}

	var endPart string
	if until == nil || untilVal.IsZero() {
		endPart = "No end"
	} else if now.After(untilVal) {
		endPart = "Ended"
	} else {
		endPart = "Ends in " + humanizeDuration(untilVal.Sub(now))
	}

	if startPart == "" && endPart == "" {
		return "", tooltip
	}
	if endPart == "" {
		return startPart, tooltip
	}
	return startPart + " → " + endPart, tooltip
}

// humanizeDuration produces short, coarse-grained phrases like "2 days" or
// "5 hours" from a positive duration.
func humanizeDuration(d time.Duration) string {
	if d < 0 {
		d = -d
	}

	// Round to the nearest hour
	hours := int(d.Hours() + 0.5)
	if hours <= 1 {
		return "1 hour"
	}
	if hours < 24 {
		return fmt.Sprintf("%d hours", hours)
	}

	days := hours / 24
	if days <= 1 {
		return "1 day"
	}
	return fmt.Sprintf("%d days", days)
}

// redirectAssignResources redirects (or HTMX-navigates) back to the
// /assign_resources page for the given group, preserving any ?return=...
// parameter when present.
func (h *Handler) redirectAssignResources(w http.ResponseWriter, r *http.Request, gid string) {
	dest := "/groups/" + gid + "/assign_resources"

	// preserve a ?return=... if present
	if ret := r.FormValue("return"); ret != "" && strings.HasPrefix(ret, "/") {
		dest = dest + "?return=" + url.QueryEscape(ret)
	} else if ret := r.URL.Query().Get("return"); ret != "" && strings.HasPrefix(ret, "/") {
		dest = dest + "?return=" + url.QueryEscape(ret)
	}

	if r.Header.Get("HX-Request") == "true" {
		// For HTMX, prefer client-side navigation instead of plain 303.
		w.Header().Set("HX-Location", `{"path":"`+dest+`","target":"#content","swap":"innerHTML"}`)
		w.Header().Set("HX-Push-Url", "true")
		w.WriteHeader(http.StatusOK)
		return
	}

	http.Redirect(w, r, dest, http.StatusSeeOther)
}
