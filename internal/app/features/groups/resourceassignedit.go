// internal/app/features/groups/resourceassignedit.go
package groups

import (
	"context"
	"net/http"
	"strings"
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/policy/grouppolicy"
	groupstore "github.com/dalemusser/stratahub/internal/app/store/groups"
	resourceassignstore "github.com/dalemusser/stratahub/internal/app/store/resourceassign"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/templates"
	nav "github.com/dalemusser/waffle/toolkit/ui/nav"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// editResourceAssignmentPageVM is the view model for the Edit Resource Assignment page.
type editResourceAssignmentPageVM struct {
	Title       string
	IsLoggedIn  bool
	Role        string
	UserName    string
	CurrentPath string

	GroupID   string
	GroupName string

	AssignmentID  string
	ResourceID    string
	ResourceTitle string
	ResourceType  string
	Subject       string

	VisibleFrom  string // for type="datetime-local"
	VisibleUntil string
	Instructions string

	TimeZone string
	BackURL  string
}

// ServeEditResourceAssignmentPage renders the page for editing an existing
// resource assignment for a group.
func (h *Handler) ServeEditResourceAssignmentPage(w http.ResponseWriter, r *http.Request) {
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

	assignHex := chi.URLParam(r, "assignmentID")
	assignID, err := primitive.ObjectIDFromHex(assignHex)
	if err != nil {
		uierrors.RenderForbidden(w, r, "Bad assignment id.", nav.ResolveBackURL(r, "/groups/"+gid+"/assign_resources"))
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
		h.Log.Warn("group GetByID(edit assignment)", zap.Error(err))
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
		h.Log.Warn("assignment GetByID(edit)", zap.Error(err))
		uierrors.RenderForbidden(w, r, "A database error occurred.", nav.ResolveBackURL(r, "/groups/"+gid+"/assign_resources"))
		return
	}
	if asn.GroupID != group.ID {
		uierrors.RenderForbidden(w, r, "Assignment does not belong to this group.", nav.ResolveBackURL(r, "/groups/"+gid+"/assign_resources"))
		return
	}

	// Determine the organization's time zone and current time in that zone.
	loc, tzLabel := resolveGroupLocation(ctx, db, group)

	var res models.Resource
	if err := db.Collection("resources").FindOne(ctx, bson.M{"_id": asn.ResourceID}).Decode(&res); err != nil {
		if err == mongo.ErrNoDocuments {
			uierrors.RenderForbidden(w, r, "Resource not found.", nav.ResolveBackURL(r, "/groups/"+gid+"/assign_resources"))
			return
		}
		h.Log.Warn("resource FindOne(edit assignment)", zap.Error(err))
		uierrors.RenderForbidden(w, r, "A database error occurred.", nav.ResolveBackURL(r, "/groups/"+gid+"/assign_resources"))
		return
	}

	// Convert stored UTC times (if any) into local time strings for the form.
	const dtLayout = "2006-01-02T15:04"
	var visibleFromStr, visibleUntilStr string
	if asn.VisibleFrom != nil && !asn.VisibleFrom.IsZero() {
		visibleFromStr = asn.VisibleFrom.In(loc).Format(dtLayout)
	}
	if asn.VisibleUntil != nil && !asn.VisibleUntil.IsZero() {
		visibleUntilStr = asn.VisibleUntil.In(loc).Format(dtLayout)
	}

	back := r.URL.Query().Get("return")
	if back == "" {
		back = nav.ResolveBackURL(r, "/groups/"+group.ID.Hex()+"/assign_resources")
	}

	vm := editResourceAssignmentPageVM{
		Title:       "📚 Edit Resource Assignment",
		IsLoggedIn:  true,
		Role:        role,
		UserName:    uname,
		CurrentPath: nav.CurrentPath(r),

		GroupID:       group.ID.Hex(),
		GroupName:     group.Name,
		AssignmentID:  asn.ID.Hex(),
		ResourceID:    res.ID.Hex(),
		ResourceTitle: res.Title,
		ResourceType:  res.Type,
		Subject:       res.Subject,
		VisibleFrom:   visibleFromStr,
		VisibleUntil:  visibleUntilStr,
		Instructions:  asn.Instructions,
		TimeZone:      tzLabel,
		BackURL:       back,
	}

	templates.RenderAutoMap(w, r, "resource_assignment_edit", nil, vm)
}

// HandleUpdateResourceAssignment processes the POST from the Edit Resource
// Assignment page and updates the assignment document.
func (h *Handler) HandleUpdateResourceAssignment(w http.ResponseWriter, r *http.Request) {
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
	assignHex := chi.URLParam(r, "assignmentID")
	assignID, err := primitive.ObjectIDFromHex(assignHex)
	if err != nil {
		uierrors.RenderForbidden(w, r, "Bad assignment id.", nav.ResolveBackURL(r, "/groups/"+gid+"/assign_resources"))
		return
	}

	visibleFromStr := r.FormValue("visible_from")
	visibleUntilStr := r.FormValue("visible_until")
	instructions := r.FormValue("instructions")

	ctx, cancel := context.WithTimeout(r.Context(), metaMedTimeout)
	defer cancel()
	db := h.DB

	group, err := groupstore.New(db).GetByID(ctx, groupOID)
	if err == mongo.ErrNoDocuments {
		uierrors.RenderForbidden(w, r, "Group not found.", nav.ResolveBackURL(r, "/groups"))
		return
	}
	if err != nil {
		h.Log.Warn("group GetByID(update assignment)", zap.Error(err))
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
		h.Log.Warn("assignment GetByID(update)", zap.Error(err))
		uierrors.RenderForbidden(w, r, "A database error occurred.", nav.ResolveBackURL(r, "/groups/"+gid+"/assign_resources"))
		return
	}
	if asn.GroupID != group.ID {
		uierrors.RenderForbidden(w, r, "Assignment does not belong to this group.", nav.ResolveBackURL(r, "/groups/"+gid+"/assign_resources"))
		return
	}

	_, uname, _, _ := authz.UserCtx(r)

	// Resolve the organization's timezone so we can interpret the submitted
	// datetime strings in the correct local time before converting to UTC.
	loc, _ := resolveGroupLocation(ctx, db, group)

	const dtLayout = "2006-01-02T15:04"

	asn.VisibleFrom = nil
	if s := strings.TrimSpace(visibleFromStr); s != "" {
		if t, err := time.ParseInLocation(dtLayout, s, loc); err == nil {
			utc := t.UTC()
			asn.VisibleFrom = &utc
		}
	}

	asn.VisibleUntil = nil
	if s := strings.TrimSpace(visibleUntilStr); s != "" {
		if t, err := time.ParseInLocation(dtLayout, s, loc); err == nil {
			utc := t.UTC()
			asn.VisibleUntil = &utc
		}
	}

	asn.Instructions = instructions
	asn.UpdatedByName = uname

	if _, err := resourceassignstore.New(db).Update(ctx, asn); err != nil {
		h.Log.Warn("assignment Update", zap.Error(err))
		uierrors.RenderForbidden(w, r, "A database error occurred.", nav.ResolveBackURL(r, "/groups/"+gid+"/assign_resources"))
		return
	}

	// Redirect back to the assignments list (or provided return URL).
	h.redirectAssignResources(w, r, gid)
}
