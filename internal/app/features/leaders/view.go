package leaders

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/templates"
	"github.com/dalemusser/waffle/toolkit/ui/nav"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type viewData struct {
	Title, Role, UserName string
	IsLoggedIn            bool
	ID, FullName, Email   string
	OrgName, Status, Auth string
	BackURL               string
}

func (h *Handler) ServeView(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.CurrentUser(r)

	// Short timeout for view operations
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	uidHex := chi.URLParam(r, "id")
	uid, err := primitive.ObjectIDFromHex(uidHex)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}

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

	data := viewData{
		Title:      "View Leader",
		IsLoggedIn: true,
		Role:       "admin",
		UserName:   u.Name,
		ID:         usr.ID.Hex(),
		FullName:   usr.FullName,
		Email:      strings.ToLower(usr.Email),
		OrgName:    orgName,
		Status:     usr.Status,
		Auth:       strings.ToLower(usr.AuthMethod),
		BackURL:    nav.ResolveBackURL(r, "/leaders"),
	}

	templates.Render(w, r, "admin_leader_view", data)
}
