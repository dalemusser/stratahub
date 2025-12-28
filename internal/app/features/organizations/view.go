// internal/app/features/organizations/view.go
package organizations

import (
	"context"
	"net/http"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	organizationstore "github.com/dalemusser/stratahub/internal/app/store/organizations"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/timezones"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/waffle/pantry/templates"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ServeView renders the read-only "View Organization" page.
// Authorization: RequireRole("admin") middleware in routes.go ensures only admins reach this handler.
func (h *Handler) ServeView(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()
	db := h.DB

	idHex := chi.URLParam(r, "id")
	orgID, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		uierrors.RenderBadRequest(w, r, "Invalid organization ID.", "/organizations")
		return
	}

	orgStore := organizationstore.New(db)
	org, err := orgStore.GetByID(ctx, orgID)
	if err != nil {
		// Treat not-found as a normal 404; other errors as 500.
		uierrors.RenderNotFound(w, r, "Organization not found.", "/organizations")
		return
	}

	tzGroups, err := timezones.Groups()
	if err != nil {
		uierrors.RenderServerError(w, r, "Failed to load time zones.", "/organizations")
		return
	}

	data := viewData{
		BaseVM:         viewdata.NewBaseVM(r, h.DB, "View Organization", "/organizations"),
		ID:             org.ID.Hex(),
		Name:           org.Name,
		City:           org.City,
		State:          org.State,
		TimeZone:       org.TimeZone,
		Contact:        org.ContactInfo,
		TimeZoneGroups: tzGroups,
	}

	templates.Render(w, r, "organization_view", data)
}
