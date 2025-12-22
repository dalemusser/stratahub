package resources

import (
	"context"
	"net/http"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/httpnav"
	"github.com/dalemusser/waffle/pantry/templates"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ServeView renders the admin detail view for a single resource.
func (h *AdminHandler) ServeView(w http.ResponseWriter, r *http.Request) {
	role, uname, _, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()
	db := h.DB

	idHex := chi.URLParam(r, "id")
	oid, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		uierrors.RenderBadRequest(w, r, "Invalid resource ID.", "/resources")
		return
	}

	var res models.Resource
	if err := db.Collection("resources").FindOne(ctx, bson.M{"_id": oid}).Decode(&res); err != nil {
		// Treat not found as a 404 to match other admin handlers
		uierrors.RenderNotFound(w, r, "Resource not found.", "/resources")
		return
	}

	data := viewData{
		Title:               "View Resource",
		IsLoggedIn:          true,
		Role:                role,
		UserName:            uname,
		ID:                  res.ID.Hex(),
		ResourceTitle:       res.Title,
		Subject:             res.Subject,
		Description:         res.Description,
		LaunchURL:           res.LaunchURL,
		Type:                res.Type,
		Status:              res.Status,
		ShowInLibrary:       res.ShowInLibrary,
		DefaultInstructions: res.DefaultInstructions,
		BackURL:             httpnav.ResolveBackURL(r, "/resources"),
	}

	templates.Render(w, r, "resource_view", data)
}
