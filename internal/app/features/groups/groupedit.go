// internal/app/features/groups/groupedit.go
package groups

import (
	"context"
	"fmt"
	"net/http"
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/policy/grouppolicy"
	groupstore "github.com/dalemusser/stratahub/internal/app/store/groups"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/formutil"
	"github.com/dalemusser/stratahub/internal/app/system/inputval"
	"github.com/dalemusser/stratahub/internal/app/system/normalize"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/httpnav"
	wafflemongo "github.com/dalemusser/waffle/pantry/mongo"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/dalemusser/waffle/pantry/text"
	"github.com/dalemusser/waffle/pantry/urlutil"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// editGroupInput defines validation rules for editing a group.
type editGroupInput struct {
	Name string `validate:"required,max=200" label:"Name"`
}

// ServeEditGroup renders the Edit Group page.
func (h *Handler) ServeEditGroup(w http.ResponseWriter, r *http.Request) {
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

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()
	db := h.DB

	grpStore := groupstore.New(db)
	group, err := grpStore.GetByID(ctx, groupOID)
	if err == mongo.ErrNoDocuments {
		uierrors.RenderForbidden(w, r, "Group not found.", httpnav.ResolveBackURL(r, "/groups"))
		return
	}
	if err != nil {
		h.Log.Warn("group GetByID", zap.Error(err))
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

	orgName := ""
	{
		var org models.Organization
		if err := db.Collection("organizations").FindOne(ctx, bson.M{"_id": group.OrganizationID}).Decode(&org); err != nil {
			if err == mongo.ErrNoDocuments {
				orgName = "(Deleted)"
			} else {
				h.ErrLog.LogServerError(w, r, "database error loading organization for group", err, "A database error occurred.", "/groups")
				return
			}
		} else {
			orgName = org.Name
		}
	}

	data := editGroupData{
		GroupID:          group.ID.Hex(),
		Name:             group.Name,
		Description:      group.Description,
		OrganizationID:   group.OrganizationID.Hex(),
		OrganizationName: orgName,
	}
	formutil.SetBase(&data.Base, r, "Edit Group", "/groups/"+group.ID.Hex()+"/manage")

	templates.Render(w, r, "group_edit", data)
}

// HandleEditGroup processes the Edit Group form submission.
func (h *Handler) HandleEditGroup(w http.ResponseWriter, r *http.Request) {
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
	if err := r.ParseForm(); err != nil {
		uierrors.RenderForbidden(w, r, "Bad request.", httpnav.ResolveBackURL(r, "/groups"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()
	db := h.DB

	grpStore := groupstore.New(db)
	group, err := grpStore.GetByID(ctx, groupOID)
	if err == mongo.ErrNoDocuments {
		uierrors.RenderForbidden(w, r, "Group not found.", httpnav.ResolveBackURL(r, "/groups"))
		return
	}
	if err != nil {
		h.Log.Warn("group GetByID", zap.Error(err))
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

	name := normalize.Name(r.FormValue("name"))
	desc := normalize.QueryParam(r.FormValue("description"))

	// Validate required fields using struct tags
	input := editGroupInput{Name: name}
	if result := inputval.Validate(input); result.HasErrors() {
		orgName := ""
		{
			var org models.Organization
			if err := db.Collection("organizations").FindOne(ctx, bson.M{"_id": group.OrganizationID}).Decode(&org); err != nil {
				if err == mongo.ErrNoDocuments {
					orgName = "(Deleted)"
				} else {
					h.ErrLog.LogServerError(w, r, "database error loading organization for group", err, "A database error occurred.", "/groups")
					return
				}
			} else {
				orgName = org.Name
			}
		}

		data := editGroupData{
			GroupID:          group.ID.Hex(),
			Name:             name,
			Description:      desc,
			OrganizationID:   group.OrganizationID.Hex(),
			OrganizationName: orgName,
		}
		formutil.SetBase(&data.Base, r, "Edit Group", "/groups/"+group.ID.Hex()+"/manage")
		data.SetError(result.First())
		templates.Render(w, r, "group_edit", data)
		return
	}

	update := bson.M{
		"name":        name,
		"name_ci":     text.Fold(name),
		"description": desc,
		"updated_at":  time.Now(),
	}

	if _, err := db.Collection("groups").UpdateOne(
		ctx,
		bson.M{"_id": groupOID},
		bson.M{"$set": update},
	); err != nil {
		var org models.Organization
		if orgErr := db.Collection("organizations").FindOne(ctx, bson.M{"_id": group.OrganizationID}).Decode(&org); orgErr != nil {
			if orgErr != mongo.ErrNoDocuments {
				h.ErrLog.LogServerError(w, r, "database error loading organization for group", orgErr, "A database error occurred.", "/groups")
				return
			}
		}

		msg := "Database error while updating the group."
		if wafflemongo.IsDup(err) {
			msg = "A group with that name already exists in this organization."
		}

		data := editGroupData{
			GroupID:          group.ID.Hex(),
			Name:             name,
			Description:      desc,
			OrganizationID:   group.OrganizationID.Hex(),
			OrganizationName: org.Name,
		}
		formutil.SetBase(&data.Base, r, "Edit Group", "/groups/"+group.ID.Hex()+"/manage")
		data.SetError(msg)
		templates.Render(w, r, "group_edit", data)
		return
	}

	ret := urlutil.SafeReturn(r.FormValue("return"), "", fmt.Sprintf("/groups/%s/manage", groupOID.Hex()))
	http.Redirect(w, r, ret, http.StatusSeeOther)
}
