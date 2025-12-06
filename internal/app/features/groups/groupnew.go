// internal/app/features/groups/groupnew.go
package groups

import (
	"context"
	"net/http"
	"strings"
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	userstore "github.com/dalemusser/stratahub/internal/app/store/users"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/templates"
	mongodb "github.com/dalemusser/waffle/toolkit/db/mongodb"
	textfold "github.com/dalemusser/waffle/toolkit/text/textfold"
	nav "github.com/dalemusser/waffle/toolkit/ui/nav"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

// ServeNewGroup renders the Add Group page.
func (h *Handler) ServeNewGroup(w http.ResponseWriter, r *http.Request) {
	role, uname, uid, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}
	if role != "admin" && role != "leader" {
		uierrors.RenderForbidden(w, r, "You do not have access to create groups.", nav.ResolveBackURL(r, "/groups"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), metaShortTimeout)
	defer cancel()
	db := h.DB

	data := newGroupData{
		Title:       "Add Group",
		IsLoggedIn:  true,
		Role:        role,
		UserName:    uname,
		BackURL:     nav.ResolveBackURL(r, "/groups"),
		CurrentPath: nav.CurrentPath(r),
	}

	if role == "admin" {
		orgOpts, orgIDs, err := loadActiveOrgs(ctx, db)
		if err != nil {
			h.Log.Warn("loadActiveOrgs", zap.Error(err))
			uierrors.RenderForbidden(w, r, "A database error occurred.", nav.ResolveBackURL(r, "/groups"))
			return
		}
		leaders, err := loadActiveLeaders(ctx, db, orgIDs)
		if err != nil {
			h.Log.Warn("loadActiveLeaders", zap.Error(err))
			uierrors.RenderForbidden(w, r, "A database error occurred.", nav.ResolveBackURL(r, "/groups"))
			return
		}
		data.Organizations = orgOpts
		data.Leaders = leaders
	} else {
		// Leader: use their org as the fixed org for the new group; auto-assign them later.
		usrStore := userstore.New(db)
		user, err := usrStore.GetByID(ctx, uid)
		if err == mongo.ErrNoDocuments {
			uierrors.RenderForbidden(w, r, "User not found.", nav.ResolveBackURL(r, "/groups"))
			return
		}
		if err != nil {
			h.Log.Warn("user GetByID", zap.Error(err))
			uierrors.RenderForbidden(w, r, "A database error occurred.", nav.ResolveBackURL(r, "/groups"))
			return
		}
		if user.OrganizationID == nil {
			uierrors.RenderForbidden(w, r, "Your account is not linked to an organization.", nav.ResolveBackURL(r, "/groups"))
			return
		}

		var org models.Organization
		_ = db.Collection("organizations").FindOne(ctx, bson.M{"_id": *user.OrganizationID}).Decode(&org)

		data.LeaderOrgID = user.OrganizationID.Hex()
		data.LeaderOrgName = org.Name
	}

	templates.Render(w, r, "group_new", data)
}

// HandleCreateGroup processes the Add Group form submission.
func (h *Handler) HandleCreateGroup(w http.ResponseWriter, r *http.Request) {
	role, uname, uid, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}
	if role != "admin" && role != "leader" {
		uierrors.RenderForbidden(w, r, "You do not have access to create groups.", nav.ResolveBackURL(r, "/groups"))
		return
	}
	if err := r.ParseForm(); err != nil {
		uierrors.RenderForbidden(w, r, "Bad request.", nav.ResolveBackURL(r, "/groups"))
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	desc := strings.TrimSpace(r.FormValue("description"))

	// Inline validation: Name required
	if name == "" {
		h.reRenderNewWithError(w, r, role, uname, newGroupData{
			Name:           name,
			Description:    desc,
			OrgHex:         strings.TrimSpace(r.FormValue("orgID")),
			SelectedLeader: toSet(r.Form["leaderIDs"]),
			Error:          "Name is required.",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), metaMedTimeout)
	defer cancel()
	db := h.DB

	// Resolve org
	var orgID primitive.ObjectID
	var err error
	if role == "admin" {
		orgHex := strings.TrimSpace(r.FormValue("orgID"))
		orgID, err = primitive.ObjectIDFromHex(orgHex)
		if err != nil {
			h.reRenderNewWithError(w, r, role, uname, newGroupData{
				Name:           name,
				Description:    desc,
				OrgHex:         orgHex,
				SelectedLeader: toSet(r.Form["leaderIDs"]),
				Error:          "Please select an organization.",
			})
			return
		}
	} else {
		usrStore := userstore.New(db)
		user, err := usrStore.GetByID(ctx, uid)
		if err != nil || user.OrganizationID == nil {
			h.Log.Warn("leader org resolve failed", zap.Error(err))
			uierrors.RenderForbidden(w, r, "Your account is not linked to an organization.", nav.ResolveBackURL(r, "/groups"))
			return
		}
		orgID = *user.OrganizationID
	}

	// Leaders
	var leaders []primitive.ObjectID
	if role == "leader" {
		leaders = []primitive.ObjectID{uid}
	} else {
		posted := r.Form["leaderIDs"] // optional
		for _, hex := range posted {
			lid, e := primitive.ObjectIDFromHex(strings.TrimSpace(hex))
			if e != nil {
				continue
			}
			cnt, _ := db.Collection("users").CountDocuments(ctx, bson.M{
				"_id":             lid,
				"role":            "leader",
				"organization_id": orgID,
			})
			if cnt > 0 {
				leaders = append(leaders, lid)
			}
		}
	}

	now := time.Now()

	doc := models.Group{
		ID:             primitive.NewObjectID(),
		Name:           name,
		NameCI:         textfold.Fold(name),
		Description:    desc,
		OrganizationID: orgID,
		Status:         "active",
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if _, err := db.Collection("groups").InsertOne(ctx, doc); err != nil {
		msg := "Insert error."
		if mongodb.IsDup(err) {
			msg = "A group with that name already exists in the selected organization."
		}
		h.reRenderNewWithError(w, r, role, uname, newGroupData{
			Name:           name,
			Description:    desc,
			OrgHex:         strings.TrimSpace(r.FormValue("orgID")),
			SelectedLeader: toSet(r.Form["leaderIDs"]),
			Error:          msg,
		})
		return
	}

	// Write leader memberships into group_memberships.
	if len(leaders) > 0 {
		var writes []mongo.WriteModel
		col := db.Collection("group_memberships")
		for _, lid := range leaders {
			writes = append(writes, mongo.NewInsertOneModel().SetDocument(bson.M{
				"group_id":   doc.ID,
				"user_id":    lid,
				"org_id":     orgID,
				"role":       "leader",
				"created_at": now,
			}))
		}
		if len(writes) > 0 {
			_, _ = col.BulkWrite(ctx, writes, options.BulkWrite().SetOrdered(false))
		}
	}

	// Success redirect
	if ret := r.FormValue("return"); ret != "" && strings.HasPrefix(ret, "/") {
		http.Redirect(w, r, ret, http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/groups", http.StatusSeeOther)
}

// reRenderNewWithError re-renders the Add Group page with a validation error
// and previously posted values.
func (h *Handler) reRenderNewWithError(w http.ResponseWriter, r *http.Request, role, uname string, data newGroupData) {
	data.Title = "Add Group"
	data.IsLoggedIn = true
	data.Role = role
	data.UserName = uname
	data.BackURL = nav.ResolveBackURL(r, "/groups")
	data.CurrentPath = nav.CurrentPath(r)

	ctx, cancel := context.WithTimeout(r.Context(), metaShortTimeout)
	defer cancel()
	db := h.DB

	if role == "admin" {
		orgOpts, _, err := loadActiveOrgs(ctx, db)
		if err == nil {
			data.Organizations = orgOpts
		} else {
			h.Log.Warn("loadActiveOrgs (re-render)", zap.Error(err))
		}
		leaders, err := loadActiveLeaders(ctx, db, nil)
		if err == nil {
			data.Leaders = leaders
		} else {
			h.Log.Warn("loadActiveLeaders (re-render)", zap.Error(err))
		}
	} else {
		_, _, uid, ok := authz.UserCtx(r)
		if !ok {
			uierrors.RenderUnauthorized(w, r, "/login")
			return
		}
		usrStore := userstore.New(db)
		user, err := usrStore.GetByID(ctx, uid)
		if err == nil && user.OrganizationID != nil {
			var org models.Organization
			_ = db.Collection("organizations").FindOne(ctx, bson.M{"_id": *user.OrganizationID}).Decode(&org)
			data.LeaderOrgID = user.OrganizationID.Hex()
			data.LeaderOrgName = org.Name
		} else if err != nil {
			h.Log.Warn("user GetByID (re-render)", zap.Error(err))
		}
	}

	templates.Render(w, r, "group_new", data)
}
