package leaders

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/templates"
	"github.com/dalemusser/waffle/toolkit/ui/nav"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const modalTimeout = 5 * time.Second

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
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), modalTimeout)
	defer cancel()

	var usr models.User
	if err := h.DB.Collection("users").FindOne(ctx, bson.M{"_id": uid, "role": "leader"}).Decode(&usr); err != nil {
		http.NotFound(w, r)
		return
	}

	orgName := ""
	if usr.OrganizationID != nil {
		var o models.Organization
		_ = h.DB.Collection("organizations").FindOne(ctx, bson.M{"_id": *usr.OrganizationID}).Decode(&o)
		orgName = o.Name
	}

	back := strings.TrimSpace(r.URL.Query().Get("return"))
	if back == "" {
		back = nav.ResolveBackURL(r, "/leaders")
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
