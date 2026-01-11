// internal/app/features/groups/manage.go
package groups

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/policy/grouppolicy"
	groupstore "github.com/dalemusser/stratahub/internal/app/store/groups"
	membershipstore "github.com/dalemusser/stratahub/internal/app/store/memberships"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/httpnav"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

// ServeManageGroup renders the main Manage Group page.
func (h *Handler) ServeManageGroup(w http.ResponseWriter, r *http.Request) {
	gid := chi.URLParam(r, "id")

	groupOID, err := primitive.ObjectIDFromHex(gid)
	if err != nil {
		uierrors.RenderBadRequest(w, r, "Invalid group ID.", "/groups")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()
	db := h.DB

	grpStore := groupstore.New(db)
	grp, err := grpStore.GetByID(ctx, groupOID)
	if err == mongo.ErrNoDocuments {
		uierrors.RenderNotFound(w, r, "Group not found.", "/groups")
		return
	}
	if err != nil {
		h.ErrLog.LogServerError(w, r, "database error loading group", err, "A database error occurred.", "/groups")
		return
	}
	canManage, policyErr := grouppolicy.CanManageGroup(ctx, db, r, grp.ID, grp.OrganizationID)
	if policyErr != nil {
		h.ErrLog.LogServerError(w, r, "database error checking group access", policyErr, "A database error occurred.", "/groups")
		return
	}
	if !canManage {
		uierrors.RenderForbidden(w, r, "You don't have permission to manage this group.", httpnav.ResolveBackURL(r, "/groups"))
		return
	}

	data, err := h.buildPageData(r, gid, "", "", "")
	if err != nil {
		h.ErrLog.LogServerError(w, r, "error building group page data", err, "Failed to load group data.", "/groups")
		return
	}

	templates.Render(w, r, "group_manage", data)
}

// buildPageData assembles the ManagePageData for a given group and search window.
func (h *Handler) buildPageData(r *http.Request, gid, q, after, before string) (ManagePageData, error) {
	_, _, _, ok := authz.UserCtx(r)
	if !ok {
		return ManagePageData{}, fmt.Errorf("unauthorized")
	}

	groupOID, err := primitive.ObjectIDFromHex(gid)
	if err != nil {
		return ManagePageData{}, fmt.Errorf("invalid id")
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()
	db := h.DB

	group, err := groupstore.New(db).GetByID(ctx, groupOID)
	if err != nil {
		return ManagePageData{}, fmt.Errorf("not found")
	}

	usersColl := db.Collection("users")

	leadIDs, err := h.fetchMemberIDs(ctx, db, group.ID, "leader")
	if err != nil {
		return ManagePageData{}, err
	}
	memberIDs, err := h.fetchMemberIDs(ctx, db, group.ID, "member")
	if err != nil {
		return ManagePageData{}, err
	}

	currentLeads, err := h.fetchUserItemsByIDs(ctx, usersColl, leadIDs)
	if err != nil {
		return ManagePageData{}, err
	}
	currentMembers, err := h.fetchUserItemsByIDs(ctx, usersColl, memberIDs)
	if err != nil {
		return ManagePageData{}, err
	}

	// Possible leaders: active leaders in org not already in leads.
	leadFilter := bson.M{
		"organization_id": group.OrganizationID,
		"role":            "leader",
		"status":          "active",
	}
	if len(leadIDs) > 0 {
		leadFilter["_id"] = bson.M{"$nin": leadIDs}
	}
	possibleLeads, err := h.fetchUserItems(ctx, usersColl, leadFilter)
	if err != nil {
		return ManagePageData{}, err
	}

	sortUsers := func(s []UserItem) {
		sort.SliceStable(s, func(i, j int) bool {
			ni, nj := strings.ToLower(s[i].FullName), strings.ToLower(s[j].FullName)
			if ni == nj {
				if s[i].FullName == s[j].FullName {
					return s[i].LoginID < s[j].LoginID
				}
				return s[i].FullName < s[j].FullName
			}
			return ni < nj
		})
	}

	sortUsers(currentLeads)
	sortUsers(currentMembers)
	sortUsers(possibleLeads)

	avail, shown, total, nextCur, prevCur, hasNext, hasPrev, err :=
		h.fetchAvailablePaged(ctx, group.OrganizationID, group.ID, q, after, before)
	if err != nil {
		return ManagePageData{}, err
	}

	var orgName string
	{
		var o models.Organization
		if err := db.Collection("organizations").
			FindOne(ctx, bson.M{"_id": group.OrganizationID}).
			Decode(&o); err != nil {
			if err == mongo.ErrNoDocuments {
				orgName = "(Deleted)"
			} else {
				h.Log.Error("database error loading organization for group", zap.Error(err), zap.String("group_id", group.ID.Hex()))
				return ManagePageData{}, fmt.Errorf("database error")
			}
		} else {
			orgName = o.Name
		}
	}

	return ManagePageData{
		BaseVM:           viewdata.NewBaseVM(r, db, "Manage Group", "/groups"),
		GroupID:          group.ID.Hex(),
		GroupName:        group.Name,
		GroupDescription: group.Description,
		OrganizationName: orgName,
		CurrentLeaders:   currentLeads,
		CurrentMembers:   currentMembers,
		PossibleLeaders:  possibleLeads,
		AvailableMembers: avail,
		AvailableShown:   shown,
		AvailableTotal:   total,
		Query:            q,
		CurrentAfter:     after,
		CurrentBefore:    before,
		NextCursor:       nextCur,
		PrevCursor:       prevCur,
		HasNext:          hasNext,
		HasPrev:          hasPrev,
	}, nil
}

// fetchMemberIDs returns all user IDs in group_memberships for a given group/role.
func (h *Handler) fetchMemberIDs(ctx context.Context, db *mongo.Database, groupID primitive.ObjectID, role string) ([]primitive.ObjectID, error) {
	memStore := membershipstore.New(db)
	memberships, err := memStore.ListByGroup(ctx, groupID, role)
	if err != nil {
		h.Log.Error("database error finding group memberships", zap.Error(err), zap.String("group_id", groupID.Hex()), zap.String("role", role))
		return nil, err
	}

	ids := make([]primitive.ObjectID, 0, len(memberships))
	for _, m := range memberships {
		ids = append(ids, m.UserID)
	}
	return ids, nil
}

// fetchUserItemsByIDs returns basic user info for a set of IDs.
func (h *Handler) fetchUserItemsByIDs(ctx context.Context, col *mongo.Collection, ids []primitive.ObjectID) ([]UserItem, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	cur, err := col.Find(
		ctx,
		bson.M{"_id": bson.M{"$in": ids}},
		options.Find().SetProjection(bson.M{"full_name": 1, "login_id": 1}),
	)
	if err != nil {
		h.Log.Error("database error finding users by IDs", zap.Error(err))
		return nil, err
	}
	defer cur.Close(ctx)

	var users []struct {
		ID       primitive.ObjectID `bson:"_id"`
		FullName string             `bson:"full_name"`
		LoginID  *string            `bson:"login_id"`
	}
	if err := cur.All(ctx, &users); err != nil {
		h.Log.Error("database error decoding users", zap.Error(err))
		return nil, err
	}

	out := make([]UserItem, len(users))
	for i, u := range users {
		loginID := ""
		if u.LoginID != nil {
			loginID = *u.LoginID
		}
		out[i] = UserItem{
			ID:       u.ID.Hex(),
			FullName: u.FullName,
			LoginID:  loginID,
		}
	}
	return out, nil
}

// fetchUserItems returns basic user info matching a filter.
func (h *Handler) fetchUserItems(ctx context.Context, col *mongo.Collection, filter bson.M) ([]UserItem, error) {
	cur, err := col.Find(
		ctx,
		filter,
		options.Find().SetProjection(bson.M{"_id": 1, "full_name": 1, "login_id": 1}),
	)
	if err != nil {
		h.Log.Error("database error finding users", zap.Error(err))
		return nil, err
	}
	defer cur.Close(ctx)

	var users []models.User
	if err := cur.All(ctx, &users); err != nil {
		h.Log.Error("database error decoding users", zap.Error(err))
		return nil, err
	}

	out := make([]UserItem, len(users))
	for i, u := range users {
		loginID := ""
		if u.LoginID != nil {
			loginID = *u.LoginID
		}
		out[i] = UserItem{
			ID:       u.ID.Hex(),
			FullName: u.FullName,
			LoginID:  loginID,
		}
	}
	return out, nil
}
