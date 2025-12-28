// internal/app/features/groups/resourceassignview.go
package groups

import (
	"context"
	"net/http"
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/policy/grouppolicy"
	groupstore "github.com/dalemusser/stratahub/internal/app/store/groups"
	resourceassignstore "github.com/dalemusser/stratahub/internal/app/store/resourceassign"
	resourcestore "github.com/dalemusser/stratahub/internal/app/store/resources"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/waffle/pantry/httpnav"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// viewResourceAssignmentPageVM is the view model for the View Resource
// Assignment page. It presents a read-only view of the assignment and its
// related resource.
type viewResourceAssignmentPageVM struct {
	viewdata.BaseVM

	GroupID   string
	GroupName string

	AssignmentID  string
	ResourceID    string
	ResourceTitle string
	Subject       string
	Type          string

	Availability string
	VisibleFrom  string
	VisibleUntil string
	Instructions string

	CreatedAt     string
	CreatedByName string
	UpdatedAt     string
	UpdatedByName string

	TimeZone string
}

// ServeViewResourceAssignmentPage renders a read-only view of a single
// resource assignment for a group.
func (h *Handler) ServeViewResourceAssignmentPage(w http.ResponseWriter, r *http.Request) {
	_, _, _, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}

	gid := chi.URLParam(r, "id")
	groupOID, err := primitive.ObjectIDFromHex(gid)
	if err != nil {
		uierrors.RenderForbidden(w, r, "Bad group id.", httpnav.ResolveBackURL(r, "/groups"))
		return
	}

	assignHex := chi.URLParam(r, "assignmentID")
	assignID, err := primitive.ObjectIDFromHex(assignHex)
	if err != nil {
		uierrors.RenderForbidden(w, r, "Bad assignment id.", httpnav.ResolveBackURL(r, "/groups/"+gid+"/assign_resources"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()
	db := h.DB

	// Load group
	group, err := groupstore.New(db).GetByID(ctx, groupOID)
	if err == mongo.ErrNoDocuments {
		uierrors.RenderForbidden(w, r, "Group not found.", httpnav.ResolveBackURL(r, "/groups"))
		return
	}
	if err != nil {
		h.Log.Warn("group GetByID(view assignment)", zap.Error(err))
		uierrors.RenderForbidden(w, r, "A database error occurred.", httpnav.ResolveBackURL(r, "/groups"))
		return
	}

	canManage, policyErr := grouppolicy.CanManageGroup(ctx, db, r, group.ID, group.OrganizationID)
	if policyErr != nil {
		h.ErrLog.LogServerError(w, r, "database error checking group access", policyErr, "A database error occurred.", "/groups")
		return
	}
	if !canManage {
		uierrors.RenderForbidden(w, r, "You do not have access to this group.", httpnav.ResolveBackURL(r, "/groups"))
		return
	}

	// Load assignment
	asn, err := resourceassignstore.New(db).GetByID(ctx, assignID)
	if err == mongo.ErrNoDocuments {
		uierrors.RenderForbidden(w, r, "Assignment not found.", httpnav.ResolveBackURL(r, "/groups/"+gid+"/assign_resources"))
		return
	}
	if err != nil {
		h.Log.Warn("assignment GetByID(view)", zap.Error(err))
		uierrors.RenderForbidden(w, r, "A database error occurred.", httpnav.ResolveBackURL(r, "/groups/"+gid+"/assign_resources"))
		return
	}
	if asn.GroupID != group.ID {
		uierrors.RenderForbidden(w, r, "Assignment does not belong to this group.", httpnav.ResolveBackURL(r, "/groups/"+gid+"/assign_resources"))
		return
	}

	// Determine the organization's time zone for display of created/updated times.
	loc, tzLabel := resolveGroupLocation(ctx, db, group)

	// Load resource
	resStore := resourcestore.New(db)
	res, err := resStore.GetByID(ctx, asn.ResourceID)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			uierrors.RenderForbidden(w, r, "Resource not found.", httpnav.ResolveBackURL(r, "/groups/"+gid+"/assign_resources"))
			return
		}
		h.Log.Warn("resource FindOne(view assignment)", zap.Error(err))
		uierrors.RenderForbidden(w, r, "A database error occurred.", httpnav.ResolveBackURL(r, "/groups/"+gid+"/assign_resources"))
		return
	}

	// Build availability summary using the same helper as the list view.
	now := time.Now().In(loc)
	availSummary, _ := summarizeAssignmentAvailability(now, asn.VisibleFrom, asn.VisibleUntil)

	const dtLayout = "2006-01-02 15:04"
	createdAtStr := asn.CreatedAt.In(loc).Format(dtLayout)
	var updatedAtStr string
	if asn.UpdatedAt != nil && !asn.UpdatedAt.IsZero() {
		updatedAtStr = asn.UpdatedAt.In(loc).Format(dtLayout)
	}

	var visibleFromStr, visibleUntilStr string
	if asn.VisibleFrom != nil && !asn.VisibleFrom.IsZero() {
		visibleFromStr = asn.VisibleFrom.In(loc).Format(dtLayout)
	}
	if asn.VisibleUntil != nil && !asn.VisibleUntil.IsZero() {
		visibleUntilStr = asn.VisibleUntil.In(loc).Format(dtLayout)
	}

	back := r.URL.Query().Get("return")
	if back == "" {
		back = httpnav.ResolveBackURL(r, "/groups/"+group.ID.Hex()+"/assign_resources")
	}

	vm := viewResourceAssignmentPageVM{
		BaseVM:        viewdata.NewBaseVM(r, h.DB, "📚 View Resource Assignment", back),
		GroupID:       group.ID.Hex(),
		GroupName:     group.Name,
		AssignmentID:  asn.ID.Hex(),
		ResourceID:    res.ID.Hex(),
		ResourceTitle: res.Title,
		Subject:       res.Subject,
		Type:          res.Type,
		Availability:  availSummary,
		VisibleFrom:   visibleFromStr,
		VisibleUntil:  visibleUntilStr,
		Instructions:  asn.Instructions,
		CreatedAt:     createdAtStr,
		CreatedByName: asn.CreatedByName,
		UpdatedAt:     updatedAtStr,
		UpdatedByName: asn.UpdatedByName,
		TimeZone:      tzLabel,
	}

	templates.RenderAutoMap(w, r, "resource_assignment_view", nil, vm)
}
