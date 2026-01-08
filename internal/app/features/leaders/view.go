package leaders

import (
	"context"
	"net/http"
	"strings"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type viewData struct {
	viewdata.BaseVM
	ID, FullName, Email   string
	OrgName, Status, Auth string
}

func (h *Handler) ServeView(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()

	uidHex := chi.URLParam(r, "id")
	uid, err := primitive.ObjectIDFromHex(uidHex)
	if err != nil {
		uierrors.RenderBadRequest(w, r, "Invalid leader ID.", "/leaders")
		return
	}

	var usr models.User
	if err := h.DB.Collection("users").FindOne(ctx, bson.M{"_id": uid, "role": "leader"}).Decode(&usr); err != nil {
		uierrors.RenderNotFound(w, r, "Leader not found.", "/leaders")
		return
	}

	// Verify workspace ownership (prevent cross-workspace access)
	wsID := workspace.IDFromRequest(r)
	if wsID != primitive.NilObjectID && (usr.WorkspaceID == nil || *usr.WorkspaceID != wsID) {
		uierrors.RenderNotFound(w, r, "Leader not found.", "/leaders")
		return
	}

	// Coordinator access check: verify access to leader's organization
	if authz.IsCoordinator(r) && usr.OrganizationID != nil {
		if !authz.CanAccessOrg(r, *usr.OrganizationID) {
			uierrors.RenderForbidden(w, r, "You don't have access to this leader.", "/leaders")
			return
		}
	}

	orgName := ""
	if usr.OrganizationID != nil {
		var o models.Organization
		if err := h.DB.Collection("organizations").FindOne(ctx, bson.M{"_id": *usr.OrganizationID}).Decode(&o); err != nil {
			if err == mongo.ErrNoDocuments {
				orgName = "(Deleted)"
			} else {
				h.ErrLog.LogServerError(w, r, "database error loading organization for leader", err, "A database error occurred.", "/leaders")
				return
			}
		} else {
			orgName = o.Name
		}
	}

	loginID := ""
	if usr.LoginID != nil {
		loginID = *usr.LoginID
	}
	data := viewData{
		BaseVM:   viewdata.NewBaseVM(r, h.DB, "View Leader", "/leaders"),
		ID:       usr.ID.Hex(),
		FullName: usr.FullName,
		Email:    strings.ToLower(loginID),
		OrgName:  orgName,
		Status:   usr.Status,
		Auth:     strings.ToLower(usr.AuthMethod),
	}

	templates.Render(w, r, "admin_leader_view", data)
}
