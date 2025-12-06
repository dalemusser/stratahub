// internal/app/features/groups/groupedit.go
package groups

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/policy/grouppolicy"
	groupstore "github.com/dalemusser/stratahub/internal/app/store/groups"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/templates"
	mongodb "github.com/dalemusser/waffle/toolkit/db/mongodb"
	textfold "github.com/dalemusser/waffle/toolkit/text/textfold"
	nav "github.com/dalemusser/waffle/toolkit/ui/nav"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// ServeEditGroup renders the Edit Group page.
func (h *Handler) ServeEditGroup(w http.ResponseWriter, r *http.Request) {
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

	grpStore := groupstore.New(db)
	group, err := grpStore.GetByID(ctx, groupOID)
	if err == mongo.ErrNoDocuments {
		uierrors.RenderForbidden(w, r, "Group not found.", nav.ResolveBackURL(r, "/groups"))
		return
	}
	if err != nil {
		h.Log.Warn("group GetByID", zap.Error(err))
		uierrors.RenderForbidden(w, r, "A database error occurred.", nav.ResolveBackURL(r, "/groups"))
		return
	}

	if !grouppolicy.CanManageGroup(ctx, db, r, group.ID) {
		uierrors.RenderForbidden(w, r, "You do not have access to this group.", nav.ResolveBackURL(r, "/groups"))
		return
	}

	var org models.Organization
	_ = db.Collection("organizations").FindOne(ctx, bson.M{"_id": group.OrganizationID}).Decode(&org)

	templates.Render(w, r, "group_edit", editGroupData{
		Title:            "Edit Group",
		IsLoggedIn:       true,
		Role:             role,
		UserName:         uname,
		GroupID:          group.ID.Hex(),
		Name:             group.Name,
		Description:      group.Description,
		OrganizationID:   group.OrganizationID.Hex(),
		OrganizationName: org.Name,
		BackURL:          nav.ResolveBackURL(r, "/groups/"+group.ID.Hex()+"/manage"),
		CurrentPath:      nav.CurrentPath(r),
	})
}

// HandleEditGroup processes the Edit Group form submission.
func (h *Handler) HandleEditGroup(w http.ResponseWriter, r *http.Request) {
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
	if err := r.ParseForm(); err != nil {
		uierrors.RenderForbidden(w, r, "Bad request.", nav.ResolveBackURL(r, "/groups"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), metaMedTimeout)
	defer cancel()
	db := h.DB

	grpStore := groupstore.New(db)
	group, err := grpStore.GetByID(ctx, groupOID)
	if err == mongo.ErrNoDocuments {
		uierrors.RenderForbidden(w, r, "Group not found.", nav.ResolveBackURL(r, "/groups"))
		return
	}
	if err != nil {
		h.Log.Warn("group GetByID", zap.Error(err))
		uierrors.RenderForbidden(w, r, "A database error occurred.", nav.ResolveBackURL(r, "/groups"))
		return
	}

	if !grouppolicy.CanManageGroup(ctx, db, r, group.ID) {
		uierrors.RenderForbidden(w, r, "You do not have access to this group.", nav.ResolveBackURL(r, "/groups"))
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	desc := strings.TrimSpace(r.FormValue("description"))

	if name == "" {
		var org models.Organization
		_ = db.Collection("organizations").FindOne(ctx, bson.M{"_id": group.OrganizationID}).Decode(&org)

		templates.Render(w, r, "group_edit", editGroupData{
			Title:            "Edit Group",
			IsLoggedIn:       true,
			Role:             role,
			UserName:         uname,
			GroupID:          group.ID.Hex(),
			Name:             name,
			Description:      desc,
			OrganizationID:   group.OrganizationID.Hex(),
			OrganizationName: org.Name,
			Error:            "Name is required.",
			BackURL:          nav.ResolveBackURL(r, "/groups/"+group.ID.Hex()+"/manage"),
			CurrentPath:      nav.CurrentPath(r),
		})
		return
	}

	update := bson.M{
		"name":        name,
		"name_ci":     textfold.Fold(name),
		"description": desc,
		"updated_at":  time.Now(),
	}

	if _, err := db.Collection("groups").UpdateOne(
		ctx,
		bson.M{"_id": groupOID},
		bson.M{"$set": update},
	); err != nil {
		var org models.Organization
		_ = db.Collection("organizations").FindOne(ctx, bson.M{"_id": group.OrganizationID}).Decode(&org)

		msg := "Database error while updating the group."
		if mongodb.IsDup(err) {
			msg = "A group with that name already exists in this organization."
		}

		templates.Render(w, r, "group_edit", editGroupData{
			Title:            "Edit Group",
			IsLoggedIn:       true,
			Role:             role,
			UserName:         uname,
			GroupID:          group.ID.Hex(),
			Name:             name,
			Description:      desc,
			OrganizationID:   group.OrganizationID.Hex(),
			OrganizationName: org.Name,
			Error:            msg,
			BackURL:          nav.ResolveBackURL(r, "/groups/"+group.ID.Hex()+"/manage"),
			CurrentPath:      nav.CurrentPath(r),
		})
		return
	}

	if ret := r.FormValue("return"); ret != "" && strings.HasPrefix(ret, "/") {
		http.Redirect(w, r, ret, http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/groups/%s/manage", groupOID.Hex()), http.StatusSeeOther)
}
