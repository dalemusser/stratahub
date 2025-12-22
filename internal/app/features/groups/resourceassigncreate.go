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
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/dalemusser/waffle/pantry/httpnav"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// assignResourcePageVM is the view model for the page-based Assign Resource flow.
type assignResourcePageVM struct {
	Title       string
	IsLoggedIn  bool
	Role        string
	UserName    string
	CurrentPath string

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
	BackURL  string
}

// ServeAssignResourcePage renders the full page used to configure a new
// resource assignment (available_from / available_until / instructions) after
// a resource has been selected from the Available list.
func (h *Handler) ServeAssignResourcePage(w http.ResponseWriter, r *http.Request) {
	role, uname, _, ok := authz.UserCtx(r)
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

	var res models.Resource
	if err := db.Collection("resources").FindOne(ctx, bson.M{"_id": resourceOID}).Decode(&res); err != nil {
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
		Title:         "📚 Assign Resource",
		IsLoggedIn:    true,
		Role:          role,
		UserName:      uname,
		CurrentPath:   httpnav.CurrentPath(r),
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
		BackURL:       back,
	}

	templates.RenderAutoMap(w, r, "resource_assignment_create", nil, vm)
}
