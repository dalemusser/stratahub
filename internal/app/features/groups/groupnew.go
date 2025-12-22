// internal/app/features/groups/groupnew.go
package groups

import (
	"context"
	"net/http"
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	userstore "github.com/dalemusser/stratahub/internal/app/store/users"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/formutil"
	"github.com/dalemusser/stratahub/internal/app/system/inputval"
	"github.com/dalemusser/stratahub/internal/app/system/navigation"
	"github.com/dalemusser/stratahub/internal/app/system/normalize"
	"github.com/dalemusser/stratahub/internal/app/system/orgutil"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/txn"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/httpnav"
	wafflemongo "github.com/dalemusser/waffle/pantry/mongo"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/dalemusser/waffle/pantry/text"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

// createGroupInput defines validation rules for creating a group.
type createGroupInput struct {
	Name string `validate:"required,max=200" label:"Name"`
}

// ServeNewGroup renders the Add Group page.
func (h *Handler) ServeNewGroup(w http.ResponseWriter, r *http.Request) {
	role, _, uid, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}
	if role != "admin" && role != "leader" {
		uierrors.RenderForbidden(w, r, "You do not have access to create groups.", httpnav.ResolveBackURL(r, "/groups"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()
	db := h.DB

	var data newGroupData
	formutil.SetBase(&data.Base, r, "Add Group", "/groups")

	if role == "admin" {
		orgOpts, orgIDs, err := orgutil.LoadActiveOrgOptions(ctx, db)
		if err != nil {
			h.Log.Warn("LoadActiveOrgOptions", zap.Error(err))
			uierrors.RenderForbidden(w, r, "A database error occurred.", httpnav.ResolveBackURL(r, "/groups"))
			return
		}
		leaders, err := orgutil.LoadActiveLeaders(ctx, db, orgIDs)
		if err != nil {
			h.Log.Warn("LoadActiveLeaders", zap.Error(err))
			uierrors.RenderForbidden(w, r, "A database error occurred.", httpnav.ResolveBackURL(r, "/groups"))
			return
		}
		data.Organizations = orgOpts
		data.Leaders = leaders
	} else {
		// Leader: use their org as the fixed org for the new group; auto-assign them later.
		usrStore := userstore.New(db)
		user, err := usrStore.GetByID(ctx, uid)
		if err == mongo.ErrNoDocuments {
			uierrors.RenderForbidden(w, r, "User not found.", httpnav.ResolveBackURL(r, "/groups"))
			return
		}
		if err != nil {
			h.Log.Warn("user GetByID", zap.Error(err))
			uierrors.RenderForbidden(w, r, "A database error occurred.", httpnav.ResolveBackURL(r, "/groups"))
			return
		}
		if user.OrganizationID == nil {
			uierrors.RenderForbidden(w, r, "Your account is not linked to an organization.", httpnav.ResolveBackURL(r, "/groups"))
			return
		}

		var org models.Organization
		if err := db.Collection("organizations").FindOne(ctx, bson.M{"_id": *user.OrganizationID}).Decode(&org); err != nil {
			if err == mongo.ErrNoDocuments {
				h.Log.Warn("organization not found for leader (may have been deleted)",
					zap.String("user_id", uid.Hex()),
					zap.String("org_id", user.OrganizationID.Hex()))
				data.LeaderOrgName = "(Deleted)"
			} else {
				h.ErrLog.LogServerError(w, r, "database error loading organization for leader", err, "A database error occurred.", "/groups")
				return
			}
		} else {
			data.LeaderOrgName = org.Name
		}

		data.LeaderOrgID = user.OrganizationID.Hex()
	}

	templates.Render(w, r, "group_new", data)
}

// HandleCreateGroup processes the Add Group form submission.
func (h *Handler) HandleCreateGroup(w http.ResponseWriter, r *http.Request) {
	role, _, uid, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}
	if role != "admin" && role != "leader" {
		uierrors.RenderForbidden(w, r, "You do not have access to create groups.", httpnav.ResolveBackURL(r, "/groups"))
		return
	}
	if err := r.ParseForm(); err != nil {
		uierrors.RenderForbidden(w, r, "Bad request.", httpnav.ResolveBackURL(r, "/groups"))
		return
	}

	name := normalize.Name(r.FormValue("name"))
	desc := normalize.QueryParam(r.FormValue("description"))

	// Validate required fields using struct tags
	input := createGroupInput{Name: name}
	if result := inputval.Validate(input); result.HasErrors() {
		h.reRenderNewWithError(w, r, newGroupData{
			Name:           name,
			Description:    desc,
			OrgHex:         normalize.QueryParam(r.FormValue("orgID")),
			SelectedLeader: toSet(r.Form["leaderIDs"]),
		}, result.First())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()
	db := h.DB

	// Resolve org
	var orgID primitive.ObjectID
	var err error
	if role == "admin" {
		orgHex := normalize.QueryParam(r.FormValue("orgID"))
		orgID, err = primitive.ObjectIDFromHex(orgHex)
		if err != nil {
			h.reRenderNewWithError(w, r, newGroupData{
				Name:           name,
				Description:    desc,
				OrgHex:         orgHex,
				SelectedLeader: toSet(r.Form["leaderIDs"]),
			}, "Please select an organization.")
			return
		}
	} else {
		usrStore := userstore.New(db)
		user, err := usrStore.GetByID(ctx, uid)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				uierrors.RenderForbidden(w, r, "User not found.", httpnav.ResolveBackURL(r, "/groups"))
				return
			}
			h.Log.Warn("leader org resolve failed", zap.Error(err))
			uierrors.RenderForbidden(w, r, "A database error occurred.", httpnav.ResolveBackURL(r, "/groups"))
			return
		}
		if user.OrganizationID == nil {
			uierrors.RenderForbidden(w, r, "Your account is not linked to an organization.", httpnav.ResolveBackURL(r, "/groups"))
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
			lid, e := primitive.ObjectIDFromHex(normalize.QueryParam(hex))
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
		NameCI:         text.Fold(name),
		Description:    desc,
		OrganizationID: orgID,
		Status:         "active",
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	// Use transaction for atomic group creation and leader assignment.
	if err := txn.Run(ctx, db, h.Log, func(ctx context.Context) error {
		// 1) Insert the group.
		if _, err := db.Collection("groups").InsertOne(ctx, doc); err != nil {
			return err
		}

		// 2) Write leader memberships into group_memberships.
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
				if _, err := col.BulkWrite(ctx, writes, options.BulkWrite().SetOrdered(false)); err != nil {
					return err
				}
			}
		}
		return nil
	}); err != nil {
		msg := "Failed to create group."
		if wafflemongo.IsDup(err) {
			msg = "A group with that name already exists in the selected organization."
		}
		h.reRenderNewWithError(w, r, newGroupData{
			Name:           name,
			Description:    desc,
			OrgHex:         normalize.QueryParam(r.FormValue("orgID")),
			SelectedLeader: toSet(r.Form["leaderIDs"]),
		}, msg)
		return
	}

	// Success redirect
	ret := navigation.SafeBackURL(r, navigation.GroupsBackURL)
	http.Redirect(w, r, ret, http.StatusSeeOther)
}

// reRenderNewWithError re-renders the Add Group page with a validation error
// and previously posted values.
func (h *Handler) reRenderNewWithError(w http.ResponseWriter, r *http.Request, data newGroupData, msg string) {
	formutil.SetBase(&data.Base, r, "Add Group", "/groups")
	data.SetError(msg)

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()
	db := h.DB

	if data.Role == "admin" {
		orgOpts, _, err := orgutil.LoadActiveOrgOptions(ctx, db)
		if err == nil {
			data.Organizations = orgOpts
		} else {
			h.Log.Warn("LoadActiveOrgOptions (re-render)", zap.Error(err))
		}
		leaders, err := orgutil.LoadActiveLeaders(ctx, db, nil)
		if err == nil {
			data.Leaders = leaders
		} else {
			h.Log.Warn("LoadActiveLeaders (re-render)", zap.Error(err))
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
			if orgErr := db.Collection("organizations").FindOne(ctx, bson.M{"_id": *user.OrganizationID}).Decode(&org); orgErr != nil {
				if orgErr == mongo.ErrNoDocuments {
					h.Log.Warn("organization not found for leader (may have been deleted)",
						zap.String("user_id", uid.Hex()),
						zap.String("org_id", user.OrganizationID.Hex()))
					data.LeaderOrgName = "(Deleted)"
				} else {
					h.ErrLog.LogServerError(w, r, "database error loading organization for leader", orgErr, "A database error occurred.", "/groups")
					return
				}
			} else {
				data.LeaderOrgName = org.Name
			}
			data.LeaderOrgID = user.OrganizationID.Hex()
		} else if err != nil {
			h.ErrLog.LogServerError(w, r, "database error loading user", err, "A database error occurred.", "/groups")
			return
		}
	}

	templates.Render(w, r, "group_new", data)
}
