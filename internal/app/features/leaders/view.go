package leaders

import (
	"context"
	"net/http"
	"strings"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/httpnav"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type viewData struct {
	Title, Role, UserName string
	IsLoggedIn            bool
	ID, FullName, Email   string
	OrgName, Status, Auth string
	BackURL               string
}

func (h *Handler) ServeView(w http.ResponseWriter, r *http.Request) {
	_, uname, _, _ := authz.UserCtx(r)

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

	data := viewData{
		Title:      "View Leader",
		IsLoggedIn: true,
		Role:       "admin",
		UserName:   uname,
		ID:         usr.ID.Hex(),
		FullName:   usr.FullName,
		Email:      strings.ToLower(usr.Email),
		OrgName:    orgName,
		Status:     usr.Status,
		Auth:       strings.ToLower(usr.AuthMethod),
		BackURL:    httpnav.ResolveBackURL(r, "/leaders"),
	}

	templates.Render(w, r, "admin_leader_view", data)
}
