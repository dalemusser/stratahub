package leaders

import (
	"context"
	"net/http"
	"strings"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/waffle/pantry/query"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/httpnav"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// leaderManageModalData is the view model for the HTMX leader manage modal.
type leaderManageModalData struct {
	LeaderID string
	FullName string
	Email    string
	OrgName  string
	BackURL  string
}

// ServeLeaderManageModal renders the HTMX modal to manage a single leader
// (View / Edit / Delete) from the list. It is mounted on
// GET /leaders/{id}/manage_modal.
func (h *Handler) ServeLeaderManageModal(w http.ResponseWriter, r *http.Request) {
	uidHex := chi.URLParam(r, "id")
	uid, err := primitive.ObjectIDFromHex(uidHex)
	if err != nil {
		uierrors.HTMXBadRequest(w, r, "Invalid leader ID.", "/leaders")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()

	var usr models.User
	if err := h.DB.Collection("users").FindOne(ctx, bson.M{"_id": uid, "role": "leader"}).Decode(&usr); err != nil {
		uierrors.HTMXError(w, r, http.StatusNotFound, "Leader not found.", func() {
			uierrors.RenderNotFound(w, r, "Leader not found.", "/leaders")
		})
		return
	}

	orgName := ""
	if usr.OrganizationID != nil {
		var o models.Organization
		if err := h.DB.Collection("organizations").FindOne(ctx, bson.M{"_id": *usr.OrganizationID}).Decode(&o); err != nil {
			if err == mongo.ErrNoDocuments {
				orgName = "(Deleted)"
			} else {
				h.ErrLog.HTMXLogServerError(w, r, "database error loading organization for leader", err, "A database error occurred.", "/leaders")
				return
			}
		} else {
			orgName = o.Name
		}
	}

	back := query.Get(r, "return")
	if back == "" {
		back = httpnav.ResolveBackURL(r, "/leaders")
	}

	data := leaderManageModalData{
		LeaderID: uid.Hex(),
		FullName: usr.FullName,
		Email:    strings.ToLower(usr.Email),
		OrgName:  orgName,
		BackURL:  back,
	}

	templates.RenderSnippet(w, "admin_leader_manage_modal", data)
}
