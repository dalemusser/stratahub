// internal/app/features/resources/admindelete.go
package resources

import (
	"context"
	"net/http"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/txn"
	"github.com/dalemusser/waffle/pantry/urlutil"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// HandleDelete deletes a resource and its assignments.
// Authorization: RequireRole("admin") middleware in routes.go ensures only admins reach this handler.
func (h *AdminHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	idHex := chi.URLParam(r, "id")
	oid, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		uierrors.RenderBadRequest(w, r, "Invalid resource ID.", "/resources")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()
	db := h.DB

	// Use transaction for atomic deletion of resource and assignments.
	if err := txn.Run(ctx, db, h.Log, func(ctx context.Context) error {
		// 1) Clean up assignments first
		if _, err := db.Collection("group_resource_assignments").DeleteMany(ctx, bson.M{"resource_id": oid}); err != nil {
			return err
		}
		// 2) Delete resource
		if _, err := db.Collection("resources").DeleteOne(ctx, bson.M{"_id": oid}); err != nil {
			return err
		}
		return nil
	}); err != nil {
		h.ErrLog.LogServerError(w, r, "delete resource failed", err, "Unable to delete resource.", "")
		return
	}

	// HTMX flow: redirect via HX-Redirect
	if r.Header.Get("HX-Request") != "" {
		w.Header().Set("HX-Redirect", "/resources")
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Normal redirect
	ret := urlutil.SafeReturn(r.FormValue("return"), idHex, "/resources")
	http.Redirect(w, r, ret, http.StatusSeeOther)
}
