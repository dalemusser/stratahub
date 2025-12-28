// internal/app/features/groups/resourceassigncreate.go
package groups

import (
	"context"
	"net/http"
	"strings"
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/policy/grouppolicy"
	groupstore "github.com/dalemusser/stratahub/internal/app/store/groups"
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

// assignResourcePageVM is the view model for the page-based Assign Resource flow.
type assignResourcePageVM struct {
	viewdata.BaseVM

	GroupID   string
	GroupName string

	ResourceID    string
	ResourceTitle string
	ResourceType  string
	Subject       string

	VisibleFrom  string // for type="datetime-local"
	VisibleUntil string
	Instructions string

	TimeZone string
}

// ServeAssignResourcePage renders the full page used to configure a new
// resource assignment (available_from / available_until / instructions) after
// a resource has been selected from the Available list.
func (h *Handler) ServeAssignResourcePage(w http.ResponseWriter, r *http.Request) {
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

	// Resource ID can come from query or form, depending on navigation path.
	resourceHex := r.URL.Query().Get("resourceID")
	if resourceHex == "" {
		resourceHex = r.FormValue("resourceID")
	}
	resourceOID, err := primitive.ObjectIDFromHex(strings.TrimSpace(resourceHex))
	if err != nil {
		uierrors.RenderForbidden(w, r, "Bad resource id.", httpnav.ResolveBackURL(r, "/groups/"+gid+"/assign_resources"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()
	db := h.DB

	group, err := groupstore.New(db).GetByID(ctx, groupOID)
	if err == mongo.ErrNoDocuments {
		uierrors.RenderForbidden(w, r, "Group not found.", httpnav.ResolveBackURL(r, "/groups"))
		return
	}
	if err != nil {
		h.Log.Warn("group GetByID(assign page)", zap.Error(err))
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

	// Determine the organization's time zone and current time in that zone.
	loc, tzLabel := resolveGroupLocation(ctx, db, group)
	now := time.Now().In(loc)
	visibleFromStr := now.Format("2006-01-02T15:04")

	resStore := resourcestore.New(db)
	res, err := resStore.GetByID(ctx, resourceOID)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			uierrors.RenderForbidden(w, r, "Resource not found.", httpnav.ResolveBackURL(r, "/groups/"+gid+"/assign_resources"))
			return
		}
		h.Log.Warn("resource FindOne(assign page)", zap.Error(err))
		uierrors.RenderForbidden(w, r, "A database error occurred.", httpnav.ResolveBackURL(r, "/groups/"+gid+"/assign_resources"))
		return
	}

	back := r.URL.Query().Get("return")
	if back == "" {
		back = "/groups/" + group.ID.Hex() + "/assign_resources"
	}

	vm := assignResourcePageVM{
		BaseVM:        viewdata.NewBaseVM(r, h.DB, "📚 Assign Resource", back),
		GroupID:       group.ID.Hex(),
		GroupName:     group.Name,
		ResourceID:    res.ID.Hex(),
		ResourceTitle: res.Title,
		ResourceType:  res.Type,
		Subject:       res.Subject,
		VisibleFrom:   visibleFromStr,
		VisibleUntil:  "",
		Instructions:  res.DefaultInstructions,
		TimeZone:      tzLabel,
	}

	templates.RenderAutoMap(w, r, "resource_assignment_create", nil, vm)
}
