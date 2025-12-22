// internal/app/features/groups/resourceassignactions.go
package groups

import (
	"context"
	"net/http"
	"strings"
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/policy/grouppolicy"
	groupstore "github.com/dalemusser/stratahub/internal/app/store/groups"
	orgstore "github.com/dalemusser/stratahub/internal/app/store/organizations"
	resourceassignstore "github.com/dalemusser/stratahub/internal/app/store/resourceassign"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/httpnav"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// HandleAssignResource handles POST /groups/{id}/assign_resources/add.
func (h *Handler) HandleAssignResource(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		uierrors.RenderForbidden(w, r, "Bad request.", httpnav.ResolveBackURL(r, "/groups"))
		return
	}

	gid := chi.URLParam(r, "id")
	groupOID, err := primitive.ObjectIDFromHex(gid)
	if err != nil {
		uierrors.RenderForbidden(w, r, "Bad group id.", httpnav.ResolveBackURL(r, "/groups"))
		return
	}
	resourceHex := r.FormValue("resourceID")
	resourceOID, err := primitive.ObjectIDFromHex(resourceHex)
	if err != nil {
		uierrors.RenderForbidden(w, r, "Bad resource id.", httpnav.ResolveBackURL(r, "/groups/"+gid+"/assign_resources"))
		return
	}

	visibleFromStr := strings.TrimSpace(r.FormValue("visible_from"))
	visibleUntilStr := strings.TrimSpace(r.FormValue("visible_until"))
	instructions := strings.TrimSpace(r.FormValue("instructions"))

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()
	db := h.DB

	group, err := groupstore.New(db).GetByID(ctx, groupOID)
	if err == mongo.ErrNoDocuments {
		uierrors.RenderForbidden(w, r, "Group not found.", httpnav.ResolveBackURL(r, "/groups"))
		return
	}
	if err != nil {
		h.Log.Warn("group GetByID(assign add)", zap.Error(err))
		uierrors.RenderForbidden(w, r, "A database error occurred.", httpnav.ResolveBackURL(r, "/groups"))
		return
	}

	canManage, policyErr := grouppolicy.CanManageGroup(ctx, db, r, group.ID, group.OrganizationID)
	if policyErr != nil {
		h.ErrLog.HTMXLogServerError(w, r, "database error checking group access", policyErr, "A database error occurred.", "/groups")
		return
	}
	if !canManage {
		uierrors.RenderForbidden(w, r, "You do not have access to this group.", httpnav.ResolveBackURL(r, "/groups"))
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
		uierrors.RenderForbidden(w, r, "A database error occurred.", httpnav.ResolveBackURL(r, "/groups/"+gid+"/assign_resources"))
		return
	}

	h.redirectAssignResources(w, r, gid)
}

// HandleRemoveAssignment handles POST /groups/{id}/assign_resources/remove.
func (h *Handler) HandleRemoveAssignment(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		uierrors.RenderForbidden(w, r, "Bad request.", httpnav.ResolveBackURL(r, "/groups"))
		return
	}

	gid := chi.URLParam(r, "id")
	groupOID, err := primitive.ObjectIDFromHex(gid)
	if err != nil {
		uierrors.RenderForbidden(w, r, "Bad group id.", httpnav.ResolveBackURL(r, "/groups"))
		return
	}
	assignHex := r.FormValue("assignmentID")
	assignID, err := primitive.ObjectIDFromHex(assignHex)
	if err != nil {
		uierrors.RenderForbidden(w, r, "Bad assignment id.", httpnav.ResolveBackURL(r, "/groups/"+gid+"/assign_resources"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()
	db := h.DB

	group, err := groupstore.New(db).GetByID(ctx, groupOID)
	if err == mongo.ErrNoDocuments {
		uierrors.RenderForbidden(w, r, "Group not found.", httpnav.ResolveBackURL(r, "/groups"))
		return
	}
	if err != nil {
		h.Log.Warn("group GetByID(assign remove)", zap.Error(err))
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

	asn, err := resourceassignstore.New(db).GetByID(ctx, assignID)
	if err == mongo.ErrNoDocuments {
		uierrors.RenderForbidden(w, r, "Assignment not found.", httpnav.ResolveBackURL(r, "/groups/"+gid+"/assign_resources"))
		return
	}
	if err != nil {
		h.Log.Warn("assignment GetByID(remove)", zap.Error(err))
		uierrors.RenderForbidden(w, r, "A database error occurred.", httpnav.ResolveBackURL(r, "/groups/"+gid+"/assign_resources"))
		return
	}
	if asn.GroupID != group.ID {
		uierrors.RenderForbidden(w, r, "Assignment does not belong to this group.", httpnav.ResolveBackURL(r, "/groups/"+gid+"/assign_resources"))
		return
	}

	if err := resourceassignstore.New(db).Delete(ctx, assignID); err != nil {
		h.Log.Warn("resource assign delete", zap.Error(err))
		uierrors.RenderForbidden(w, r, "A database error occurred.", httpnav.ResolveBackURL(r, "/groups/"+gid+"/assign_resources"))
		return
	}

	h.redirectAssignResources(w, r, gid)
}
