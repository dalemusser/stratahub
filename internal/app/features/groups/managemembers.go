// internal/app/features/groups/managemembers.go
package groups

import (
	"context"
	"net/http"
	"net/url"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/policy/grouppolicy"
	groupstore "github.com/dalemusser/stratahub/internal/app/store/groups"
	membershipstore "github.com/dalemusser/stratahub/internal/app/store/memberships"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// HandleAddMember adds a member to the group (via search list).
func (h *Handler) HandleAddMember(w http.ResponseWriter, r *http.Request) {
	gid := chi.URLParam(r, "id")
	q := r.FormValue("q")
	after := r.FormValue("after")
	before := r.FormValue("before")

	groupOID, err := primitive.ObjectIDFromHex(gid)
	if err != nil {
		uierrors.HTMXBadRequest(w, r, "Invalid group ID.", "/groups")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()
	db := h.DB

	group, err := groupstore.New(db).GetByID(ctx, groupOID)
	if err == mongo.ErrNoDocuments {
		uierrors.HTMXError(w, r, http.StatusNotFound, "Group not found.", func() {
			uierrors.RenderNotFound(w, r, "Group not found.", "/groups")
		})
		return
	}
	if err != nil {
		h.ErrLog.HTMXLogServerError(w, r, "database error loading group", err, "A database error occurred.", "/groups")
		return
	}
	canManage, policyErr := grouppolicy.CanManageGroup(ctx, db, r, group.ID, group.OrganizationID)
	if policyErr != nil {
		h.ErrLog.HTMXLogServerError(w, r, "database error checking group access", policyErr, "A database error occurred.", "/groups")
		return
	}
	if !canManage {
		uierrors.HTMXForbidden(w, r, "You don't have permission to manage this group.", "/groups")
		return
	}

	targetOID, err := primitive.ObjectIDFromHex(r.FormValue("userID"))
	if err != nil {
		uierrors.HTMXBadRequest(w, r, "Invalid user ID.", "/groups")
		return
	}

	var u struct {
		ID             primitive.ObjectID `bson:"_id"`
		OrganizationID primitive.ObjectID `bson:"organization_id"`
		Role           string             `bson:"role"`
		Status         string             `bson:"status"`
	}
	if err := db.Collection("users").FindOne(ctx, bson.M{
		"_id":             targetOID,
		"organization_id": group.OrganizationID,
		"role":            "member",
		"status":          "active",
	}).Decode(&u); err != nil {
		uierrors.HTMXBadRequest(w, r, "Member must exist in same organization and be active.", "/groups")
		return
	}

	if err := membershipstore.New(db).Add(ctx, group.ID, targetOID, "member"); err != nil {
		h.ErrLog.HTMXLogBadRequest(w, r, "database error adding member to group", err, "Failed to add member.", "/groups")
		return
	}

	data, err := h.buildPageData(r, gid, q, after, before)
	if err != nil {
		h.ErrLog.HTMXLogServerError(w, r, "error building group page data", err, "Failed to load group data.", "/groups")
		return
	}

	// If the current page is now empty, adjust paging backwards or to first page.
	// Use refreshAvailableMembers to avoid N+1 pattern of rebuilding entire page.
	if data.AvailableShown == 0 {
		if after != "" {
			// Try to page backwards inclusively from current position
			if p, shown, total, nextCur, prevCur, hasNext, hasPrev, err2 :=
				h.fetchAvailablePrevInclusive(ctx, groupOID, q, after); err2 == nil {

				data.AvailableMembers = p
				data.AvailableShown = shown
				data.AvailableTotal = total
				data.NextCursor = nextCur
				data.PrevCursor = prevCur
				data.HasNext = hasNext
				data.HasPrev = hasPrev
				if !hasPrev {
					data.CurrentAfter, data.CurrentBefore = "", ""
				} else {
					data.CurrentAfter, data.CurrentBefore = "", prevCur
				}
			} else {
				// Fallback: refresh just the available members portion from first page
				h.refreshAvailableMembers(ctx, &data, group.OrganizationID, groupOID, q, "", "")
			}
		} else {
			// Refresh just the available members portion from first page
			h.refreshAvailableMembers(ctx, &data, group.OrganizationID, groupOID, q, "", "")
		}
	}

	// Re-render the members and available lists.
	templates.RenderSnippet(w, "group_members_list_inner", data)
	templates.RenderSnippet(w, "group_members_header_oob", data)
	templates.RenderSnippet(w, "group_available_members_block_oob", data)
}

// HandleRemoveMember removes a member from the group.
func (h *Handler) HandleRemoveMember(w http.ResponseWriter, r *http.Request) {
	gid := chi.URLParam(r, "id")
	q := r.FormValue("q")
	after := r.FormValue("after")
	before := r.FormValue("before")

	groupOID, err := primitive.ObjectIDFromHex(gid)
	if err != nil {
		uierrors.HTMXBadRequest(w, r, "Invalid group ID.", "/groups")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()
	db := h.DB

	group, err := groupstore.New(db).GetByID(ctx, groupOID)
	if err != nil {
		h.ErrLog.HTMXLogServerError(w, r, "database error loading group", err, "A database error occurred.", "/groups")
		return
	}
	canManage, policyErr := grouppolicy.CanManageGroup(ctx, db, r, group.ID, group.OrganizationID)
	if policyErr != nil {
		h.ErrLog.HTMXLogServerError(w, r, "database error checking group access", policyErr, "A database error occurred.", "/groups")
		return
	}
	if !canManage {
		uierrors.HTMXForbidden(w, r, "You don't have permission to manage this group.", "/groups")
		return
	}

	targetHex := r.FormValue("userID")
	targetOID, err := primitive.ObjectIDFromHex(targetHex)
	if err != nil {
		uierrors.HTMXBadRequest(w, r, "Invalid user ID.", "/groups")
		return
	}
	if err := membershipstore.New(db).Remove(ctx, group.ID, targetOID); err != nil {
		h.ErrLog.HTMXLogBadRequest(w, r, "database error removing member from group", err, "Failed to remove member.", "/groups")
		return
	}

	data, err := h.buildPageData(r, gid, q, after, before)
	if err != nil {
		h.ErrLog.HTMXLogServerError(w, r, "error building group page data", err, "Failed to load group data.", "/groups")
		return
	}

	// Guard stale before anchor -> first page.
	// Use refreshAvailableMembers to avoid N+1 pattern of rebuilding entire page.
	if before != "" && !data.HasPrev {
		h.refreshAvailableMembers(ctx, &data, group.OrganizationID, groupOID, q, "", "")
	}

	templates.RenderSnippet(w, "group_members_list_inner", data)
	templates.RenderSnippet(w, "group_members_header_oob", data)
	templates.RenderSnippet(w, "group_available_members_block_oob", data)

	// Also emit a "recently removed" chip snippet if we can load the user.
	var usr models.User
	if err := db.Collection("users").FindOne(ctx, bson.M{"_id": targetOID}).Decode(&usr); err != nil {
		if err != mongo.ErrNoDocuments {
			h.Log.Warn("failed to load user for recently removed chip",
				zap.Error(err),
				zap.String("user_id", targetOID.Hex()))
		}
	} else {
		chip := struct {
			FullName string
			GroupID  string
			UserID   string
		}{
			FullName: usr.FullName,
			GroupID:  gid,
			UserID:   targetOID.Hex(),
		}
		templates.RenderSnippet(w, "group_recent_chip_oob", chip)
	}
}

// ServeSearchMembers serves HTMX search + paging for available members.
func (h *Handler) ServeSearchMembers(w http.ResponseWriter, r *http.Request) {
	gid := chi.URLParam(r, "id")
	q := r.URL.Query().Get("q")
	after := r.URL.Query().Get("after")
	before := r.URL.Query().Get("before")

	data, err := h.buildPageData(r, gid, q, after, before)
	if err != nil {
		h.ErrLog.HTMXLogForbidden(w, r, "error building group page data", err, "Failed to load group data.", "/groups")
		return
	}

	// If we paged backwards and there's no previous page, snap to first page and update the URL.
	// Use refreshAvailableMembers to avoid N+1 pattern of rebuilding entire page.
	if before != "" && !data.HasPrev {
		groupOID, err := primitive.ObjectIDFromHex(gid)
		if err != nil {
			uierrors.HTMXBadRequest(w, r, "Invalid group ID.", "/groups")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
		defer cancel()

		group, grpErr := groupstore.New(h.DB).GetByID(ctx, groupOID)
		if grpErr == nil {
			h.refreshAvailableMembers(ctx, &data, group.OrganizationID, groupOID, q, "", "")
		}

		base := "/groups/" + gid + "/manage/search-members"
		v := url.Values{}
		if q != "" {
			v.Set("q", q)
		}
		if ret := r.URL.Query().Get("return"); ret != "" {
			v.Set("return", ret)
		}
		if enc := v.Encode(); enc != "" {
			base += "?" + enc
		}
		w.Header().Set("HX-Push-Url", base)
	}

	templates.RenderSnippet(w, "group_available_members_block", data)
}
